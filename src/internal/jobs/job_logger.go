package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Keep generated statements visible without exposing bound parameter values
type jobLogger struct{}

func (jobLogger) LogMode(logger.LogLevel) logger.Interface { return jobLogger{} }
func (jobLogger) Info(context.Context, string, ...any)     {}
func (jobLogger) Warn(context.Context, string, ...any)     {}
func (jobLogger) Error(context.Context, string, ...any)    {}

func (jobLogger) ParamsFilter(_ context.Context, statement string, _ ...any) (string, []any) {
	return statement, nil
}

func (jobLogger) Trace(ctx context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	telemetry.Log(ctx, slog.LevelInfo, "sql.statement", "query", query, "parameters", "[redacted]")
}

func (r *PostgresRepository) run(ctx context.Context, operation string, query func(*gorm.DB) *gorm.DB) *gorm.DB {
	ctx, span := otel.Tracer("platform/jobs").Start(ctx, "queue.database")
	defer span.End()
	span.SetAttributes(attribute.String("db.operation.name", operation))
	started := time.Now()
	telemetry.Log(ctx, slog.LevelInfo, "sql.started", "operation", operation)
	db := r.DB.Session(&gorm.Session{NewDB: true, Logger: jobLogger{}}).WithContext(ctx)
	result := query(db)
	status, sqlstate := "success", ""
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		status = "not_found"
	} else if result.Error != nil {
		status = "failed"
		var state interface{ SQLState() string }
		if errors.As(result.Error, &state) {
			sqlstate = state.SQLState()
		}
		span.SetStatus(codes.Error, "queue database operation failed")
	}
	span.SetAttributes(attribute.Int64("db.rows", result.RowsAffected))
	telemetry.Log(ctx, slog.LevelInfo, "sql.finished", "operation", operation, "status", status,
		"sqlstate", sqlstate, "duration_ms", float64(time.Since(started))/float64(time.Millisecond), "rows", result.RowsAffected)
	return result
}
