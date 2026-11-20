package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"gorm.io/gorm"
)

func (r *PostgresRepository) UpdateProgress(ctx context.Context, job Job, progress Progress) error {
	if progress.CommittedBatches < 0 || progress.CommittedRows < 0 {
		return apperror.ErrInvalidInput
	}
	return affectedOne(r.run(ctx, "job.progress", func(db *gorm.DB) *gorm.DB {
		return claimedJob(db, job).Updates(map[string]any{
			"committed_batches": gorm.Expr("GREATEST(committed_batches, ?)", progress.CommittedBatches),
			"committed_rows":    gorm.Expr("GREATEST(committed_rows, ?)", progress.CommittedRows),
			"updated_at":        gorm.Expr("clock_timestamp()"),
		})
	}))
}

type completion struct {
	Status  contracts.JobStatus
	Result  contracts.ExecutionResult
	Inspect bool
	Delay   time.Duration
}

func (r *PostgresRepository) Finish(ctx context.Context, job Job, done completion) error {
	encoded, err := json.Marshal(done.Result)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *PostgresRepository) error {
		gate, err := tx.lockGate(ctx, job.DatabaseID)
		if err != nil {
			return err
		}
		if gate.BlockedJobID == nil || *gate.BlockedJobID != job.ID {
			return ErrClaimLost
		}
		var finishedAt any
		if done.Status == contracts.JobSucceeded || done.Status == contracts.JobFailed || done.Status == contracts.JobBlocked {
			finishedAt = gorm.Expr("clock_timestamp()")
		}
		result := tx.run(ctx, "job.finish", func(db *gorm.DB) *gorm.DB {
			return claimedJob(db, job).Updates(map[string]any{
				"status": done.Status, "result_json": encoded, "error_code": done.Result.ErrorCode,
				"committed_batches": gorm.Expr("GREATEST(committed_batches, ?)", done.Result.CommittedBatches),
				"committed_rows":    gorm.Expr("GREATEST(committed_rows, ?)", done.Result.CommittedRows),
				"needs_inspection":  done.Inspect, "next_attempt_at": leaseDeadline(done.Delay),
				"claim_token": nil, "worker_id": nil, "lease_expires_at": nil,
				"updated_at": gorm.Expr("clock_timestamp()"), "finished_at": finishedAt,
			})
		})
		if err := affectedOne(result); err != nil {
			return err
		}
		var blockedJob any
		if done.Result.BlocksDatabase {
			blockedJob = job.ID
		}
		result = tx.run(ctx, "job.finish_gate", func(db *gorm.DB) *gorm.DB {
			return db.Model(&DatabaseGate{}).Where("database_id = ? AND blocked_job_id = ?", job.DatabaseID, job.ID).
				Updates(map[string]any{"blocked_job_id": blockedJob, "updated_at": gorm.Expr("clock_timestamp()")})
		})
		if err := affectedOne(result); err != nil {
			return err
		}
		return nil
	})
}
