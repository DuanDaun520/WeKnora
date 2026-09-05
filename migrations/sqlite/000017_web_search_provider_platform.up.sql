-- 000017 (sqlite): platform-wide web search provider catalog + per-workspace
-- assignments. Mirror of the Postgres 000095 migration — see that file for
-- the design rationale.

CREATE TABLE IF NOT EXISTS tenant_web_search_provider_assignments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER NOT NULL,
    provider_id VARCHAR(36) NOT NULL,
    is_default  INTEGER NOT NULL DEFAULT 0,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_web_search_provider_assignments_provider_id
    ON tenant_web_search_provider_assignments(provider_id);

-- Preserve current per-workspace visibility (and defaults) before resetting
-- ownership.
INSERT OR IGNORE INTO tenant_web_search_provider_assignments (tenant_id, provider_id, is_default, assigned_by)
SELECT DISTINCT p.tenant_id, p.id, COALESCE(p.is_default, 0), 'system'
FROM web_search_providers p
WHERE p.deleted_at IS NULL AND p.tenant_id > 0;

UPDATE web_search_providers
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id > 0;
