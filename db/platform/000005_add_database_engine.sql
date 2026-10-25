-- Safe for existing installations; old targets remain PostgreSQL.
ALTER TABLE managed_databases
    ADD COLUMN IF NOT EXISTS engine VARCHAR(16) NOT NULL DEFAULT 'postgres'
    CHECK (engine IN ('postgres', 'mysql'));
