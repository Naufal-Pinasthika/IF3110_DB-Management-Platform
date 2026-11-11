-- Existing groups are assigned explicitly by an admin through PATCH group.
-- New groups require an active leader in the application.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS leader_id UUID
    REFERENCES users (id) ON DELETE RESTRICT;

-- One user may lead multiple groups; this index must not be UNIQUE.
CREATE INDEX IF NOT EXISTS groups_leader_id_idx ON groups (leader_id);
