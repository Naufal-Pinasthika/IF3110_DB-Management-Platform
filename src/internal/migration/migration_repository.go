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

type Repository interface {
	ListMigrations(ctx context.Context, databaseID string, status models.MigrationStatus) ([]models.Migration, error)
	GetMigration(ctx context.Context, databaseID, migrationID string) (models.Migration, error)
	GetMigrationWithApprovals(ctx context.Context, databaseID, migrationID string) (models.Migration, []models.MigrationApproval, error)
	CreateMigration(ctx context.Context, migration models.Migration) (models.Migration, error)
	UpdateMigrationFields(ctx context.Context, databaseID, migrationID string, updates map[string]any, now time.Time) (models.Migration, error)
	UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error)
	LockDatabase(ctx context.Context, databaseID string) (func() error, error)
	MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error)
	MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error)
	MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error)
	MarkRolledBack(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error)
	RestoreApplied(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error)
	SubmitForReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error
	SubmitRollbackReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error
	SubmitForwardReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error
	GetPendingApprovalForLeader(ctx context.Context, migrationID, leaderID string) ([]models.MigrationApproval, error)
	UpdateApprovalStatus(ctx context.Context, approvalID string, status models.ApprovalStatus, comment *string, decidedAt time.Time) error
	CheckAndSetApproved(ctx context.Context, databaseID, migrationID string) (bool, error)
	CheckAndSetRollbackApproved(ctx context.Context, databaseID, migrationID string) (bool, error)
	RejectMigration(ctx context.Context, databaseID, migrationID string) error
	RejectRollbackReview(ctx context.Context, databaseID, migrationID string) error
	RejectForwardReview(ctx context.Context, databaseID, migrationID string) error
	GetDatabaseGroupsWithLeaders(ctx context.Context, databaseID string) ([]models.Group, error)
	GetLastAppliedMigration(ctx context.Context, databaseID string) (*models.Migration, error)
	GetNextForwardCandidate(ctx context.Context, databaseID string) (*models.Migration, error)
	CreateExecution(ctx context.Context, exec *models.MigrationExecution) error
	GetExecution(ctx context.Context, executionID string) (*models.MigrationExecution, error)
	UpdateExecution(ctx context.Context, exec *models.MigrationExecution) error
	ListExecutions(ctx context.Context, databaseID string, hideRollback bool) ([]models.MigrationExecution, error)
}

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ListMigrations(ctx context.Context, databaseID string, status models.MigrationStatus) ([]models.Migration, error) {
	var migrations []models.Migration
	query := r.DB.WithContext(ctx).Where("database_id = ?", databaseID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("version ASC NULLS LAST, created_at ASC").Find(&migrations).Error
	return migrations, err
}

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

func (r *PostgresRepository) GetMigrationWithApprovals(ctx context.Context, databaseID, migrationID string) (models.Migration, []models.MigrationApproval, error) {
	m, err := r.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return m, nil, err
	}

	_ = r.DB.WithContext(ctx).Exec(`
		UPDATE migration_approvals ma
		SET leader_id = g.leader_id
		FROM groups g
		WHERE ma.migration_id = ? 
		  AND ma.group_id = g.id 
		  AND ma.status = 'PENDING' 
		  AND g.leader_id IS NOT NULL 
		  AND ma.leader_id IS DISTINCT FROM g.leader_id
	`, migrationID).Error

	var approvals []models.MigrationApproval
	err = r.DB.WithContext(ctx).
		Preload("Group").
		Preload("Leader").
		Where("migration_id = ?", migrationID).
		Order("created_at ASC").
		Find(&approvals).Error

	return m, approvals, err
}

func (r *PostgresRepository) CreateMigration(ctx context.Context, migration models.Migration) (models.Migration, error) {
	migration.Status = models.MigrationDraft
	migration.Version = nil
	err := r.DB.WithContext(ctx).Create(&migration).Error
	return migration, err
}

func (r *PostgresRepository) UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error) {
	return r.UpdateMigrationFields(ctx, databaseID, migrationID, map[string]any{"sql_text": sqlText}, now)
}

func (r *PostgresRepository) UpdateMigrationFields(ctx context.Context, databaseID, migrationID string, updates map[string]any, now time.Time) (models.Migration, error) {
	updates["updated_at"] = now

	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status IN (?, ?)", databaseID, migrationID, models.MigrationDraft, models.MigrationRejected).
		Updates(updates)

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}

	return r.GetMigration(ctx, databaseID, migrationID)
}

