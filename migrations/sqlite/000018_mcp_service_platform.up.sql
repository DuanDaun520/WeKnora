-- 000018 (sqlite): platform-wide MCP service catalog + per-workspace
-- assignments. Mirror of the Postgres 000096 migration — see that file for
-- the design rationale.

CREATE TABLE IF NOT EXISTS tenant_mcp_service_assignments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER NOT NULL,
    service_id  VARCHAR(36) NOT NULL,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, service_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_mcp_service_assignments_service_id
    ON tenant_mcp_service_assignments(service_id);

-- Preserve current per-workspace visibility before resetting ownership.
INSERT OR IGNORE INTO tenant_mcp_service_assignments (tenant_id, service_id, assigned_by)
SELECT DISTINCT s.tenant_id, s.id, 'system'
FROM mcp_services s
WHERE s.deleted_at IS NULL AND s.tenant_id > 0 AND COALESCE(s.is_builtin, 0) = 0;

UPDATE mcp_services
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id > 0;
