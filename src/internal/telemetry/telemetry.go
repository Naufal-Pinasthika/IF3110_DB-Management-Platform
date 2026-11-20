// Package telemetry supplies JSON logging and console OpenTelemetry traces.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Init configures a bounded batch exporter to stdout; no collector is required.
// Call the returned shutdown function after the API/worker has stopped.
func Init(service string) func(context.Context) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
		sdktrace.WithBatcher(consoleExporter{}, sdktrace.WithMaxQueueSize(1024)),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown
}

// Carrier intentionally excludes baggage, cookies, credentials, and sessions.
func Carrier(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier
}

func Extract(ctx context.Context, carrier map[string]string) context.Context {
	return propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier(carrier))
}

type correlationKey struct{}
type correlation struct{ actor, database, job, execution string }

// Correlate attaches non-secret identifiers to repository and worker log events.
func Correlate(ctx context.Context, actor, databaseID, jobID, executionID string) context.Context {
	return context.WithValue(ctx, correlationKey{}, correlation{actor, databaseID, jobID, executionID})
}

func Log(ctx context.Context, level slog.Level, event string, attrs ...any) {
	if fields, ok := ctx.Value(correlationKey{}).(correlation); ok {
		for _, field := range []struct{ name, value string }{
			{"actor", fields.actor}, {"database_id", fields.database}, {"job_id", fields.job}, {"execution_id", fields.execution},
		} {
			present := false
			for i := 0; i+1 < len(attrs); i += 2 {
				if attrs[i] == field.name {
					present = true
					break
				}
			}
			if field.value != "" && !present {
				attrs = append(attrs, field.name, field.value)
			}
		}
	}
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		attrs = append(attrs, "trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
	}
	slog.Log(ctx, level, event, attrs...)
}

// HTTP keeps SQL, URL query parameters, and bodies out of automatic spans/logs.
// Domain instrumentation can add known, non-sensitive attributes explicitly.
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("platform/http").Start(ctx, "http.request",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", r.Method)))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type consoleExporter struct{}

func (consoleExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		attrs := make(map[string]any)
		for _, attr := range span.Attributes() {
			attrs[string(attr.Key)] = attr.Value.AsInterface()
		}
		slog.InfoContext(ctx, "trace.span", "trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(), "parent_span_id", span.Parent().SpanID().String(),
			"name", span.Name(), "kind", span.SpanKind().String(), "status", span.Status().Code.String(),
			"started_at", span.StartTime().UTC().Format(time.RFC3339Nano),
			"duration_ms", float64(span.EndTime().Sub(span.StartTime()))/float64(time.Millisecond), "attributes", attrs)
	}
	return nil
}

func (consoleExporter) Shutdown(context.Context) error { return nil }
