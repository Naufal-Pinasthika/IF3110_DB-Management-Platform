BEGIN;

-- Preserve a database-wide applied-version high-water mark across rollbacks.
CREATE TABLE IF NOT EXISTS migration_version_sequences (
    database_id UUID PRIMARY KEY REFERENCES managed_databases(id) ON DELETE RESTRICT,
    last_version BIGINT NOT NULL DEFAULT 0 CHECK (last_version >= 0)
);

INSERT INTO migration_version_sequences (database_id, last_version)
SELECT d.id,
       GREATEST(
           COALESCE((SELECT MAX(m.version) FROM migrations m WHERE m.database_id = d.id), 0),
           (SELECT COUNT(*) FROM migrations m WHERE m.database_id = d.id AND m.status = 'APPLIED') +
           (SELECT COUNT(*) FROM migration_executions e WHERE e.database_id = d.id AND e.action_type = 'ROLLBACK' AND e.status = 'SUCCEEDED')
       )
FROM managed_databases d
ON CONFLICT (database_id) DO NOTHING;

COMMIT;
