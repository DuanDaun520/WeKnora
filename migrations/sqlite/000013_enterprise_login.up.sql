-- Mirrors versioned migration 000091_enterprise_login:
-- employee_id becomes the primary account identifier and login name;
-- must_change_password forces rotation of admin-set passwords on first
-- login; username/email lose their UNIQUE-ness (real names collide, email
-- is demoted to an optional contact field).
--
-- SQLite backs the inline `UNIQUE` constraints from 000000_init with
-- implicit auto-indexes that cannot be dropped individually, so the table
-- is rebuilt instead. This runs on the migration connection, which does
-- NOT enable foreign_keys (see internal/database/migration.go — the DSN
-- is the bare DB path), so DROP TABLE does not cascade into auth_tokens.

CREATE TABLE users_rebuilt (
    id VARCHAR(36) PRIMARY KEY,
    employee_id VARCHAR(64),
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    must_change_password BOOLEAN NOT NULL DEFAULT 0,
    avatar VARCHAR(500),
    tenant_id INTEGER,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    can_access_all_tenants BOOLEAN NOT NULL DEFAULT 0,
    is_system_admin BOOLEAN NOT NULL DEFAULT 0,
    -- Per-user JSON preferences; TEXT stands in for Postgres jsonb.
    preferences TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

-- Legacy accounts adopt their username as the employee ID. Usernames were
-- unique under the old schema, so this cannot collide. Legacy users know
-- their passwords — must_change_password starts at 0.
INSERT INTO users_rebuilt (
    id, employee_id, username, email, password_hash, must_change_password,
    avatar, tenant_id, is_active, can_access_all_tenants, is_system_admin,
    preferences, created_at, updated_at, deleted_at
)
SELECT
    id, username, username, email, password_hash, 0,
    avatar, tenant_id, is_active, can_access_all_tenants, is_system_admin,
    preferences, created_at, updated_at, deleted_at
FROM users;

DROP TABLE users;
ALTER TABLE users_rebuilt RENAME TO users;

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_users_is_system_admin ON users (is_system_admin);

-- Unique among live rows only; the predicate also covers the login lookup
-- (GORM's default soft-delete scope), so no separate plain index is needed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_employee_id_live
    ON users(employee_id)
    WHERE deleted_at IS NULL AND employee_id IS NOT NULL AND employee_id <> '';
