package contracts

type ExecutionRef struct {
	ExecutionID string `json:"execution_id"`
	DatabaseID string `json:"database_id"`
}

// describes the database execution outcome
type ExecutionState string
const (
	ExecutionSucceeded ExecutionState = "SUCCEEDED"
	ExecutionFailed ExecutionState = "FAILED"
	ExecutionPartial ExecutionState = "PARTIAL"
	ExecutionPartiallyReverted ExecutionState = "PARTIALLY_REVERTED"
	ExecutionUnknown ExecutionState = "UNKNOWN"
)

// determine migration decision about safe recovery on retry
type RetryAdvice string
const (
	RetryStop RetryAdvice = "STOP" // forbid current execution
	RetrySafe RetryAdvice = "RETRY_SAFE" // allow same execution to be retried (after migration) so it cannot duplicate committed changes
	RetryResume RetryAdvice = "RESUME" // allow resume execution without replaying committed batches
	RetryReconcile RetryAdvice = "RECONCILE" // inspect execution before any further action 
)

// reports migration outcome, history and committed checkpoint
type ExecutionResult struct {
	State ExecutionState `json:"state"`
	CommittedBatches int64 `json:"committed_batches"`
	CommittedRows int64 `json:"committed_rows"`
	RetryAdvice RetryAdvice `json:"retry_advice"`
	BlocksDatabase bool `json:"blocks_database"`
	ErrorCode string `json:"error_code,omitempty"`
}

// status queue processing (not whether database changes committed)
// to do that inspect ExecutionResult to determine the database outcome
type JobStatus string
const (
	JobPending JobStatus = "PENDING"
	JobRunning JobStatus = "RUNNING"
	JobRetryWait JobStatus = "RETRY_WAIT"
	JobSucceeded JobStatus = "SUCCEEDED"
	JobFailed JobStatus = "FAILED"
	JobBlocked JobStatus = "BLOCKED"
)

// acknowledge durable acceptance (does not mean successful SQL execution)
// repeated enqueue calls for one ExecutionID return the same logical job
type JobReceipt struct {
	JobID string `json:"job_id"`
	ExecutionID string `json:"execution_id"`
	Status JobStatus `json:"status"`
}
