ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'PENDING_REVIEW';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'APPROVED';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'QUEUED';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'REJECTED';

ALTER TABLE migrations
    ADD COLUMN IF NOT EXISTS sql_down TEXT,
    ADD COLUMN IF NOT EXISTS is_irreversible BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS irreversible_reason TEXT,
    ADD COLUMN IF NOT EXISTS migration_type VARCHAR(20) NOT NULL DEFAULT 'REGULAR',
    ADD COLUMN IF NOT EXISTS batch_attribute VARCHAR(100),
    ADD COLUMN IF NOT EXISTS batch_size INTEGER DEFAULT 1000;

CREATE TABLE IF NOT EXISTS migration_approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    migration_id UUID NOT NULL REFERENCES migrations(id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    leader_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    comment TEXT,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT migration_approvals_unique UNIQUE (migration_id, group_id)
);

CREATE INDEX IF NOT EXISTS migration_approvals_migration_idx ON migration_approvals (migration_id);
CREATE INDEX IF NOT EXISTS migration_approvals_leader_idx ON migration_approvals (leader_id);

CREATE TABLE IF NOT EXISTS migration_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    migration_id UUID NOT NULL REFERENCES migrations(id) ON DELETE RESTRICT,
    database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE RESTRICT,
    action_type VARCHAR(20) NOT NULL CHECK (action_type IN ('FORWARD', 'ROLLBACK', 'REVERT_BATCH', 'RETRY')),
    rolled_back_execution_id UUID REFERENCES migration_executions(id) ON DELETE SET NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED', 'PARTIAL', 'PARTIALLY_REVERTED', 'UNKNOWN')),
    executed_sql TEXT NOT NULL,
    executed_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMPTZ,
    version BIGINT,
    error_summary TEXT,
    committed_batches BIGINT NOT NULL DEFAULT 0,
    committed_rows BIGINT NOT NULL DEFAULT 0,
    last_cursor_value TEXT
);

CREATE INDEX IF NOT EXISTS migration_executions_db_idx ON migration_executions (database_id, started_at DESC);
CREATE INDEX IF NOT EXISTS migration_executions_migration_idx ON migration_executions (migration_id);
CREATE INDEX IF NOT EXISTS migration_executions_rollback_idx ON migration_executions (rolled_back_execution_id);
