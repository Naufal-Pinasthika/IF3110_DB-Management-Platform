// Package telemetry supplies redacted JSON logging, OpenTelemetry tracing, and Prometheus metrics.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const redacted = "[REDACTED]"

var (
	dsnPasswordPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:\s/@]+:)([^@\s/]+)(@)`)
	bearerPattern      = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]+`)
	kvSecretPattern    = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|csrf_token|session_id|session_token|cookie|api_key)\s*([=:])\s*([^\s;,&'"]+)`)
)

// RedactKey reports whether a log attribute key may carry credentials or session identifiers.
func RedactKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{
		"password", "passwd", "secret", "token", "cookie",
		"authorization", "csrf", "credential", "encryption_key",
		"private_key", "dsn", "database_url", "api_key",
		"session_id", "session_token", "session_cookie",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// stripLogNewlines removes CR/LF control characters to prevent log injection.
func stripLogNewlines(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", " ")
	return v
}

// SanitizeValue masks credentials, DSN passwords, bearer tokens, secret key-value pairs,
// and newline control characters.
func SanitizeValue(v string) string {
	if v == "" {
		return v
	}
	v = stripLogNewlines(v)
	v = dsnPasswordPattern.ReplaceAllString(v, "${1}"+redacted+"${3}")
	v = bearerPattern.ReplaceAllString(v, "${1}"+redacted)
	v = kvSecretPattern.ReplaceAllString(v, "${1}${2}"+redacted)
	return v
}

// RedactParams returns redacted placeholders matching the supplied parameter count.
func RedactParams(params ...any) []string {
	if len(params) == 0 {
		return []string{}
	}
	out := make([]string, len(params))
	for i := range out {
		out[i] = redacted
	}
	return out
}

// SQLState extracts a PostgreSQL SQLSTATE or MySQL error code from an error chain.
func SQLState(err error) string {
	if err == nil {
		return ""
	}
	var stateErr interface{ SQLState() string }
	if errors.As(err, &stateErr) {
		return stateErr.SQLState()
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code)
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return fmt.Sprintf("MYSQL_%d", myErr.Number)
	}
	return ""
}

// NewJSONHandler builds a JSON slog.Handler with "event" as message key and automatic redaction.
func NewJSONHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				a.Key = "event"
				return a
			}
			if RedactKey(a.Key) {
				return slog.String(a.Key, redacted)
			}
			if a.Value.Kind() == slog.KindString {
				return slog.String(a.Key, SanitizeValue(a.Value.String()))
			}
			return a
		},
	})
}

