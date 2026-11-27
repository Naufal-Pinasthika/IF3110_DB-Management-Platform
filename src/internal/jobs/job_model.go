package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
)

var (
	ErrUnavailable = errors.New("migration execution integration is not configured")
	ErrClaimLost   = errors.New("job claim or database lock was lost")
)

// Migration owns permission approval and safe queue admission
type ExecutionRegistry interface {
	ValidateExecution(context.Context, contracts.ExecutionRef) (ExecutionMetadata, error)
}

type ExecutionMetadata struct {
	ActorID string
	// Migration authorizes this handoff after reconciling the blocked execution
	ReplaceBlockedExecutionID string
}

type ProgressReader interface {
	ReadProgress(context.Context, contracts.ExecutionRef) (Progress, error)
}

type Progress struct {
	CommittedBatches int64 `json:"committed_batches"`
	CommittedRows    int64 `json:"committed_rows"`
}

// queue state and mirrors committed execution progress
type Job struct {
	ID               string                     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Sequence         int64                      `gorm:"type:bigint;->;uniqueIndex:migration_jobs_sequence_key;index:migration_jobs_database_idx,priority:2" json:"-"`
	ExecutionID      string                     `gorm:"type:uuid;not null;uniqueIndex:migration_jobs_execution_id_key" json:"execution_id"`
	DatabaseID       string                     `gorm:"type:uuid;not null;index:migration_jobs_database_idx,priority:1" json:"database_id"`
	ActorID          string                     `gorm:"type:uuid;not null" json:"actor_id"`
	Status           contracts.JobStatus        `gorm:"type:text;not null;default:PENDING" json:"status"`
	AttemptCount     int                        `gorm:"not null;default:0" json:"attempt_count"` // Count executor calls and inspections
	MaxAttempts      int                        `gorm:"not null" json:"max_attempts"`
	NeedsInspection  bool                       `gorm:"not null;default:false" json:"needs_inspection"`
	ClaimToken       *string                    `gorm:"type:uuid" json:"-"`
	WorkerID         *string                    `gorm:"type:text" json:"-"`
	LeaseExpiresAt   *time.Time                 `gorm:"type:timestamptz" json:"lease_expires_at,omitempty"`
	HeartbeatAt      *time.Time                 `gorm:"type:timestamptz" json:"heartbeat_at,omitempty"`
	NextAttemptAt    time.Time                  `gorm:"type:timestamptz;not null;default:clock_timestamp()" json:"next_attempt_at"`
	CommittedBatches int64                      `gorm:"type:bigint;not null;default:0" json:"committed_batches"`
	CommittedRows    int64                      `gorm:"type:bigint;not null;default:0" json:"committed_rows"`
	ResultJSON       []byte                     `gorm:"column:result_json;type:jsonb" json:"-"`
	TraceJSON        []byte                     `gorm:"column:trace_json;type:jsonb;not null" json:"-"`
	ErrorCode        string                     `gorm:"type:text;not null;default:''" json:"error_code,omitempty"`
	CreatedAt        time.Time                  `gorm:"type:timestamptz;not null;autoCreateTime:false;default:clock_timestamp()" json:"created_at"`
	UpdatedAt        time.Time                  `gorm:"type:timestamptz;not null;autoUpdateTime:false;default:clock_timestamp()" json:"updated_at"`
	StartedAt        *time.Time                 `gorm:"type:timestamptz" json:"started_at,omitempty"`
	FinishedAt       *time.Time                 `gorm:"type:timestamptz" json:"finished_at,omitempty"`
	Result           *contracts.ExecutionResult `gorm:"-" json:"result,omitempty"`
	DatabaseBlocked  bool                       `gorm:"->;-:migration" json:"database_blocked"`
}

func (Job) TableName() string { return "migration_jobs" }

// Keep the database blocked after a crash until its execution outcome is confirmed
type DatabaseGate struct {
	DatabaseID   string    `gorm:"type:uuid;primaryKey"`
	BlockedJobID *string   `gorm:"type:uuid"`
	UpdatedAt    time.Time `gorm:"type:timestamptz;not null;autoUpdateTime:false;default:clock_timestamp()"`
}

func (DatabaseGate) TableName() string { return "migration_job_databases" }

func (j Job) Ref() contracts.ExecutionRef {
	return contracts.ExecutionRef{ExecutionID: j.ExecutionID, DatabaseID: j.DatabaseID}
}

func (j Job) Receipt() contracts.JobReceipt {
	return contracts.JobReceipt{JobID: j.ID, ExecutionID: j.ExecutionID, Status: j.Status}
}

type ListOptions struct {
	Status contracts.JobStatus
	Limit  int
	Before int64
}

type Page struct {
	Jobs       []Job `json:"jobs"`
	NextBefore int64 `json:"next_before,omitempty"`
}
