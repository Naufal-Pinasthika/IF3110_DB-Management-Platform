BEGIN;

CREATE TYPE access_level AS ENUM (
    'VIEW_ONLY',
    'VIEW_EDIT'
);

CREATE TYPE migration_status AS ENUM (
    'DRAFT',
    'RUNNING',
    'APPLIED',
    'FAILED'
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_platform_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE user_groups (
    user_id UUID NOT NULL,
    group_id UUID NOT NULL,
    PRIMARY KEY (user_id, group_id),
    CONSTRAINT user_groups_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT user_groups_group_id_fkey
        FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE CASCADE
);

CREATE INDEX user_groups_group_id_idx ON user_groups (group_id);

CREATE TABLE managed_databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(100) NOT NULL UNIQUE,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 5432 CHECK (port BETWEEN 1 AND 65535),
    database_name VARCHAR(100) NOT NULL,
    ssl_mode VARCHAR(20) NOT NULL,
    reader_username VARCHAR(100) NOT NULL,
    reader_password_enc TEXT NOT NULL,
    migrator_username VARCHAR(100) NOT NULL,
    migrator_password_enc TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE group_access (
    group_id UUID NOT NULL,
    database_id UUID NOT NULL,
    access_level access_level NOT NULL,
    PRIMARY KEY (group_id, database_id),
    CONSTRAINT group_access_group_id_fkey
        FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE CASCADE,
    CONSTRAINT group_access_database_id_fkey
        FOREIGN KEY (database_id) REFERENCES managed_databases (id) ON DELETE CASCADE
);

CREATE INDEX group_access_database_id_idx ON group_access (database_id);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    previous_token_hash CHAR(64),
    previous_valid_until TIMESTAMPTZ,
    csrf_token_hash CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT sessions_previous_token_grace_pair_check CHECK (
        (previous_token_hash IS NULL AND previous_valid_until IS NULL)
        OR (previous_token_hash IS NOT NULL AND previous_valid_until IS NOT NULL)
    ),
    CONSTRAINT sessions_absolute_expiration_check CHECK (
        absolute_expires_at > created_at
    )
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_active_expiration_idx
    ON sessions (absolute_expires_at)
    WHERE revoked_at IS NULL;
CREATE INDEX sessions_previous_token_hash_idx
    ON sessions (previous_token_hash)
    WHERE previous_token_hash IS NOT NULL;

CREATE TABLE migrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id UUID NOT NULL REFERENCES managed_databases (id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    sql_text TEXT NOT NULL,
    status migration_status NOT NULL DEFAULT 'DRAFT',
    version BIGINT,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    executed_by UUID REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    executed_at TIMESTAMPTZ,
    error_summary TEXT,
    CONSTRAINT migrations_version_positive_check CHECK (version IS NULL OR version > 0),
    CONSTRAINT migrations_applied_version_check CHECK (
        (status = 'APPLIED' AND version IS NOT NULL AND executed_by IS NOT NULL AND executed_at IS NOT NULL)
        OR (status <> 'APPLIED' AND version IS NULL)
    ),
    CONSTRAINT migrations_version_unique UNIQUE (database_id, version)
);

CREATE INDEX migrations_database_status_idx ON migrations (database_id, status);

COMMIT;
