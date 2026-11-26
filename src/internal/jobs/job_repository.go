package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostgresRepository struct{ DB *gorm.DB }

func fresh(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true})
}

func (r *PostgresRepository) transaction(ctx context.Context, fn func(*PostgresRepository) error) error {
	telemetry.Log(ctx, slog.LevelInfo, "transaction.started", "component", "jobs")
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{DB: tx})
	})

	if err != nil {
		telemetry.Log(ctx, slog.LevelWarn, "transaction.unconfirmed", "component", "jobs", "status", "failed_or_unknown")
	} else {
		telemetry.Log(ctx, slog.LevelInfo, "transaction.commit", "component", "jobs", "status", "success")
	}

	return err
}

func decodeJob(job *Job) error {
	if len(job.ResultJSON) != 0 {
		return json.Unmarshal(job.ResultJSON, &job.Result)
	}

	return nil
}

func affectedOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrClaimLost
	}

	return nil
}

func (r *PostgresRepository) findJob(ctx context.Context, operation string, build func(*gorm.DB) *gorm.DB) (Job, error) {
	var job Job
	result := r.run(ctx, operation, func(db *gorm.DB) *gorm.DB {
		return build(db).Take(&job)
	})

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return job, apperror.ErrNotFound
	}

	if result.Error != nil {
		return job, result.Error
	}

	if err := decodeJob(&job); err != nil {
		return job, err
	}

	return job, nil
}

func (r *PostgresRepository) GetByExecution(ctx context.Context, executionID string) (Job, error) {
	return r.findJob(ctx, "job.find_execution", func(db *gorm.DB) *gorm.DB {
		return db.Select("migration_jobs.*").Where("execution_id = ?", executionID)
	})
}

func (r *PostgresRepository) Enqueue(ctx context.Context, ref contracts.ExecutionRef, metadata ExecutionMetadata, maxAttempts int, carrier map[string]string) (Job, error) {
	var job Job
	traceJSON, err := json.Marshal(carrier)
	if err != nil {
		return job, err
	}

	err = r.transaction(ctx, func(tx *PostgresRepository) error {
		gate := DatabaseGate{DatabaseID: ref.DatabaseID}
		result := tx.run(ctx, "job.create_gate", func(db *gorm.DB) *gorm.DB {
			return db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "database_id"}}, DoNothing: true,
			}).Create(&gate)
		})

		if result.Error != nil {
			return result.Error
		}

		gate, err := tx.lockGate(ctx, ref.DatabaseID)
		if err != nil {
			return err
		}
		pending := Job{
			ExecutionID: ref.ExecutionID, DatabaseID: ref.DatabaseID, ActorID: metadata.ActorID,
			Status: contracts.JobPending, MaxAttempts: maxAttempts, TraceJSON: traceJSON,
		}

		result = tx.run(ctx, "job.enqueue", func(db *gorm.DB) *gorm.DB {
			return db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "execution_id"}}, DoNothing: true,
			}).Create(&pending)
		})

		if result.Error != nil {
			return result.Error
		}

		created := result.RowsAffected == 1
		job, err = tx.GetByExecution(ctx, ref.ExecutionID)

		if err != nil {
			return err
		}

		if job.DatabaseID != ref.DatabaseID || job.ActorID != metadata.ActorID {
			return apperror.ErrConflict
		}

		if created && metadata.ReplaceBlockedExecutionID != "" {
			return tx.handoff(ctx, gate, metadata.ReplaceBlockedExecutionID, job)
		}
		return nil
	})
	return job, err
}

// Transfer the gate only when Migration explicitly replaces its blocked request
func (r *PostgresRepository) handoff(ctx context.Context, gate DatabaseGate, blockedExecutionID string, replacement Job) error {
	if gate.BlockedJobID == nil {
		return apperror.ErrConflict
	}
	result := r.run(ctx, "job.supersede", func(db *gorm.DB) *gorm.DB {
		return db.Model(&Job{}).
			Where("id = ? AND execution_id = ? AND database_id = ?", *gate.BlockedJobID, blockedExecutionID, replacement.DatabaseID).
			Where("status IN ?", []contracts.JobStatus{contracts.JobBlocked, contracts.JobRetryWait}).
			Updates(map[string]any{
				"status": contracts.JobFailed, "error_code": "JOB_SUPERSEDED", "needs_inspection": false,
				"updated_at": gorm.Expr("clock_timestamp()"), "finished_at": gorm.Expr("clock_timestamp()"),
			})
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperror.ErrConflict
	}
	return affectedOne(r.run(ctx, "job.handoff", func(db *gorm.DB) *gorm.DB {
		return db.Model(&DatabaseGate{}).Where("database_id = ?", replacement.DatabaseID).
			Updates(map[string]any{"blocked_job_id": replacement.ID, "updated_at": gorm.Expr("clock_timestamp()")})
	}))
}

func jobView(db *gorm.DB) *gorm.DB {
	return db.Model(&Job{}).Table("migration_jobs AS j").
		Joins("JOIN migration_job_databases AS g ON g.database_id = j.database_id").
		Select("j.*, g.blocked_job_id IS NOT NULL AS database_blocked")
}

func (r *PostgresRepository) Get(ctx context.Context, databaseID, jobID string) (Job, error) {
	return r.findJob(ctx, "job.get", func(db *gorm.DB) *gorm.DB {
		return jobView(db).Where("j.database_id = ? AND j.id = ?", databaseID, jobID)
	})
}

func (r *PostgresRepository) List(ctx context.Context, databaseID string, options ListOptions) ([]Job, error) {
	jobs := make([]Job, 0)
	result := r.run(ctx, "job.list", func(db *gorm.DB) *gorm.DB {
		query := jobView(db).Where("j.database_id = ?", databaseID)

		if options.Status != "" {
			query = query.Where("j.status = ?", options.Status)
		}

		if options.Before != 0 {
			query = query.Where("j.sequence < ?", options.Before)
		}

		return query.Order("j.sequence DESC").Limit(options.Limit).Find(&jobs)
	})

	if result.Error != nil {
		return nil, result.Error
	}

	for i := range jobs {
		if err := decodeJob(&jobs[i]); err != nil {
			return nil, err
		}
	}

	return jobs, nil
}

// Lock the database gate before job rows to keep transaction ordering consistent
func (r *PostgresRepository) lockGate(ctx context.Context, databaseID string) (DatabaseGate, error) {
	var gate DatabaseGate
	result := r.run(ctx, "job.lock_gate", func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("database_id = ?", databaseID).Take(&gate)
	})

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return gate, ErrClaimLost
	}

	return gate, result.Error
}

// CountByStatus returns current migration_jobs row counts grouped by status for Prometheus gauges.
func (r *PostgresRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	err := r.DB.WithContext(ctx).
		Table("migration_jobs").
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
