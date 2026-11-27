package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/sqlguard"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"github.com/go-sql-driver/mysql"
	pgquery "github.com/pganalyze/pg_query_go/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type Runner struct {
	Targets *store.TargetStore
	Timeout time.Duration
}

var _ SQLRunner = (*Runner)(nil)

func (r *Runner) Execute(ctx context.Context, databaseID, sqlText string) error {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return err
	}
	statements := []string{sqlText}
	var plan sqlguard.MySQLMigration
	if config.TargetEngine() == models.EngineMySQL {
		plan, err = sqlguard.ParseMySQLMigration(sqlText, config.DatabaseName)
		if err != nil {
			return err
		}
		statements = plan.Statements
	} else {
		parsed, err := pgquery.Parse(sqlText)
		if err != nil || len(parsed.Stmts) == 0 {
			return fmt.Errorf("%w: invalid migration SQL", apperror.ErrInvalidInput)
		}
		for _, raw := range parsed.Stmts {
			if raw.Stmt.GetTransactionStmt() != nil {
				return fmt.Errorf("%w: manual transaction control is forbidden", apperror.ErrInvalidInput)
			}
		}
	}
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("migrator connection failed: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	ctx, span := otel.Tracer("platform/migration").Start(ctx, "migration.sql")
	span.SetAttributes(
		attribute.String("database.id", databaseID),
		attribute.String("db.system", config.TargetEngine()),
	)
	defer span.End()

	if plan.DDL {
		return executeMySQLDDL(ctx, db, statements[0])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "begin transaction failed")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if config.TargetEngine() == models.EngineMySQL {
		for _, table := range plan.Tables {
			var engine sql.NullString
			err := tx.QueryRowContext(ctx, "SELECT engine FROM information_schema.tables WHERE table_schema=? AND table_name=?", config.DatabaseName, table).Scan(&engine)
			if err != nil || !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
				return fmt.Errorf("%w: DML migrations require existing InnoDB tables", apperror.ErrInvalidInput)
			}
		}
	}
	for _, statement := range statements {
		stmtStarted := time.Now()
		telemetry.Log(ctx, slog.LevelInfo, "sql.started",
			"operation", "migration.execute",
			"database_id", databaseID,
			"query", statement,
			"parameters", telemetry.RedactParams(),
			"status", "started",
		)
		res, execErr := tx.ExecContext(ctx, statement)
		durMS := float64(time.Since(stmtStarted)) / float64(time.Millisecond)
		if execErr != nil {
			span.RecordError(execErr)
			span.SetStatus(codes.Error, "migration SQL failed")
			telemetry.Log(ctx, slog.LevelError, "sql.finished",
				"operation", "migration.execute",
				"database_id", databaseID,
				"query", statement,
				"parameters", telemetry.RedactParams(),
				"duration_ms", durMS,
				"status", "failed",
				"error", telemetry.SanitizeValue(execErr.Error()),
				"sqlstate", telemetry.SQLState(execErr),
			)
			if rbErr := tx.Rollback(); rbErr != nil {
				telemetry.Log(ctx, slog.LevelError, "sql.transaction_rollback",
					"operation", "migration.execute",
					"database_id", databaseID,
					"status", "failed",
					"error", telemetry.SanitizeValue(rbErr.Error()),
					"sqlstate", telemetry.SQLState(rbErr),
				)
				return fmt.Errorf("%w: execution failed and rollback was not confirmed", apperror.ErrTransactionOutcomeUnknown)
			}
			committed = true
			telemetry.Log(ctx, slog.LevelInfo, "sql.transaction_rollback",
				"operation", "migration.execute",
				"database_id", databaseID,
				"status", "success",
			)
			return fmt.Errorf("%w: migration failed and safely rolled back: %v", apperror.ErrInvalidInput, execErr)
		}
		rowsAffected, _ := res.RowsAffected()
		span.SetAttributes(attribute.Int64("db.rows", rowsAffected))
		telemetry.Log(ctx, slog.LevelInfo, "sql.finished",
			"operation", "migration.execute",
			"database_id", databaseID,
			"query", statement,
			"parameters", telemetry.RedactParams(),
			"duration_ms", durMS,
			"rows", rowsAffected,
			"status", "success",
		)
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "migration commit unconfirmed")
		telemetry.Log(ctx, slog.LevelError, "sql.transaction_commit",
			"operation", "migration.execute",
			"database_id", databaseID,
			"status", "failed",
			"error", telemetry.SanitizeValue(err.Error()),
			"sqlstate", telemetry.SQLState(err),
		)
		return fmt.Errorf("%w: target commit was not confirmed", apperror.ErrTransactionOutcomeUnknown)
	}
	committed = true
	telemetry.Log(ctx, slog.LevelInfo, "sql.transaction_commit",
		"operation", "migration.execute",
		"database_id", databaseID,
		"status", "success",
	)
	return nil
}
func executeMySQLDDL(ctx context.Context, db *sql.DB, statement string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET SESSION default_storage_engine = 'InnoDB'"); err != nil {
		return err
	}
	started := time.Now()
	telemetry.Log(ctx, slog.LevelInfo, "sql.started",
		"operation", "migration.mysql_ddl",
		"query", statement,
		"parameters", telemetry.RedactParams(),
		"status", "started",
	)
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		durMS := float64(time.Since(started)) / float64(time.Millisecond)
		telemetry.Log(ctx, slog.LevelError, "sql.finished",
			"operation", "migration.mysql_ddl",
			"query", statement,
			"parameters", telemetry.RedactParams(),
			"duration_ms", durMS,
			"status", "failed",
			"error", telemetry.SanitizeValue(err.Error()),
			"sqlstate", telemetry.SQLState(err),
		)
		var serverError *mysql.MySQLError
		if errors.As(err, &serverError) {
			return fmt.Errorf("%w: MySQL schema change failed; inspect the schema before retrying (DDL is not transactionally rolled back): %v", apperror.ErrInvalidInput, err)
		}
		return fmt.Errorf("%w: MySQL schema change was not confirmed; inspect the target before retrying", apperror.ErrTransactionOutcomeUnknown)
	}
	telemetry.Log(ctx, slog.LevelInfo, "sql.finished",
		"operation", "migration.mysql_ddl",
		"query", statement,
		"parameters", telemetry.RedactParams(),
		"duration_ms", float64(time.Since(started))/float64(time.Millisecond),
		"status", "success",
	)
	telemetry.Log(ctx, slog.LevelInfo, "sql.transaction_commit",
		"operation", "migration.mysql_ddl",
		"status", "success",
	)
	return nil
}

