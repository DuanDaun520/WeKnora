-- 000019 (sqlite): platform-level sandbox connection catalog + per-workspace
-- materialization. Mirror of the Postgres 000097 migration — see that file
-- for the design rationale.

CREATE TABLE IF NOT EXISTS sandbox_connections (
    id           VARCHAR(36) PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    sandbox_type VARCHAR(32) NOT NULL,
    config       JSON NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at   TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sandbox_connections_name
    ON sandbox_connections (name) WHERE deleted_at IS NULL;

ALTER TABLE tenant_sandbox_configs ADD COLUMN source_connection_id VARCHAR(36);
ALTER TABLE tenant_sandbox_configs ADD COLUMN source_pushed_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_tenant_sandbox_configs_source_connection
    ON tenant_sandbox_configs (source_connection_id)
    WHERE deleted_at IS NULL AND source_connection_id IS NOT NULL;
