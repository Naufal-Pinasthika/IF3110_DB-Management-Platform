-- Migration 000010: Allow applied migrations undergoing rollback review/execution
-- to retain their applied version until MarkRolledBack clears it.
ALTER TABLE migrations DROP CONSTRAINT IF EXISTS migrations_applied_version_check;
ALTER TABLE migrations ADD CONSTRAINT migrations_applied_version_check CHECK (
    (status = 'APPLIED' AND version IS NOT NULL AND executed_by IS NOT NULL AND executed_at IS NOT NULL)
    OR (status IN ('PENDING_REVIEW', 'ROLLBACK_APPROVED', 'RUNNING'))
    OR (status NOT IN ('APPLIED', 'PENDING_REVIEW', 'ROLLBACK_APPROVED', 'RUNNING') AND version IS NULL)
);