func (r *PostgresRepository) LockDatabase(ctx context.Context, databaseID string) (func() error, error) {
	sqlDB, err := r.DB.DB()
	if err != nil {
		return nil, err
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}

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

func (r *PostgresRepository) MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status IN (?, ?, ?, ?)", databaseID, migrationID, models.MigrationDraft, models.MigrationApproved, models.MigrationRollbackApproved, models.MigrationQueued).
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

func (r *PostgresRepository) MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	var maxVersion sql.NullInt64
	r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND version IS NOT NULL", databaseID).
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

func (r *PostgresRepository) MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationRunning).
		Updates(map[string]interface{}{
			"status":        models.MigrationFailed,
			"error_summary": errorSummary,
			"executed_by":   executorID,
			"executed_at":   now,
			"updated_at":    now,
		})

	if result.Error != nil {
		return models.Migration{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.Migration{}, apperror.ErrConflict
	}

	return r.GetMigration(ctx, databaseID, migrationID)
}

func (r *PostgresRepository) MarkRolledBack(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ?", databaseID, migrationID).
		Updates(map[string]interface{}{
			"status":     models.MigrationRolledBack,
			"version":    nil,
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

// RestoreApplied restores migration status back to APPLIED when a rollback attempt fails cleanly.
func (r *PostgresRepository) RestoreApplied(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error) {
	result := r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ?", databaseID, migrationID).
		Updates(map[string]interface{}{
			"status":     models.MigrationApplied,
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

func (r *PostgresRepository) submitReviewFromStatuses(ctx context.Context, databaseID, migrationID string, allowedStatuses []models.MigrationStatus, approvals []models.MigrationApproval) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Migration{}).
			Where("database_id = ? AND id = ? AND status IN ?", databaseID, migrationID, allowedStatuses).
			Update("status", models.MigrationPendingReview)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return apperror.ErrConflict
		}

		if err := tx.Where("migration_id = ?", migrationID).Delete(&models.MigrationApproval{}).Error; err != nil {
			return err
		}
		if len(approvals) > 0 {
			if err := tx.Create(&approvals).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *PostgresRepository) SubmitForReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	return r.submitReviewFromStatuses(ctx, databaseID, migrationID, []models.MigrationStatus{models.MigrationDraft, models.MigrationRejected}, approvals)
}

func (r *PostgresRepository) SubmitRollbackReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	return r.submitReviewFromStatuses(ctx, databaseID, migrationID, []models.MigrationStatus{models.MigrationApplied}, approvals)
}

func (r *PostgresRepository) SubmitForwardReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	return r.submitReviewFromStatuses(ctx, databaseID, migrationID, []models.MigrationStatus{models.MigrationRolledBack}, approvals)
}

func (r *PostgresRepository) GetPendingApprovalForLeader(ctx context.Context, migrationID, leaderID string) ([]models.MigrationApproval, error) {
	_ = r.DB.WithContext(ctx).Exec(`
		UPDATE migration_approvals ma
		SET leader_id = g.leader_id
		FROM groups g
		WHERE ma.migration_id = ? 
		  AND ma.group_id = g.id 
		  AND ma.status = 'PENDING' 
		  AND g.leader_id IS NOT NULL 
		  AND ma.leader_id IS DISTINCT FROM g.leader_id
	`, migrationID).Error

	var approvals []models.MigrationApproval
	err := r.DB.WithContext(ctx).
		Where("migration_id = ? AND leader_id = ? AND status = ?", migrationID, leaderID, models.ApprovalPending).
		Find(&approvals).Error
	return approvals, err
}

func (r *PostgresRepository) UpdateApprovalStatus(ctx context.Context, approvalID string, status models.ApprovalStatus, comment *string, decidedAt time.Time) error {
	updates := map[string]any{
		"status":     status,
		"comment":    comment,
		"decided_at": decidedAt,
	}
	return r.DB.WithContext(ctx).Model(&models.MigrationApproval{}).
		Where("id = ?", approvalID).
		Updates(updates).Error
}

func (r *PostgresRepository) checkAndSetReviewStatus(ctx context.Context, databaseID, migrationID string, targetStatus models.MigrationStatus) (bool, error) {
	var totalCount int64
	if err := r.DB.WithContext(ctx).Model(&models.MigrationApproval{}).
		Where("migration_id = ?", migrationID).
		Count(&totalCount).Error; err != nil {
		return false, err
	}
	if totalCount == 0 {
		return false, nil
	}

	var pendingCount int64
	if err := r.DB.WithContext(ctx).Model(&models.MigrationApproval{}).
		Where("migration_id = ? AND status != ?", migrationID, models.ApprovalApproved).
		Count(&pendingCount).Error; err != nil {
		return false, err
	}

	if pendingCount == 0 {
		err := r.DB.WithContext(ctx).Model(&models.Migration{}).
			Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationPendingReview).
			Update("status", targetStatus).Error
		return true, err
	}
	return false, nil
}

