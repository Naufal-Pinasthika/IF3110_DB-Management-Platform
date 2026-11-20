package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Build queue eligibility from subqueries while preserving database ordering
func readyJobs(db *gorm.DB) *gorm.DB {
	running := fresh(db).Model(&Job{}).Table("migration_jobs AS running").Select("1").
		Where("running.database_id = j.database_id").Where("running.id <> j.id").
		Where("running.status = ?", contracts.JobRunning)

	earlier := fresh(db).Model(&Job{}).Table("migration_jobs AS earlier").Select("1").
		Where("earlier.database_id = j.database_id").Where("earlier.sequence < j.sequence").
		Where(fresh(db).Where("earlier.status IN ?", []contracts.JobStatus{
			contracts.JobPending, contracts.JobRunning, contracts.JobRetryWait,
		}).Or("earlier.id = g.blocked_job_id"))

	due := fresh(db).Where("j.status IN ?", []contracts.JobStatus{contracts.JobPending, contracts.JobRetryWait}).
		Or(fresh(db).Where("j.status = ?", contracts.JobRunning).Where("j.lease_expires_at <= clock_timestamp()"))

	allowed := fresh(db).Where("g.blocked_job_id IS NULL").Or("g.blocked_job_id = j.id")
	ordered := fresh(db).Where("g.blocked_job_id = j.id").Or("NOT EXISTS (?)", earlier)

	return db.Model(&Job{}).Table("migration_jobs AS j").Select("j.*").
		Joins("JOIN migration_job_databases AS g ON g.database_id = j.database_id").
		Where("j.next_attempt_at <= clock_timestamp()").Where(due).Where(allowed).
		Where("NOT EXISTS (?)", running).Where(ordered)
}

func leaseDeadline(duration time.Duration) clause.Expr {
	return gorm.Expr("clock_timestamp() + ?::bigint * interval '1 millisecond'", duration.Milliseconds())
}

func claimedJob(db *gorm.DB, job Job) *gorm.DB {
	return db.Model(&Job{}).Where("id = ? AND claim_token = ?", job.ID, job.ClaimToken).
		Where("status = ?", contracts.JobRunning).Where("lease_expires_at > clock_timestamp()")
}

func (r *PostgresRepository) Ready(ctx context.Context) error {
	var rows []struct{ ID string }
	return r.run(ctx, "job.schema", func(db *gorm.DB) *gorm.DB {
		return db.Model(&Job{}).Table("migration_jobs AS j").Select("j.id").
			Joins("JOIN migration_job_databases AS g ON g.database_id = j.database_id").Limit(1).Find(&rows)
	}).Error
}

func (r *PostgresRepository) Candidates(ctx context.Context, limit int) ([]Job, error) {
	jobs := make([]Job, 0)
	result := r.run(ctx, "job.candidates", func(db *gorm.DB) *gorm.DB {
		return readyJobs(db).Order("j.sequence").Limit(limit).Find(&jobs)
	})
	return jobs, result.Error
}

// Claim a ready job while the caller holds the database advisory lock
func (r *PostgresRepository) Claim(ctx context.Context, jobID, workerID string, lease time.Duration) (Job, error) {
	var job Job
	err := r.transaction(ctx, func(tx *PostgresRepository) error {
		candidate, err := tx.findJob(ctx, "job.claim_lock", func(db *gorm.DB) *gorm.DB {
			return readyJobs(db).Where("j.id = ?", jobID).Clauses(clause.Locking{
				Strength: "UPDATE", Table: clause.Table{Name: "j"}, Options: "SKIP LOCKED",
			})
		})

		if errors.Is(err, apperror.ErrNotFound) {
			return ErrClaimLost
		}

		if err != nil {
			return err
		}

		result := tx.run(ctx, "job.claim", func(db *gorm.DB) *gorm.DB {
			return db.Model(&candidate).Clauses(clause.Returning{}).Updates(map[string]any{
				"status": contracts.JobRunning, "claim_token": gorm.Expr("gen_random_uuid()"),
				"worker_id": workerID, "lease_expires_at": leaseDeadline(lease),
				"heartbeat_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"),
				"started_at": gorm.Expr("COALESCE(started_at, clock_timestamp())"), "finished_at": nil,
			})
		})

		if err := affectedOne(result); err != nil {
			return err
		}

		job = candidate
		return decodeJob(&job)
	})
	return job, err
}

func (r *PostgresRepository) Heartbeat(ctx context.Context, job Job, lease time.Duration) error {
	return affectedOne(r.run(ctx, "job.heartbeat", func(db *gorm.DB) *gorm.DB {
		return claimedJob(db, job).Updates(map[string]any{
			"heartbeat_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"),
			"lease_expires_at": leaseDeadline(lease),
		})
	}))
}

// Persist uncertainty and a database block before invoking the executor
func (r *PostgresRepository) BeginExecution(ctx context.Context, job *Job) error {
	var updated Job
	err := r.transaction(ctx, func(tx *PostgresRepository) error {
		gate, err := tx.lockGate(ctx, job.DatabaseID)
		if err != nil {
			return err
		}

		if gate.BlockedJobID != nil && *gate.BlockedJobID != job.ID {
			return ErrClaimLost
		}

		changes := map[string]any{
			"needs_inspection": true, "updated_at": gorm.Expr("clock_timestamp()"),
			"attempt_count": gorm.Expr("attempt_count + ?", 1),
		}

		result := tx.run(ctx, "job.begin_execution", func(db *gorm.DB) *gorm.DB {
			return claimedJob(db, *job).Model(&updated).Clauses(clause.Returning{}).Updates(changes)
		})

		if err := affectedOne(result); err != nil {
			return err
		}

		result = tx.run(ctx, "job.block_database", func(db *gorm.DB) *gorm.DB {
			return db.Model(&DatabaseGate{}).Where("database_id = ?", job.DatabaseID).Updates(map[string]any{
				"blocked_job_id": job.ID, "updated_at": gorm.Expr("clock_timestamp()"),
			})
		})

		if err := affectedOne(result); err != nil {
			return err
		}

		return decodeJob(&updated)
	})

	if err == nil {
		*job = updated
	}
	return err
}
