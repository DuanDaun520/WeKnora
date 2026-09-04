-- Reverts sqlite migration 000013: restores the legacy users shape
-- (UNIQUE username/email, no employee_id / must_change_password) via a
-- table rebuild. Fails loudly if relaxing the constraints admitted
-- duplicates — that data cannot be downgraded.

CREATE TABLE users_legacy (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    avatar VARCHAR(500),
    tenant_id INTEGER,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    can_access_all_tenants BOOLEAN NOT NULL DEFAULT 0,
    is_system_admin BOOLEAN NOT NULL DEFAULT 0,
    preferences TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

INSERT INTO users_legacy (
    id, username, email, password_hash, avatar, tenant_id, is_active,
    can_access_all_tenants, is_system_admin, preferences, created_at, updated_at, deleted_at
)
SELECT
    id, username, email, password_hash, avatar, tenant_id, is_active,
    can_access_all_tenants, is_system_admin, preferences, created_at, updated_at, deleted_at
FROM users;

DROP TABLE users;
ALTER TABLE users_legacy RENAME TO users;

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_users_is_system_admin ON users (is_system_admin);
