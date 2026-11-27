package jobs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"hash/crc32"

	"log/slog"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type databaseLock struct {
	conn *sql.Conn
	key  int64
	pid  int
}

// acquires session-level advisory lock
func tryDatabaseLock(ctx context.Context, pool *sql.DB, databaseID string) (*databaseLock, error) {
	started := time.Now()
	ctx, span := otel.Tracer("platform/jobs").Start(ctx, "database.lock")

	defer span.End()

	span.SetAttributes(attribute.String("database.id", databaseID))
	conn, err := pool.Conn(ctx)
	if err != nil {
		telemetry.ObserveLockWait(time.Since(started), "error")
		return nil, err
	}

	lock := &databaseLock{conn: conn, key: int64(int32(crc32.ChecksumIEEE([]byte(databaseID))))}

	var acquired bool

	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1::bigint), pg_backend_pid()`, lock.key).Scan(&acquired, &lock.pid); err != nil {
		lock.discard()
		span.SetStatus(codes.Error, "database lock unavailable")
		telemetry.ObserveLockWait(time.Since(started), "error")
		return nil, err
	}

	if !acquired {
		conn.Close()
		telemetry.ObserveLockWait(time.Since(started), "busy")
		return nil, nil
	}

	telemetry.ObserveLockWait(time.Since(started), "acquired")
	telemetry.Log(ctx, slog.LevelInfo, "job.lock_acquired", "database_id", databaseID, "status", "success", "duration_ms", float64(time.Since(started))/float64(time.Millisecond))
	return lock, nil
}

// checks advisory lock session ownership
func (l *databaseLock) alive(ctx context.Context) error {
	var pid int

	if err := l.conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		return err
	}

	if pid != l.pid {
		return ErrClaimLost
	}

	return nil
}

// taints and closes lock connection
func (l *databaseLock) discard() {
	_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = l.conn.Close()
}

func (l *databaseLock) release() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var unlocked bool
	if err := l.conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1::bigint)`, l.key).Scan(&unlocked); err != nil || !unlocked {
		l.discard()
		return
	}

	_ = l.conn.Close()
}