// Init configures structured JSON logging to stdout and OpenTelemetry tracing.
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, logs and traces are also exported via OTLP/HTTP
// to an OpenTelemetry Collector; otherwise traces fall back to console JSON output.
func Init(service string) func(context.Context) error {
	res := resource.NewSchemaless(attribute.String("service.name", service))
	stdoutHandler := NewJSONHandler(os.Stdout)

	var shutdowns []func(context.Context) error
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

	var traceExporter sdktrace.SpanExporter = consoleExporter{}
	var logHandler slog.Handler = stdoutHandler

	if endpoint != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Rely on OTel's native OTEL_EXPORTER_OTLP_ENDPOINT handling so /v1/traces and /v1/logs are preserved.
		if exp, err := otlptracehttp.New(ctx); err == nil {
			traceExporter = exp
		} else {
			slog.New(stdoutHandler).Warn("telemetry.otlp_trace_init_failed", "service", service, "error", SanitizeValue(err.Error()))
		}

		if logExp, err := otlploghttp.New(ctx); err == nil {
			logProvider := sdklog.NewLoggerProvider(
				sdklog.WithResource(res),
				sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
			)
			shutdowns = append(shutdowns, logProvider.Shutdown)
			logHandler = multiHandler{handlers: []slog.Handler{
				stdoutHandler,
				redactingHandler{next: otelslog.NewHandler(service, otelslog.WithLoggerProvider(logProvider))},
			}}
		} else {
			slog.New(stdoutHandler).Warn("telemetry.otlp_log_init_failed", "service", service, "error", SanitizeValue(err.Error()))
		}
	}

	slog.SetDefault(slog.New(logHandler).With("service", service))

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter, sdktrace.WithMaxQueueSize(1024)),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagationTraceContext{})
	shutdowns = append(shutdowns, provider.Shutdown)

	return func(ctx context.Context) error {
		var firstErr error
		for _, fn := range shutdowns {
			if err := fn(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

// Carrier serializes only W3C TraceContext (no baggage, cookies, credentials, or sessions).
func Carrier(ctx context.Context) map[string]string {
	carrier := mapCarrier{}
	propagationTraceContext{}.Inject(ctx, carrier)
	return carrier
}

// Extract restores W3C TraceContext from a serialized map carrier.
func Extract(ctx context.Context, carrier map[string]string) context.Context {
	return propagationTraceContext{}.Extract(ctx, mapCarrier(carrier))
}

type correlationKey struct{}
type actorSlotKey struct{}

type actorSlot struct {
	mu    sync.Mutex
	actor string
}

type correlation struct {
	requestID   string
	actor       string
	database    string
	migrationID string
	job         string
	execution   string
	attempt     int
	batch       int
}

func currentCorrelation(ctx context.Context) correlation {
	if fields, ok := ctx.Value(correlationKey{}).(correlation); ok {
		return fields
	}
	return correlation{}
}

// Correlate attaches non-secret identifiers to context while preserving previously set fields.
func Correlate(ctx context.Context, actor, databaseID, jobID, executionID string) context.Context {
	fields := currentCorrelation(ctx)
	if actor != "" {
		fields.actor = actor
	}
	if databaseID != "" {
		fields.database = databaseID
	}
	if jobID != "" {
		fields.job = jobID
	}
	if executionID != "" {
		fields.execution = executionID
	}
	return context.WithValue(ctx, correlationKey{}, fields)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	fields := currentCorrelation(ctx)
	fields.requestID = requestID
	return context.WithValue(ctx, correlationKey{}, fields)
}

func WithActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	if slot, ok := ctx.Value(actorSlotKey{}).(*actorSlot); ok && slot != nil {
		slot.mu.Lock()
		slot.actor = actor
		slot.mu.Unlock()
	}
	return Correlate(ctx, actor, "", "", "")
}

func WithDatabase(ctx context.Context, databaseID string) context.Context {
	return Correlate(ctx, "", databaseID, "", "")
}

func WithMigration(ctx context.Context, migrationID string) context.Context {
	if migrationID == "" {
		return ctx
	}
	fields := currentCorrelation(ctx)
	fields.migrationID = migrationID
	return context.WithValue(ctx, correlationKey{}, fields)
}

func WithAttempt(ctx context.Context, attempt int) context.Context {
	if attempt <= 0 {
		return ctx
	}
	fields := currentCorrelation(ctx)
	fields.attempt = attempt
	return context.WithValue(ctx, correlationKey{}, fields)
}

func WithBatch(ctx context.Context, batch int) context.Context {
	if batch <= 0 {
		return ctx
	}
	fields := currentCorrelation(ctx)
	fields.batch = batch
	return context.WithValue(ctx, correlationKey{}, fields)
}

func sanitizeLogArg(v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		return stripLogNewlines(val)
	case []string:
		clean := make([]string, len(val))
		for i, item := range val {
			clean[i] = stripLogNewlines(item)
		}
		return clean
	case slog.Attr:
		return slog.String(stripLogNewlines(val.Key), stripLogNewlines(val.Value.String()))
	case error:
		return stripLogNewlines(val.Error())
	case fmt.Stringer:
		return stripLogNewlines(val.String())
	case int:
		n, _ := strconv.Atoi(stripLogNewlines(strconv.Itoa(val)))
		return n
	case int64:
		n, _ := strconv.ParseInt(stripLogNewlines(strconv.FormatInt(val, 10)), 10, 64)
		return n
	case float64:
		f, _ := strconv.ParseFloat(stripLogNewlines(strconv.FormatFloat(val, 'g', -1, 64)), 64)
		return f
	case bool:
		if val {
			return true
		}
		return false
	default:
		return stripLogNewlines(fmt.Sprint(val))
	}
}

// Log emits a structured JSON log event enriched with context correlation IDs and OTel trace/span IDs.
func Log(ctx context.Context, level slog.Level, event string, attrs ...any) {
	cleanEvent := stripLogNewlines(event)
	cleanAttrs := make([]any, 0, len(attrs)+16)
	for _, attr := range attrs {
		cleanAttrs = append(cleanAttrs, sanitizeLogArg(attr))
	}

	hasAttr := func(name string) bool {
		for i := 0; i+1 < len(cleanAttrs); i += 2 {
			if cleanAttrs[i] == name {
				return true
			}
		}
		return false
	}

	if fields, ok := ctx.Value(correlationKey{}).(correlation); ok {
		for _, field := range []struct{ name, value string }{
			{"request_id", fields.requestID},
			{"actor", fields.actor},
			{"database_id", fields.database},
			{"migration_id", fields.migrationID},
			{"job_id", fields.job},
			{"execution_id", fields.execution},
		} {
			if field.value != "" && !hasAttr(field.name) {
				cleanAttrs = append(cleanAttrs, field.name, stripLogNewlines(field.value))
			}
		}
		if fields.attempt > 0 && !hasAttr("attempt") {
			cleanAttrs = append(cleanAttrs, "attempt", fields.attempt)
		}
		if fields.batch > 0 && !hasAttr("batch") {
			cleanAttrs = append(cleanAttrs, "batch", fields.batch)
		}
	}
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		if !hasAttr("trace_id") {
			cleanAttrs = append(cleanAttrs, "trace_id", sc.TraceID().String())
		}
		if !hasAttr("span_id") {
			cleanAttrs = append(cleanAttrs, "span_id", sc.SpanID().String())
		}
	}
	slog.Log(ctx, level, cleanEvent, cleanAttrs...)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newRequestID(headerVal string) string {
	trimmed := strings.TrimSpace(headerVal)
	if trimmed != "" && len(trimmed) <= 64 && !strings.ContainsAny(trimmed, " \t\r\n") {
		return trimmed
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// HTTP starts a server trace span, attaches a correlation request_id, and logs request completion
// without recording URL query parameters, cookies, credentials, or request bodies.
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagationTraceContext{}.Extract(r.Context(), headerCarrier(r.Header))
		reqID := newRequestID(r.Header.Get("X-Request-ID"))
		slot := &actorSlot{}
		ctx = context.WithValue(ctx, actorSlotKey{}, slot)
		ctx = WithRequestID(ctx, reqID)

		ctx, span := otel.Tracer("platform/http").Start(ctx, "http.request",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
				attribute.String("request.id", reqID),
			))
		defer span.End()

		w.Header().Set("X-Request-ID", reqID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()

		next.ServeHTTP(rec, r.WithContext(ctx))

		durationMS := float64(time.Since(started)) / float64(time.Millisecond)
		span.SetAttributes(attribute.Int("http.response.status_code", rec.status))
		statusLabel := "success"
		level := slog.LevelInfo
		if rec.status >= 500 {
			statusLabel = "error"
			level = slog.LevelError
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		} else if rec.status >= 400 {
			statusLabel = "client_error"
			level = slog.LevelWarn
		}

		// Skip noisy scrape/probe endpoints unless they fail.
		if (r.URL.Path == "/health" || r.URL.Path == "/metrics") && rec.status < 400 {
			return
		}

		slot.mu.Lock()
		actor := slot.actor
		slot.mu.Unlock()
		if actor != "" {
			ctx = Correlate(ctx, actor, "", "", "")
		}

		Log(ctx, level, "http.request_finished",
			"method", r.Method,
			"path", r.URL.Path,
			"http_status", rec.status,
			"status", statusLabel,
			"duration_ms", durationMS,
		)
	})
}

