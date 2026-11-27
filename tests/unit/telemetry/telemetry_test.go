package telemetry_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestStructuredLogAndRedaction(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(telemetry.NewJSONHandler(&buf)).With("service", "platform-api"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := context.Background()
	ctx = telemetry.WithRequestID(ctx, "req-abc123")
	ctx = telemetry.Correlate(ctx, "user-42", "db-99", "job-7", "exec-55")
	ctx = telemetry.WithMigration(ctx, "mig-11")
	ctx = telemetry.WithAttempt(ctx, 2)
	ctx = telemetry.WithBatch(ctx, 3)

	telemetry.Log(ctx, slog.LevelInfo, "sql.finished",
		"status", "success",
		"query", "UPDATE users SET active = true WHERE id > $1 LIMIT $2",
		"parameters", telemetry.RedactParams(100, 500),
		"password", "PlainSecretPass!",
		"session_id", "raw-session-xyz",
		"csrf_token", "raw-csrf-999",
		"authorization", "Bearer super-secret-jwt",
		"cookie", "session=abc123",
		"error", "failed connecting to postgres://migrator:TopSecret42@db:5432/app?sslmode=disable",
		"duration_ms", 12.5,
		"rows", 500,
	)

	raw := buf.String()
	for _, secret := range []string{"PlainSecretPass!", "raw-session-xyz", "raw-csrf-999", "super-secret-jwt", "session=abc123", "TopSecret42"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("sensitive value %q leaked into log output: %s", secret, raw)
		}
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, raw)
	}

	for _, key := range []string{"time", "level", "event", "service", "request_id", "actor", "database_id", "migration_id", "job_id", "execution_id", "attempt", "batch", "status", "query", "parameters"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("missing required JSON key %q in log entry: %v", key, entry)
		}
	}
	if entry["event"] != "sql.finished" {
		t.Fatalf("event = %v, want sql.finished", entry["event"])
	}
	for _, redactedKey := range []string{"password", "session_id", "csrf_token", "authorization", "cookie"} {
		if entry[redactedKey] != "[REDACTED]" {
			t.Fatalf("%s = %v, want [REDACTED]", redactedKey, entry[redactedKey])
		}
	}
	if !strings.Contains(entry["error"].(string), "postgres://migrator:[REDACTED]@db:5432/app") {
		t.Fatalf("DSN password not sanitized in error: %v", entry["error"])
	}
}

func TestTracePropagationAPIQueueWorker(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	var buf bytes.Buffer
	prevLog := slog.Default()
	slog.SetDefault(slog.New(telemetry.NewJSONHandler(&buf)).With("service", "platform-worker"))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	// 1. API request span -> Queue enqueue span
	apiCtx, apiSpan := otel.Tracer("platform/http").Start(context.Background(), "http.request", trace.WithSpanKind(trace.SpanKindServer))
	enqCtx, enqSpan := otel.Tracer("platform/jobs").Start(apiCtx, "queue.enqueue", trace.WithSpanKind(trace.SpanKindProducer))
	carrier := telemetry.Carrier(enqCtx)
	enqSpan.End()
	apiSpan.End()

	// Simulate persistence in migration_jobs.trace_json
	blob, err := json.Marshal(carrier)
	if err != nil {
		t.Fatalf("marshal carrier: %v", err)
	}
	var restored map[string]string
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatalf("unmarshal carrier: %v", err)
	}

	// 2. Worker extracts context -> Lock -> Migration -> Batch span
	workerCtx := telemetry.Extract(context.Background(), restored)
	workerCtx = telemetry.Correlate(workerCtx, "user-1", "db-1", "job-1", "exec-1")
	workerCtx, procSpan := otel.Tracer("platform/jobs").Start(workerCtx, "queue.process", trace.WithSpanKind(trace.SpanKindConsumer))
	lockCtx, lockSpan := otel.Tracer("platform/jobs").Start(workerCtx, "database.lock")
	lockSpan.End()
	execCtx, execSpan := otel.Tracer("platform/jobs").Start(lockCtx, "migration.execute")
	batchCtx, batchSpan := otel.Tracer("platform/migration").Start(telemetry.WithBatch(execCtx, 1), "migration.batch")

	telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_commit", "operation", "migration.batch", "status", "success", "rows", 1000)

	batchSpan.End()
	execSpan.End()
	procSpan.End()

	spans := recorder.Ended()
	if len(spans) != 6 {
		t.Fatalf("ended spans = %d, want 6", len(spans))
	}
	rootTraceID := apiSpan.SpanContext().TraceID()
	for _, s := range spans {
		if s.SpanContext().TraceID() != rootTraceID {
			t.Fatalf("span %q trace_id = %s, want %s", s.Name(), s.SpanContext().TraceID(), rootTraceID)
		}
	}

	// Verify log entry carries correlated trace_id & batch span_id
	var logEntry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry); err != nil {
		t.Fatalf("decode worker log: %v", err)
	}
	if logEntry["trace_id"] != rootTraceID.String() {
		t.Fatalf("log trace_id = %v, want %s", logEntry["trace_id"], rootTraceID.String())
	}
	if logEntry["span_id"] != batchSpan.SpanContext().SpanID().String() {
		t.Fatalf("log span_id = %v, want %s", logEntry["span_id"], batchSpan.SpanContext().SpanID().String())
	}
}

