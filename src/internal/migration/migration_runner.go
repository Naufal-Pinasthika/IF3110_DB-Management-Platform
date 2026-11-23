package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/sqlguard"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/go-sql-driver/mysql"
	pgquery "github.com/pganalyze/pg_query_go/v5"
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
		// Parse transaction commands, including END/ABORT, instead of searching comments/literals.
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
	if plan.DDL {
		return executeMySQLDDL(ctx, db, statements[0])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if config.TargetEngine() == models.EngineMySQL {
		// Nontransactional tables would silently violate the rollback guarantee.
		for _, table := range plan.Tables {
			var engine sql.NullString
			err := tx.QueryRowContext(ctx, "SELECT engine FROM information_schema.tables WHERE table_schema=? AND table_name=?", config.DatabaseName, table).Scan(&engine)
			if err != nil || !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
				return fmt.Errorf("%w: DML migrations require existing InnoDB tables", apperror.ErrInvalidInput)
			}
		}
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("%w: execution failed and rollback was not confirmed", apperror.ErrTransactionOutcomeUnknown)
			}
			return fmt.Errorf("%w: migration failed and safely rolled back: %v", apperror.ErrInvalidInput, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: target commit was not confirmed", apperror.ErrTransactionOutcomeUnknown)
	}
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
	// One DDL statement, outside a transaction: MySQL implicitly commits schema changes.
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		var serverError *mysql.MySQLError
		if errors.As(err, &serverError) {
			return fmt.Errorf("%w: MySQL schema change failed; inspect the schema before retrying (DDL is not transactionally rolled back): %v", apperror.ErrInvalidInput, err)
		}
		return fmt.Errorf("%w: MySQL schema change was not confirmed; inspect the target before retrying", apperror.ErrTransactionOutcomeUnknown)
	}
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

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
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

		var rowsAffected int64
		nextCursor := cursorVal

		// Attempt RETURNING to accurately track cursor progression across batches
		hasReturning := strings.Contains(strings.ToUpper(batchSQL), "RETURNING")
		querySQL := batchSQL
		if !hasReturning && attr != "" {
			trimmed := strings.TrimRight(strings.TrimSpace(batchSQL), ";")
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "UPDATE") || strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "INSERT") {
				querySQL = trimmed + " RETURNING " + attr
			}
		}

		qRows, qErr := tx.QueryContext(ctx, querySQL)
		if qErr == nil {
			for qRows.Next() {
				rowsAffected++
				var val any
				if err := qRows.Scan(&val); err == nil && val != nil {
					nextCursor = fmt.Sprintf("%v", val)
				}
			}
			_ = qRows.Close()
		} else {
			// Fallback to direct ExecContext if RETURNING is not supported by statement structure
			res, err := tx.ExecContext(ctx, batchSQL)
			if err != nil {
				_ = tx.Rollback()
				return committedBatches, committedRows, lastCursor, fmt.Errorf("batch execution failed: %w", err)
			}
			rowsAffected, _ = res.RowsAffected()
			// If cursor couldn't be scanned, attempt integer increment fallback
			if rowsAffected > 0 {
				var intCursor int64
				if _, err := fmt.Sscanf(cursorVal, "%d", &intCursor); err == nil {
					nextCursor = fmt.Sprintf("%d", intCursor+rowsAffected)
				}
			}
		}

		if rowsAffected == 0 {
			_ = tx.Rollback()
			break
		}

		committedBatches++
		committedRows += rowsAffected
		lastCursor = nextCursor

		_, err = tx.ExecContext(ctx, `
			INSERT INTO _platform_migration_checkpoints (execution_id, batch_number, last_cursor, rows_affected, status)
			VALUES ($1::uuid, $2, $3, $4, 'COMMITTED')
		`, execID, committedBatches, nextCursor, rowsAffected)
		if err != nil {
			_ = tx.Rollback()
			return committedBatches - 1, committedRows - rowsAffected, lastCursor, fmt.Errorf("checkpoint save failed: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return committedBatches - 1, committedRows - rowsAffected, lastCursor, fmt.Errorf("%w: batch commit unconfirmed", apperror.ErrTransactionOutcomeUnknown)
		}

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
	onProgress func(batches, rows int64, cursor string),
) (int64, int64, error) {
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
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return revertedBatches, revertedRows, err
		}

		downSQL := sqlDownTemplate
		if strings.Contains(downSQL, "{{CURSOR}}") {
			downSQL = strings.ReplaceAll(downSQL, "{{CURSOR}}", b.cursor)
			downSQL = strings.ReplaceAll(downSQL, "{{ATTRIBUTE}}", attr)
		}

		if _, err := tx.ExecContext(ctx, downSQL); err != nil {
			_ = tx.Rollback()
			return revertedBatches, revertedRows, fmt.Errorf("revert batch %d failed: %w", b.num, err)
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE _platform_migration_checkpoints
			SET status = 'REVERTED'
			WHERE execution_id = $1::uuid AND batch_number = $2
		`, execID, b.num)
		if err != nil {
			_ = tx.Rollback()
			return revertedBatches, revertedRows, fmt.Errorf("update checkpoint revert failed: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return revertedBatches, revertedRows, fmt.Errorf("%w: revert commit unconfirmed", apperror.ErrTransactionOutcomeUnknown)
		}

		revertedBatches++
		revertedRows += b.rows
		if onProgress != nil {
			onProgress(revertedBatches, revertedRows, b.cursor)
		}
	}

	return revertedBatches, revertedRows, nil
}
