-- Migration 000011: Allow pending approval rows when a group has no leader yet
-- so submissions remain PENDING_REVIEW rather than being rejected (Q&A #12).
ALTER TABLE migration_approvals
    ALTER COLUMN leader_id DROP NOT NULL;
