-- 000097: platform-level sandbox connection catalog + per-workspace
-- materialization.
--
-- A sandbox connection is configured ONCE here (provider endpoints,
-- credentials, runtime defaults) and assigned to 0..N workspaces. Unlike MCP
-- (000096) an assignment cannot share the platform row: skill installs
-- promote a snapshot into each config row's own config blob
-- (switchImagePointer rewrites tenant_sandbox_configs.config->skill_image)
-- and volume mounts are per-tenant, so a shared row would leak one
-- workspace's skills/volumes into another. Assignment therefore MATERIALIZES
-- an ordinary tenant_sandbox_configs row (payload copied, skill_image and
-- volume_mount always null) — the materialized row IS the assignment:
--
--   assignment list  = tenant_sandbox_configs WHERE source_connection_id = ?
--                      AND deleted_at IS NULL
--   drift            = sandbox_connections.updated_at
--                      > tenant_sandbox_configs.source_pushed_at
--
-- Edits to a connection propagate only through the explicit push action
-- (which reuses the workspace update cordon flow); workspace admins may keep
-- editing or deleting their materialized rows like any self-built config.
--
-- No separate assignment table exists on purpose: the materialized row
-- already carries every assignment fact, and a second table would need
-- bidirectional sync with a real failure mode when the two disagree.

DO $$ BEGIN RAISE NOTICE '[Migration 000097] Creating table: sandbox_connections'; END $$;
CREATE TABLE IF NOT EXISTS sandbox_connections (
    id           VARCHAR(36)  PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    sandbox_type VARCHAR(32)  NOT NULL,
    config       JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

-- Unique among live rows (mirrors tenant_sandbox_configs, 000082): the
-- console keys off connection names and materialization auto-suffixes on
-- collisions, so duplicates at platform level would make those collisions
-- routine.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sandbox_connections_name
    ON sandbox_connections (name) WHERE deleted_at IS NULL;

COMMENT ON COLUMN sandbox_connections.sandbox_type IS 'Promoted out of config so listing needs no decryption (mirrors tenant_sandbox_configs)';
COMMENT ON COLUMN sandbox_connections.config IS 'Connection payload (encrypted TenantSandboxConfig); skill_image and volume_mount are always null — they are per-workspace row state';

DO $$ BEGIN RAISE NOTICE '[Migration 000097] Linking materialized configs to their source connection'; END $$;
ALTER TABLE tenant_sandbox_configs
    ADD COLUMN IF NOT EXISTS source_connection_id VARCHAR(36) NULL,
    ADD COLUMN IF NOT EXISTS source_pushed_at TIMESTAMPTZ NULL;

COMMENT ON COLUMN tenant_sandbox_configs.source_connection_id IS 'sandbox_connections.id this row was materialized from; NULL = workspace self-built';
COMMENT ON COLUMN tenant_sandbox_configs.source_pushed_at IS 'Source connection updated_at snapshot at the last successful push/assignment; older than the connection updated_at means the row is drifting';

CREATE INDEX IF NOT EXISTS idx_tenant_sandbox_configs_source_connection
    ON tenant_sandbox_configs (source_connection_id)
    WHERE deleted_at IS NULL AND source_connection_id IS NOT NULL;