type consoleExporter struct{}

func (consoleExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		attrs := make(map[string]any)
		for _, attr := range span.Attributes() {
			k := string(attr.Key)
			if RedactKey(k) {
				attrs[k] = redacted
				continue
			}
			v := attr.Value.AsInterface()
			if s, ok := v.(string); ok {
				v = SanitizeValue(s)
			}
			attrs[k] = v
		}
		slog.InfoContext(ctx, "trace.span",
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
			"parent_span_id", span.Parent().SpanID().String(),
			"name", span.Name(),
			"kind", span.SpanKind().String(),
			"status", span.Status().Code.String(),
			"started_at", span.StartTime().UTC().Format(time.RFC3339Nano),
			"duration_ms", float64(span.EndTime().Sub(span.StartTime()))/float64(time.Millisecond),
			"attributes", attrs,
		)
	}
	return nil
}

func (consoleExporter) Shutdown(context.Context) error { return nil }

// multiHandler fans out slog records to stdout JSON and optional OTLP log bridge.
type multiHandler struct {
	handlers []slog.Handler
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return multiHandler{handlers: next}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return multiHandler{handlers: next}
}

// redactingHandler ensures sensitive fields are masked before forwarding to OTLP log bridge.
type redactingHandler struct {
	next slog.Handler
}

func sanitizeAttr(a slog.Attr) slog.Attr {
	if RedactKey(a.Key) {
		return slog.String(a.Key, redacted)
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, SanitizeValue(a.Value.String()))
	}
	return a
}

func (r redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return r.next.Enabled(ctx, level)
}

func (r redactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(sanitizeAttr(a))
		return true
	})
	return r.next.Handle(ctx, clean)
}

func (r redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = sanitizeAttr(a)
	}
	return redactingHandler{next: r.next.WithAttrs(clean)}
}

func (r redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{next: r.next.WithGroup(name)}
}
