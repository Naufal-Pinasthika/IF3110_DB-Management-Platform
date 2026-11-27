-- Migration 000009: Add rollback/forward approval support
-- Adds ROLLBACK_APPROVED and ROLLED_BACK statuses and action column on migration_approvals

ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'ROLLBACK_APPROVED';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'ROLLED_BACK';

-- action column distinguishes approval rounds: MIGRATE (initial), ROLLBACK, FORWARD.
ALTER TABLE migration_approvals
    ADD COLUMN IF NOT EXISTS action VARCHAR(20) NOT NULL DEFAULT 'MIGRATE';
