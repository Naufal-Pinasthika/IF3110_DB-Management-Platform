package migration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)


type Runner struct {
	Targets *store.TargetStore
	Timeout time.Duration
}


var _ SQLRunner = (*Runner)(nil)


// Execute runs every migration statement in one target transaction.
func (r *Runner) Execute(ctx context.Context, databaseID, sqlText string) error {
	upperSQL := strings.ToUpper(sqlText)
	if strings.Contains(upperSQL, "BEGIN") || strings.Contains(upperSQL, "COMMIT") || strings.Contains(upperSQL, "ROLLBACK") {
		return fmt.Errorf("%w: manual transaction control is forbidden", apperror.ErrInvalidInput)
	}
	if strings.Contains(upperSQL, "CREATE INDEX CONCURRENTLY") || strings.Contains(upperSQL, "CREATE DATABASE") {
		return fmt.Errorf("%w: unsupported statement in transaction", apperror.ErrInvalidInput)
	}

	// Target migrator connection
	migratorDB, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("migrator connection failed: %w", err)
	}
	// Pool should typically be managed by TargetStore, we just use the borrowed connection/pool.

	txCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	tx, err := migratorDB.BeginTx(txCtx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Executing everything as one block
	if _, err := tx.ExecContext(txCtx, sqlText); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("execute failed: %v, rollback also failed: %w", err, rbErr)
		}
		return fmt.Errorf("%w: migration failed and safely rolled back: %v", apperror.ErrInvalidInput, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}