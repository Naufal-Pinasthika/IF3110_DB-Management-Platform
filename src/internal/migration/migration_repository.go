package migration

import (
	"context"
	"database/sql"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

type PostgresRepository struct {
	DB *sql.DB
}

var _ Repository = (*PostgresRepository)(nil)

// ListMigrations reads history scoped to a target.
func (r *PostgresRepository) ListMigrations(ctx context.Context, databaseID string, status api.MigrationStatus) ([]api.MigrationSummary, error) {
	// TODO Filter by database ID and optional status with bound values.
	// TODO Order applied history by version and document draft and failed ordering.
	return nil, apperror.ErrNotImplemented
}

// GetMigration loads a migration only within its target.
func (r *PostgresRepository) GetMigration(ctx context.Context, databaseID, migrationID string) (api.MigrationDetail, error) {
	// TODO Use both IDs in the query to prevent cross database object access.
	return api.MigrationDetail{}, apperror.ErrNotImplemented
}

// CreateMigration persists an unversioned draft.
func (r *PostgresRepository) CreateMigration(ctx context.Context, databaseID, creatorID string, input api.CreateMigrationRequest, now time.Time) (api.MigrationCreated, error) {
	// TODO Insert SQL and creator and timestamps with status DRAFT and version NULL.
	return api.MigrationCreated{}, apperror.ErrNotImplemented
}

// UpdateMigration updates SQL only while the record is a draft.
func (r *PostgresRepository) UpdateMigration(ctx context.Context, databaseID, migrationID string, input api.UpdateMigrationRequest, now time.Time) (api.MigrationUpdated, error) {
	// TODO Use both IDs and status DRAFT in an atomic update and update updated_at.
	// TODO Report a conflict for RUNNING or APPLIED or FAILED records.
	return api.MigrationUpdated{}, apperror.ErrNotImplemented
}

// LockDatabase serializes execution and version allocation for a target.
func (r *PostgresRepository) LockDatabase(ctx context.Context, databaseID string) (func() error, error) {
	// TODO Hold a PostgreSQL advisory lock on a dedicated metadata connection.
	// TODO Return a release function and keep ownership until final history recording.
	// TODO A per database mutex is an alternative only for one documented server instance.
	return nil, apperror.ErrNotImplemented
}

// MarkRunning claims the exact draft SQL that will be executed.
func (r *PostgresRepository) MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (api.MigrationDetail, error) {
	// TODO Atomically change DRAFT to RUNNING using both IDs and retain version NULL.
	// TODO Record the executor and return the claimed SQL to prevent concurrent draft edits.
	// TODO Report a conflict if another request already claimed the draft.
	return api.MigrationDetail{}, apperror.ErrNotImplemented
}

// MarkApplied records a confirmed target commit with the next version.
func (r *PostgresRepository) MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (api.MigrationExecuted, error) {
	// TODO While holding the database lock allocate the next successful version starting at 1.
	// TODO Update RUNNING to APPLIED with version and executor and completion time atomically.
	// TODO Keep version uniqueness per database using the existing constraint.
	return api.MigrationExecuted{}, apperror.ErrNotImplemented
}

// MarkFailed records a failure only after rollback is confirmed.
func (r *PostgresRepository) MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (api.MigrationExecuted, error) {
	// TODO Update RUNNING to FAILED and keep version NULL with executor and completion time.
	// TODO Store a sanitized summary and keep the original SQL immutable.
	return api.MigrationExecuted{}, apperror.ErrNotImplemented
}