func TestHTTPMiddlewareLogsAndSanitizes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
	})

	var buf bytes.Buffer
	prevLog := slog.Default()
	slog.SetDefault(slog.New(telemetry.NewJSONHandler(&buf)).With("service", "platform-api"))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	h := telemetry.HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = telemetry.WithActor(r.Context(), "actor-77")
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/db-1/queries?token=url-secret&password=123", nil)
	req.Header.Set("Cookie", "session_id=cookie-secret-val")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}

	raw := buf.String()
	for _, secret := range []string{"url-secret", "cookie-secret-val", "password=123"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("HTTP middleware leaked sensitive value %q: %s", secret, raw)
		}
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("decode HTTP log: %v\nraw: %s", err, raw)
	}
	if entry["event"] != "http.request_finished" || entry["actor"] != "actor-77" || entry["level"] != "ERROR" {
		t.Fatalf("unexpected HTTP log entry: %+v", entry)
	}

	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("expected 1 error span for 500 response, got %+v", spans)
	}
}

type stubSQLStateErr struct{ code string }

func (e stubSQLStateErr) Error() string    { return "pg error" }
func (e stubSQLStateErr) SQLState() string { return e.code }

func TestSQLStateExtraction(t *testing.T) {
	if got := telemetry.SQLState(nil); got != "" {
		t.Fatalf("nil err sqlstate = %q, want empty", got)
	}
	if got := telemetry.SQLState(errors.New("plain")); got != "" {
		t.Fatalf("plain err sqlstate = %q, want empty", got)
	}
	if got := telemetry.SQLState(stubSQLStateErr{code: "23505"}); got != "23505" {
		t.Fatalf("pgconn sqlstate = %q, want 23505", got)
	}
	if got := telemetry.SQLState(&pq.Error{Code: "42P01"}); got != "42P01" {
		t.Fatalf("pq sqlstate = %q, want 42P01", got)
	}
	if got := telemetry.SQLState(&mysql.MySQLError{Number: 1062}); got != "MYSQL_1062" {
		t.Fatalf("mysql sqlstate = %q, want MYSQL_1062", got)
	}
}

