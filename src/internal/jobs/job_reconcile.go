package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReconcileOutcome string

const (
	ReconcileApplied    ReconcileOutcome = "APPLIED"
	ReconcileNotApplied ReconcileOutcome = "NOT_APPLIED"
)

// ReconcileExecution stores an explicit operator decision after target state
// was inspected. It only accepts UNKNOWN, blocked jobs and keeps an earlier
// partial execution blocked when a retry/compensation is confirmed unapplied.
func (r *PostgresRepository) ReconcileExecution(ctx context.Context, databaseID, executionID string, outcome ReconcileOutcome) (Job, error) {
	if outcome != ReconcileApplied && outcome != ReconcileNotApplied {
		return Job{}, apperror.ErrInvalidInput
	}
	var reconciled Job
	err := r.transaction(ctx, func(tx *PostgresRepository) error {
		gate, err := tx.lockGate(ctx, databaseID)
		if err != nil {
			return err
		}
		job, err := tx.GetByExecution(ctx, executionID)
		if err != nil {
			return err
		}
		if job.DatabaseID != databaseID || job.Status != contracts.JobBlocked || gate.BlockedJobID == nil || *gate.BlockedJobID != job.ID || job.Result == nil || job.Result.State != contracts.ExecutionUnknown {
			return apperror.ErrConflict
		}

		var execution struct {
			ID                    string  `gorm:"column:id"`
			DatabaseID            string  `gorm:"column:database_id"`
			MigrationID           string  `gorm:"column:migration_id"`
			ActionType            string  `gorm:"column:action_type"`
			Status                string  `gorm:"column:status"`
			ExecutedBy            string  `gorm:"column:executed_by"`
			RolledBackExecutionID *string `gorm:"column:rolled_back_execution_id"`
		}
		if err := tx.DB.WithContext(ctx).Table("migration_executions").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", executionID).Take(&execution).Error; err != nil {
			return err
		}
		if execution.DatabaseID != databaseID || execution.Status != string(contracts.ExecutionUnknown) {
			return apperror.ErrConflict
		}

		now := time.Now()
		decisionText := "Manually reconciled after target inspection: changes confirmed applied."
		result := contracts.ExecutionResult{State: contracts.ExecutionSucceeded, RetryAdvice: contracts.RetryStop}
		jobStatus := contracts.JobSucceeded
		var blockedJobID any
		version := int64(0)

		if outcome == ReconcileApplied {
			switch execution.ActionType {
			case "FORWARD", "RETRY":
				if err := allocateMigrationVersion(ctx, tx.DB, databaseID, &version); err != nil {
					return err
				}
				if err := updateExactlyOne(tx.DB.WithContext(ctx).Table("migrations").Where("id = ? AND database_id = ? AND status = ?", execution.MigrationID, databaseID, "RUNNING").Updates(map[string]any{
					"status": "APPLIED", "version": version, "executed_by": execution.ExecutedBy,
					"executed_at": now, "updated_at": now, "error_summary": nil,
				})); err != nil {
					return err
				}
			case "ROLLBACK", "REVERT_BATCH":
				if err := updateExactlyOne(tx.DB.WithContext(ctx).Table("migrations").Where("id = ? AND database_id = ? AND status = ?", execution.MigrationID, databaseID, "RUNNING").Updates(map[string]any{
					"status": "ROLLED_BACK", "version": nil, "updated_at": now,
				})); err != nil {
					return err
				}
			default:
				return apperror.ErrConflict
			}
		} else {
			decisionText = "Manually reconciled after target inspection: changes confirmed not applied."
			result.State = contracts.ExecutionFailed
			result.ErrorCode = "MANUALLY_CONFIRMED_NOT_APPLIED"
			jobStatus = contracts.JobFailed
			if execution.ActionType == "ROLLBACK" {
				if err := updateExactlyOne(tx.DB.WithContext(ctx).Table("migrations").Where("id = ? AND database_id = ? AND status = ? AND version IS NOT NULL", execution.MigrationID, databaseID, "RUNNING").Updates(map[string]any{
					"status": "APPLIED", "updated_at": now,
					"executed_by": gorm.Expr("COALESCE((SELECT executed_by FROM migration_executions WHERE id = ?), executed_by)", execution.RolledBackExecutionID),
					"executed_at": gorm.Expr("COALESCE((SELECT finished_at FROM migration_executions WHERE id = ?), executed_at)", execution.RolledBackExecutionID),
				})); err != nil {
					return err
				}
			} else if (execution.ActionType == "RETRY" || execution.ActionType == "REVERT_BATCH") && execution.RolledBackExecutionID != nil {
				// The earlier partial execution remains unresolved. Restore its job
				// as the database gate owner so it can be resumed or compensated.
				previous, err := tx.GetByExecution(ctx, *execution.RolledBackExecutionID)
				if err != nil {
					return err
				}
				partialState := contracts.ExecutionPartial
				if previous.Result != nil && (previous.Result.State == contracts.ExecutionPartial || previous.Result.State == contracts.ExecutionPartiallyReverted) {
					partialState = previous.Result.State
				}
				partialResult := contracts.ExecutionResult{State: partialState, RetryAdvice: contracts.RetryResume, BlocksDatabase: true, CommittedBatches: previous.CommittedBatches, CommittedRows: previous.CommittedRows}
				encodedPartial, err := json.Marshal(partialResult)
				if err != nil {
					return err
				}
				if err := updateExactlyOne(tx.DB.WithContext(ctx).Model(&Job{}).Where("id = ? AND execution_id = ?", previous.ID, *execution.RolledBackExecutionID).Updates(map[string]any{
					"status": contracts.JobBlocked, "needs_inspection": false, "result_json": encodedPartial,
					"error_code": "PARTIAL_EXECUTION_REQUIRES_ACTION", "finished_at": now, "updated_at": now,
				})); err != nil {
					return err
				}
				blockedJobID = previous.ID
			} else {
				if err := updateExactlyOne(tx.DB.WithContext(ctx).Table("migrations").Where("id = ? AND database_id = ? AND status = ?", execution.MigrationID, databaseID, "RUNNING").Updates(map[string]any{
					"status": "FAILED", "version": nil, "executed_by": execution.ExecutedBy,
					"executed_at": now, "updated_at": now, "error_summary": decisionText,
				})); err != nil {
					return err
				}
			}
		}

		if version > 0 {
			if err := tx.DB.WithContext(ctx).Table("migration_executions").Where("id = ?", executionID).Update("version", version).Error; err != nil {
				return err
			}
		}
		if err := updateExactlyOne(tx.DB.WithContext(ctx).Table("migration_executions").Where("id = ? AND status = ?", executionID, string(contracts.ExecutionUnknown)).Updates(map[string]any{
			"status": string(result.State), "finished_at": now, "error_summary": decisionText,
		})); err != nil {
			return err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := updateExactlyOne(tx.DB.WithContext(ctx).Model(&Job{}).Where("id = ? AND status = ?", job.ID, contracts.JobBlocked).Updates(map[string]any{
			"status": jobStatus, "result_json": encoded, "error_code": result.ErrorCode,
			"needs_inspection": false, "claim_token": nil, "worker_id": nil,
			"lease_expires_at": nil, "finished_at": now, "updated_at": now,
		})); err != nil {
			return err
		}
		if err := updateExactlyOne(tx.DB.WithContext(ctx).Model(&DatabaseGate{}).Where("database_id = ? AND blocked_job_id = ?", databaseID, job.ID).
			Updates(map[string]any{"blocked_job_id": blockedJobID, "updated_at": now})); err != nil {
			return err
		}
		reconciled, err = tx.GetByExecution(ctx, executionID)
		return err
	})
	return reconciled, err
}

func allocateMigrationVersion(ctx context.Context, db *gorm.DB, databaseID string, version *int64) error {
	initialize := db.WithContext(ctx).Exec(`INSERT INTO migration_version_sequences(database_id,last_version)
		SELECT ?, GREATEST(
			COALESCE((SELECT MAX(version) FROM migrations WHERE database_id = ?), 0),
			(SELECT COUNT(*) FROM migrations WHERE database_id = ? AND status = 'APPLIED') +
			(SELECT COUNT(*) FROM migration_executions WHERE database_id = ? AND action_type = 'ROLLBACK' AND status = 'SUCCEEDED')
		) ON CONFLICT (database_id) DO NOTHING`, databaseID, databaseID, databaseID, databaseID)
	if initialize.Error != nil {
		return initialize.Error
	}
	return db.WithContext(ctx).Raw(`UPDATE migration_version_sequences SET last_version = last_version + 1
		WHERE database_id = ? RETURNING last_version`, databaseID).Scan(version).Error
}

func updateExactlyOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperror.ErrConflict
	}
	return nil
}