func (r *Runner) EnsureCheckpointTable(ctx context.Context, databaseID string) error {
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return err
	}
	query := `CREATE TABLE IF NOT EXISTS _platform_migration_checkpoints (
		execution_id UUID NOT NULL,
		batch_number BIGINT NOT NULL,
		last_cursor TEXT NOT NULL,
		rows_affected BIGINT NOT NULL,
		status TEXT NOT NULL DEFAULT 'COMMITTED',
		committed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
		PRIMARY KEY (execution_id, batch_number)
	)`
	_, err = db.ExecContext(ctx, query)
	return err
}

func (r *Runner) InspectCheckpoints(ctx context.Context, databaseID, execID string) (int64, int64, string, error) {
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return 0, 0, "", err
	}
	_ = r.EnsureCheckpointTable(ctx, databaseID)

	var committedBatches, committedRows int64
	var lastCursor sql.NullString

	row := db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(rows_affected), 0), MAX(last_cursor)
		FROM _platform_migration_checkpoints
		WHERE execution_id = $1::uuid AND status = 'COMMITTED'
	`, execID)

	if err := row.Scan(&committedBatches, &committedRows, &lastCursor); err != nil {
		return 0, 0, "", err
	}
	return committedBatches, committedRows, lastCursor.String, nil
}

func (r *Runner) ExecuteBatch(
	ctx context.Context,
	databaseID, execID, sqlTemplate, attr string,
	batchSize int,
	startCursor string,
	onProgress func(batches, rows int64, cursor string),
) (int64, int64, string, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	if attr == "" {
		attr = "id"
	}
	if err := r.EnsureCheckpointTable(ctx, databaseID); err != nil {
		return 0, 0, "", err
	}

	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return 0, 0, "", err
	}

	committedBatches, committedRows, lastCursor, err := r.InspectCheckpoints(ctx, databaseID, execID)
	if err != nil {
		return 0, 0, "", err
	}
	if startCursor != "" && lastCursor == "" {
		lastCursor = startCursor
	}

	for {
		select {
		case <-ctx.Done():
			return committedBatches, committedRows, lastCursor, ctx.Err()
		default:
		}

		batchNum := int(committedBatches + 1)
		batchCtx := telemetry.WithBatch(ctx, batchNum)
		batchCtx, batchSpan := otel.Tracer("platform/migration").Start(batchCtx, "migration.batch")
		batchSpan.SetAttributes(
			attribute.String("database.id", databaseID),
			attribute.String("execution.id", execID),
			attribute.Int("migration.batch_number", batchNum),
		)
		batchStarted := time.Now()

		tx, err := db.BeginTx(batchCtx, nil)
		if err != nil {
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "batch begin failed")
			batchSpan.End()
			telemetry.IncBatchProcessed("forward", "failed", 0)
			return committedBatches, committedRows, lastCursor, err
		}

		batchSQL := sqlTemplate
		cursorVal := lastCursor
		if cursorVal == "" {
			cursorVal = "0"
		}

		if strings.Contains(batchSQL, "{{CURSOR}}") || strings.Contains(batchSQL, "{{BATCH_SIZE}}") {
			batchSQL = strings.ReplaceAll(batchSQL, "{{CURSOR}}", cursorVal)
			batchSQL = strings.ReplaceAll(batchSQL, "{{BATCH_SIZE}}", fmt.Sprintf("%d", batchSize))
			batchSQL = strings.ReplaceAll(batchSQL, "{{ATTRIBUTE}}", attr)
		}

		telemetry.Log(batchCtx, slog.LevelInfo, "sql.started",
			"operation", "migration.batch",
			"database_id", databaseID,
			"query", batchSQL,
			"parameters", telemetry.RedactParams(cursorVal, batchSize),
			"status", "started",
		)

		var rowsAffected int64
		nextCursor := cursorVal

		hasReturning := strings.Contains(strings.ToUpper(batchSQL), "RETURNING")
		querySQL := batchSQL
		if !hasReturning && attr != "" {
			trimmed := strings.TrimRight(strings.TrimSpace(batchSQL), ";")
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "UPDATE") || strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "INSERT") {
				querySQL = trimmed + " RETURNING " + attr
			}
		}

		qRows, qErr := tx.QueryContext(batchCtx, querySQL)
		var queryErr error
		if qErr != nil {
			// QueryContext has already executed the statement. Retrying it with
			// ExecContext in this transaction can only replace the original SQL
			// error with PostgreSQL's "current transaction is aborted" error.
			queryErr = qErr
		} else {
			for qRows.Next() {
				rowsAffected++
				var val any
				if err := qRows.Scan(&val); err != nil {
					queryErr = err
					break
				}
				if val != nil {
					nextCursor = fmt.Sprintf("%v", val)
				}
			}
			if rowsErr := qRows.Err(); queryErr == nil {
				queryErr = rowsErr
			}
			if closeErr := qRows.Close(); queryErr == nil {
				queryErr = closeErr
			}
		}
		if queryErr != nil {
			durMS := float64(time.Since(batchStarted)) / float64(time.Millisecond)
			batchSpan.RecordError(queryErr)
			batchSpan.SetStatus(codes.Error, "batch SQL failed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.finished",
				"operation", "migration.batch",
				"database_id", databaseID,
				"query", batchSQL,
				"parameters", telemetry.RedactParams(cursorVal, batchSize),
				"duration_ms", durMS,
				"rows", 0,
				"status", "failed",
				"error", telemetry.SanitizeValue(queryErr.Error()),
				"sqlstate", telemetry.SQLState(queryErr),
			)
			rbErr := tx.Rollback()
			rbStatus := "success"
			if rbErr != nil {
				rbStatus = "failed"
			}
			telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_rollback",
				"operation", "migration.batch",
				"database_id", databaseID,
				"status", rbStatus,
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("forward", "failed", 0)
			return committedBatches, committedRows, lastCursor, fmt.Errorf("batch execution failed: %w", queryErr)
		}

		durMS := float64(time.Since(batchStarted)) / float64(time.Millisecond)
		batchSpan.SetAttributes(attribute.Int64("db.rows", rowsAffected))
		telemetry.Log(batchCtx, slog.LevelInfo, "sql.finished",
			"operation", "migration.batch",
			"database_id", databaseID,
			"query", batchSQL,
			"parameters", telemetry.RedactParams(cursorVal, batchSize),
			"duration_ms", durMS,
			"rows", rowsAffected,
			"status", "success",
		)

		if rowsAffected == 0 {
			_ = tx.Rollback()
			telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_rollback",
				"operation", "migration.batch",
				"database_id", databaseID,
				"status", "success",
				"reason", "empty_batch",
			)
			batchSpan.End()
			break
		}

		committedBatches++
		committedRows += rowsAffected
		lastCursor = nextCursor

		_, err = tx.ExecContext(batchCtx, `
			INSERT INTO _platform_migration_checkpoints (execution_id, batch_number, last_cursor, rows_affected, status)
			VALUES ($1::uuid, $2, $3, $4, 'COMMITTED')
		`, execID, committedBatches, nextCursor, rowsAffected)
		if err != nil {
			_ = tx.Rollback()
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "checkpoint save failed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.transaction_rollback",
				"operation", "migration.batch_checkpoint",
				"database_id", databaseID,
				"status", "success",
				"error", telemetry.SanitizeValue(err.Error()),
				"sqlstate", telemetry.SQLState(err),
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("forward", "failed", 0)
			return committedBatches - 1, committedRows - rowsAffected, lastCursor, fmt.Errorf("checkpoint save failed: %w", err)
		}

		if err := tx.Commit(); err != nil {
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "batch commit unconfirmed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.transaction_commit",
				"operation", "migration.batch",
				"database_id", databaseID,
				"status", "failed",
				"error", telemetry.SanitizeValue(err.Error()),
				"sqlstate", telemetry.SQLState(err),
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("forward", "failed", 0)
			return committedBatches - 1, committedRows - rowsAffected, lastCursor, fmt.Errorf("%w: batch commit unconfirmed", apperror.ErrTransactionOutcomeUnknown)
		}

		telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_commit",
			"operation", "migration.batch",
			"database_id", databaseID,
			"rows", rowsAffected,
			"status", "success",
		)
		batchSpan.End()
		telemetry.IncBatchProcessed("forward", "committed", rowsAffected)

		if onProgress != nil {
			onProgress(committedBatches, committedRows, nextCursor)
		}

		if int(rowsAffected) < batchSize {
			break
		}
	}

	return committedBatches, committedRows, lastCursor, nil
}

func (r *Runner) RevertBatch(
	ctx context.Context,
	databaseID, execID, sqlDownTemplate, attr string,
	batchSize int,
	onProgress func(batches, rows int64, cursor string),
) (int64, int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	if attr == "" {
		attr = "id"
	}
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return 0, 0, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT batch_number, last_cursor, rows_affected
		FROM _platform_migration_checkpoints
		WHERE execution_id = $1::uuid AND status = 'COMMITTED'
		ORDER BY batch_number DESC
	`, execID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	type batchInfo struct {
		num    int64
		cursor string
		rows   int64
	}
	var batches []batchInfo
	for rows.Next() {
		var b batchInfo
		if err := rows.Scan(&b.num, &b.cursor, &b.rows); err == nil {
			batches = append(batches, b)
		}
	}

	var revertedBatches, revertedRows int64
	for _, b := range batches {
		batchCtx := telemetry.WithBatch(ctx, int(b.num))
		batchCtx, batchSpan := otel.Tracer("platform/migration").Start(batchCtx, "migration.revert_batch")
		batchSpan.SetAttributes(
			attribute.String("database.id", databaseID),
			attribute.String("execution.id", execID),
			attribute.Int64("migration.batch_number", b.num),
		)
		batchStarted := time.Now()

		tx, err := db.BeginTx(batchCtx, nil)
		if err != nil {
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "revert begin failed")
			batchSpan.End()
			telemetry.IncBatchProcessed("rollback", "failed", 0)
			return revertedBatches, revertedRows, err
		}

		downSQL := sqlDownTemplate
		if strings.Contains(downSQL, "{{CURSOR}}") || strings.Contains(downSQL, "{{BATCH_SIZE}}") {
			downSQL = strings.ReplaceAll(downSQL, "{{CURSOR}}", b.cursor)
			downSQL = strings.ReplaceAll(downSQL, "{{BATCH_SIZE}}", fmt.Sprintf("%d", batchSize))
			downSQL = strings.ReplaceAll(downSQL, "{{ATTRIBUTE}}", attr)
		}

		telemetry.Log(batchCtx, slog.LevelInfo, "sql.started",
			"operation", "migration.revert_batch",
			"database_id", databaseID,
			"query", downSQL,
			"parameters", telemetry.RedactParams(b.cursor),
			"status", "started",
		)

		if _, err := tx.ExecContext(batchCtx, downSQL); err != nil {
			durMS := float64(time.Since(batchStarted)) / float64(time.Millisecond)
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "revert SQL failed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.finished",
				"operation", "migration.revert_batch",
				"database_id", databaseID,
				"query", downSQL,
				"parameters", telemetry.RedactParams(b.cursor),
				"duration_ms", durMS,
				"status", "failed",
				"error", telemetry.SanitizeValue(err.Error()),
				"sqlstate", telemetry.SQLState(err),
			)
			_ = tx.Rollback()
			telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_rollback",
				"operation", "migration.revert_batch",
				"database_id", databaseID,
				"status", "success",
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("rollback", "failed", 0)
			return revertedBatches, revertedRows, fmt.Errorf("revert batch %d failed: %w", b.num, err)
		}

		durMS := float64(time.Since(batchStarted)) / float64(time.Millisecond)
		batchSpan.SetAttributes(attribute.Int64("db.rows", b.rows))
		telemetry.Log(batchCtx, slog.LevelInfo, "sql.finished",
			"operation", "migration.revert_batch",
			"database_id", databaseID,
			"query", downSQL,
			"parameters", telemetry.RedactParams(b.cursor),
			"duration_ms", durMS,
			"rows", b.rows,
			"status", "success",
		)

		_, err = tx.ExecContext(batchCtx, `
			UPDATE _platform_migration_checkpoints
			SET status = 'REVERTED'
			WHERE execution_id = $1::uuid AND batch_number = $2
		`, execID, b.num)
		if err != nil {
			_ = tx.Rollback()
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "revert checkpoint update failed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.transaction_rollback",
				"operation", "migration.revert_batch_checkpoint",
				"database_id", databaseID,
				"status", "success",
				"error", telemetry.SanitizeValue(err.Error()),
				"sqlstate", telemetry.SQLState(err),
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("rollback", "failed", 0)
			return revertedBatches, revertedRows, fmt.Errorf("update checkpoint revert failed: %w", err)
		}

		if err := tx.Commit(); err != nil {
			batchSpan.RecordError(err)
			batchSpan.SetStatus(codes.Error, "revert commit unconfirmed")
			telemetry.Log(batchCtx, slog.LevelError, "sql.transaction_commit",
				"operation", "migration.revert_batch",
				"database_id", databaseID,
				"status", "failed",
				"error", telemetry.SanitizeValue(err.Error()),
				"sqlstate", telemetry.SQLState(err),
			)
			batchSpan.End()
			telemetry.IncBatchProcessed("rollback", "failed", 0)
			return revertedBatches, revertedRows, fmt.Errorf("%w: revert commit unconfirmed", apperror.ErrTransactionOutcomeUnknown)
		}

		telemetry.Log(batchCtx, slog.LevelInfo, "sql.transaction_commit",
			"operation", "migration.revert_batch",
			"database_id", databaseID,
			"rows", b.rows,
			"status", "success",
		)
		batchSpan.End()
		telemetry.IncBatchProcessed("rollback", "committed", b.rows)

		revertedBatches++
		revertedRows += b.rows
		if onProgress != nil {
			onProgress(revertedBatches, revertedRows, b.cursor)
		}
	}

	return revertedBatches, revertedRows, nil
}
