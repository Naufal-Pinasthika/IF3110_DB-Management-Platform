package migration

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

// ListMigrations reads history scoped to a target.
func (r *PostgresRepository) ListMigrations(ctx context.Context, databaseID string, status models.MigrationStatus) ([]models.Migration, error) {
	// TODO Filter by database ID and optional status with bound values.
	// TODO Order applied history by version and document draft and failed ordering.
	return nil, apperror.ErrNotImplemented
}

// GetMigration loads a migration only within its target.
func (r *PostgresRepository) GetMigration(ctx context.Context, databaseID, migrationID string) (models.Migration, error) {
	// TODO Use both IDs in the query to prevent cross database object access.
	return models.Migration{}, apperror.ErrNotImplemented
}

// CreateMigration persists an unversioned draft.
func (r *PostgresRepository) CreateMigration(ctx context.Context, migration models.Migration) (models.Migration, error) {
	// TODO Create models.Migration with DRAFT status and a nil Version and supplied creator and times.
	return models.Migration{}, apperror.ErrNotImplemented
}

// UpdateMigration updates SQL only while the record is a draft.
func (r *PostgresRepository) UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error) {
	// TODO Update selected model fields with GORM using both IDs and status DRAFT.
	// TODO Check RowsAffected and update updated_at without using Save upserts.
	// TODO Report a conflict for RUNNING or APPLIED or FAILED records.
	return models.Migration{}, apperror.ErrNotImplemented
}

// LockDatabase serializes execution and version allocation for a target.
func (r *PostgresRepository) LockDatabase(ctx context.Context, databaseID string) (func() error, error) {
	// TODO Pin a dedicated underlying sql.Conn and hold its PostgreSQL advisory lock.
	// TODO Reserve enough pool capacity for history queries while the lock connection is held.
	// TODO Return a release function and keep ownership until final history recording.
	// TODO A per database mutex is an alternative only for one documented server instance.
	return nil, apperror.ErrNotImplemented
}

// MarkRunning claims the exact draft SQL that will be executed.
func (r *PostgresRepository) MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	// TODO Atomically change DRAFT to RUNNING using both IDs and retain version NULL.
	// TODO Record the executor and return the claimed SQL to prevent concurrent draft edits.
	// TODO Report a conflict if another request already claimed the draft.
	return models.Migration{}, apperror.ErrNotImplemented
}

// MarkApplied records a confirmed target commit with the next version.
func (r *PostgresRepository) MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	// TODO While holding the database lock allocate the next successful version starting at 1.
	// TODO Update RUNNING to APPLIED with version and executor and completion time atomically.
	// TODO Keep version uniqueness per database using the existing constraint.
	return models.Migration{}, apperror.ErrNotImplemented
}

// MarkFailed records a failure only after rollback is confirmed.
func (r *PostgresRepository) MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error) {
	// TODO Update RUNNING to FAILED and keep version NULL with executor and completion time.
	// TODO Use an update map to preserve NULL version and store a sanitized error summary.
	return models.Migration{}, apperror.ErrNotImplemented
}
