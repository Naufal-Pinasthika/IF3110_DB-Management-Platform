package migration

import (
	"context"
	"database/sql"
	"errors"
	"hash/crc32"
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
	var migrations []models.Migration
	query := r.DB.WithContext(ctx).Where("database_id = ?", databaseID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Order applied history by version (nulls last keeps drafts at the end) and created_at
	err := query.Order("version ASC NULLS LAST, created_at ASC").Find(&migrations).Error
	return migrations, err
}

// GetMigration loads a migration only within its target.
func (r *PostgresRepository) GetMigration(ctx context.Context, databaseID, migrationID string) (models.Migration, error) {
	var migration models.Migration
	err := r.DB.WithContext(ctx).
		Where("database_id = ? AND id = ?", databaseID, migrationID).
		First(&migration).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migration, apperror.ErrNotFound
	}
	return migration, err
}

// CreateMigration persists an unversioned draft.
func (r *PostgresRepository) CreateMigration(ctx context.Context, migration models.Migration) (models.Migration, error) {
	migration.Status = models.MigrationDraft
	migration.Version = nil
	err := r.DB.WithContext(ctx).Create(&migration).Error
	return migration, err
}

// UpdateMigration updates SQL only while the record is a draft.
func (r *PostgresRepository) UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationDraft).
		Updates(map[string]interface{}{
			"sql_text":   sqlText,
			"updated_at": now,
		})

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}

	return r.GetMigration(ctx, databaseID, migrationID)
}

// LockDatabase serializes execution and version allocation for a target.
func (r *PostgresRepository) LockDatabase(ctx context.Context, databaseID string) (func() error, error) {
	sqlDB, err := r.DB.DB()
	if err != nil {
		return nil, err
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}

	// Generate a deterministic 32-bit lock ID from the database UUID
	lockID := int32(crc32.ChecksumIEEE([]byte(databaseID)))

	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID)
	if err != nil {
		conn.Close()
		return nil, err
	}

	release := func() error {
		defer conn.Close()
		_, unlockErr := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
		return unlockErr
	}

	return release, nil
}

// MarkRunning claims the exact draft SQL that will be executed.
func (r *PostgresRepository) MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationDraft).
		Updates(map[string]interface{}{
			"status":      models.MigrationRunning,
			"executed_by": executorID,
			"updated_at":  now,
		})

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}

	return r.GetMigration(ctx, databaseID, migrationID)
}

// MarkApplied records a confirmed target commit with the next version.
func (r *PostgresRepository) MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	var maxVersion sql.NullInt64
	r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND status = ?", databaseID, models.MigrationApplied).
		Select("MAX(version)").Scan(&maxVersion)

	nextVersion := int64(1)
	if maxVersion.Valid {
		nextVersion = maxVersion.Int64 + 1
	}

	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationRunning).
		Updates(map[string]interface{}{
			"status":      models.MigrationApplied,
			"version":     nextVersion,
			"executed_by": executorID,
			"executed_at": now,
			"updated_at":  now,
		})

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}

	return r.GetMigration(ctx, databaseID, migrationID)
}

// MarkFailed records a failure only after rollback is confirmed.
func (r *PostgresRepository) MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationRunning).
		Updates(map[string]interface{}{
			"status":        models.MigrationFailed,
			"error_summary": errorSummary,
			"executed_by":   executorID,
			"executed_at":   now,
			"updated_at":    now,
			// version remains NULL as it's not included in the map
		})

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}
	
	return r.GetMigration(ctx, databaseID, migrationID)
}