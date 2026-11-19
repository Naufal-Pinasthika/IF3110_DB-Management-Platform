BEGIN;

CREATE TABLE IF NOT EXISTS migration_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sequence BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    execution_id UUID NOT NULL UNIQUE,
    database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE RESTRICT,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','RUNNING','RETRY_WAIT','SUCCEEDED','FAILED','BLOCKED')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    needs_inspection BOOLEAN NOT NULL DEFAULT FALSE,
    claim_token UUID,
    worker_id TEXT,
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    committed_batches BIGINT NOT NULL DEFAULT 0 CHECK (committed_batches >= 0),
    committed_rows BIGINT NOT NULL DEFAULT 0 CHECK (committed_rows >= 0),
    result_json JSONB,
    trace_json JSONB NOT NULL DEFAULT '{}',
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    CHECK ((status = 'RUNNING') = (claim_token IS NOT NULL AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS migration_jobs_ready_idx ON migration_jobs(next_attempt_at, sequence)
    WHERE status IN ('PENDING','RETRY_WAIT','RUNNING');
CREATE INDEX IF NOT EXISTS migration_jobs_database_idx ON migration_jobs(database_id, sequence);
CREATE UNIQUE INDEX IF NOT EXISTS migration_jobs_one_running_idx ON migration_jobs(database_id)
    WHERE status = 'RUNNING';

CREATE TABLE IF NOT EXISTS migration_job_databases (
    database_id UUID PRIMARY KEY REFERENCES managed_databases(id) ON DELETE RESTRICT,
    blocked_job_id UUID REFERENCES migration_jobs(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

COMMIT;