func TestPrometheusMetricsEndpointAndLowCardinality(t *testing.T) {
	telemetry.RegisterDBPoolProvider("test_pools", func() map[string]sql.DBStats {
		return map[string]sql.DBStats{
			"platform": {
				MaxOpenConnections: 10,
				OpenConnections:    4,
				InUse:              2,
				Idle:               2,
				WaitCount:          3,
				WaitDuration:       150 * time.Millisecond,
			},
		}
	})
	telemetry.RegisterJobStatusCounter(func(context.Context) (map[string]int64, error) {
		return map[string]int64{"PENDING": 2, "RUNNING": 1, "SUCCEEDED": 5, "FAILED": 1}, nil
	})
	t.Cleanup(func() {
		telemetry.RegisterDBPoolProvider("test_pools", nil)
		telemetry.RegisterJobStatusCounter(nil)
	})

	telemetry.IncJobEnqueued()
	telemetry.IncJobRetry("retry_resume")
	telemetry.IncJobFinished("succeeded")
	telemetry.ObserveQueueWait(250 * time.Millisecond)
	telemetry.ObserveLockWait(15 * time.Millisecond, "acquired")
	telemetry.ObserveMigrationDuration("batch", "forward", "succeeded", 1200*time.Millisecond)
	telemetry.IncBatchProcessed("forward", "committed", 1000)

	rr := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()

	requiredMetrics := []string{
		"platform_migration_jobs_by_status",
		"platform_migration_jobs_total",
		"platform_migration_job_retries_total",
		"platform_migration_queue_wait_seconds",
		"platform_migration_lock_wait_seconds",
		"platform_migration_duration_seconds",
		"platform_migration_batches_processed_total",
		"platform_migration_rows_processed_total",
		"platform_db_pool_connections",
		"platform_db_pool_wait_count_total",
		"platform_db_pool_wait_duration_seconds_total",
	}
	for _, metric := range requiredMetrics {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing Prometheus metric %q in /metrics output", metric)
		}
	}

	// Enforce spec hal. 12: avoid high-cardinality labels (user/job/execution ID, SQL)
	for _, forbiddenLabel := range []string{"user_id=", "actor=", "job_id=", "execution_id=", "database_id=", "sql=", "query="} {
		if strings.Contains(body, forbiddenLabel) {
			t.Fatalf("high-cardinality label %q found in /metrics output", forbiddenLabel)
		}
	}
}

func TestOTLPExportsTracesAndLogsToCollector(t *testing.T) {
	var (
		mu          sync.Mutex
		gotTraces   bool
		gotLogs     bool
		logPayloads []byte
	)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		switch r.URL.Path {
		case "/v1/traces":
			gotTraces = true
		case "/v1/logs":
			gotLogs = true
			logPayloads = append(logPayloads, body...)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	prevLog := slog.Default()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(prevLog)
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	shutdown := telemetry.Init("platform-api-test")

	ctx, span := otel.Tracer("platform/test").Start(context.Background(), "test.op")
	telemetry.Log(ctx, slog.LevelInfo, "test.otlp_event", "status", "success", "password", "NeverLeakOTLP!")
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("telemetry shutdown failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !gotTraces {
		t.Fatal("OTLP collector did not receive POST /v1/traces")
	}
	if !gotLogs {
		t.Fatal("OTLP collector did not receive POST /v1/logs")
	}
	if bytes.Contains(logPayloads, []byte("NeverLeakOTLP!")) {
		t.Fatal("sensitive password leaked into OTLP log export payload")
	}
}

func TestLogSanitizesNewlinesAgainstLogInjection(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(telemetry.NewJSONHandler(&buf)).With("service", "platform-api"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := telemetry.WithDatabase(context.Background(), "db-1\r\nforged")
	telemetry.Log(ctx, slog.LevelWarn, "auth.login_failed\r\nfake",
		"username", "alice\n{\"event\":\"auth.login_succeeded\"}",
		"path", "/api/v1/databases/db-1\r\n/evil",
	)

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("decode sanitized log: %v", err)
	}
	for _, field := range []string{"event", "username", "path", "database_id"} {
		val, _ := entry[field].(string)
		if strings.ContainsAny(val, "\r\n") {
			t.Fatalf("field %q still contains CR/LF: %q", field, val)
		}
	}
}