func (r *PostgresRepository) CheckAndSetApproved(ctx context.Context, databaseID, migrationID string) (bool, error) {
	return r.checkAndSetReviewStatus(ctx, databaseID, migrationID, models.MigrationApproved)
}

func (r *PostgresRepository) CheckAndSetRollbackApproved(ctx context.Context, databaseID, migrationID string) (bool, error) {
	return r.checkAndSetReviewStatus(ctx, databaseID, migrationID, models.MigrationRollbackApproved)
}

func (r *PostgresRepository) RejectMigration(ctx context.Context, databaseID, migrationID string) error {
	return r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationPendingReview).
		Update("status", models.MigrationRejected).Error
}

func (r *PostgresRepository) RejectRollbackReview(ctx context.Context, databaseID, migrationID string) error {
	return r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationPendingReview).
		Update("status", models.MigrationApplied).Error
}

func (r *PostgresRepository) RejectForwardReview(ctx context.Context, databaseID, migrationID string) error {
	return r.DB.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND id = ? AND status = ?", databaseID, migrationID, models.MigrationPendingReview).
		Update("status", models.MigrationRolledBack).Error
}

func (r *PostgresRepository) GetDatabaseGroupsWithLeaders(ctx context.Context, databaseID string) ([]models.Group, error) {
	var groups []models.Group
	err := r.DB.WithContext(ctx).
		Table("groups").
		Select("DISTINCT groups.*").
		Joins("INNER JOIN group_access ON group_access.group_id = groups.id").
		Where("group_access.database_id = ?", databaseID).
		Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *PostgresRepository) GetLastAppliedMigration(ctx context.Context, databaseID string) (*models.Migration, error) {
	var m models.Migration
	err := r.DB.WithContext(ctx).
		Where("database_id = ? AND version IS NOT NULL", databaseID).
		Order("version DESC").
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *PostgresRepository) GetNextForwardCandidate(ctx context.Context, databaseID string) (*models.Migration, error) {
	var m models.Migration
	err := r.DB.WithContext(ctx).
		Where("database_id = ? AND version IS NULL AND status IN (?, ?, ?)", databaseID, models.MigrationApproved, models.MigrationRolledBack, models.MigrationPendingReview).
		Order("created_at ASC").
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *PostgresRepository) CreateExecution(ctx context.Context, exec *models.MigrationExecution) error {
	return r.DB.WithContext(ctx).Create(exec).Error
}

func (r *PostgresRepository) GetExecution(ctx context.Context, executionID string) (*models.MigrationExecution, error) {
	var exec models.MigrationExecution
	err := r.DB.WithContext(ctx).Where("id = ?", executionID).First(&exec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return &exec, nil
}

func (r *PostgresRepository) UpdateExecution(ctx context.Context, exec *models.MigrationExecution) error {
	return r.DB.WithContext(ctx).Save(exec).Error
}

// FinishExecutionIfRunning closes an execution only if it is still awaiting
// queue acceptance. This avoids a late enqueue error overwriting a worker's
// already recorded result.
func (r *PostgresRepository) FinishExecutionIfRunning(ctx context.Context, executionID, status, message string, finishedAt time.Time) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&models.MigrationExecution{}).
		Where("id = ? AND status = ?", executionID, "RUNNING").
		Updates(map[string]any{"status": status, "error_summary": message, "finished_at": finishedAt})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *PostgresRepository) ListExecutions(ctx context.Context, databaseID string, hideRollback bool) ([]models.MigrationExecution, error) {
	query := r.DB.WithContext(ctx).
		Preload("Migration").
		Preload("Executor").
		Where("database_id = ?", databaseID)

	if hideRollback {
		query = query.Where(`id NOT IN (
			SELECT rolled_back_execution_id 
			FROM migration_executions 
			WHERE database_id = ? AND action_type = 'ROLLBACK' AND status = 'SUCCEEDED' AND rolled_back_execution_id IS NOT NULL
		)`, databaseID).Where(`id NOT IN (
			SELECT id 
			FROM migration_executions 
			WHERE database_id = ? AND action_type = 'ROLLBACK' AND status = 'SUCCEEDED' AND rolled_back_execution_id IS NOT NULL
		)`, databaseID)
	}

	var executions []models.MigrationExecution
	err := query.Order("started_at ASC").Find(&executions).Error
	return executions, err
}
