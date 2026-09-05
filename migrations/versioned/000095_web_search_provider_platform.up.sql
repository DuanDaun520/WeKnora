-- 000095: platform-wide web search provider catalog + per-workspace
-- assignments.
--
-- Web search platform rework (mirrors the 000094 model governance design):
--   1. A search service is configured ONCE at platform level (credentials,
--      engine, proxy live in a single row) and then shared with workspaces
--      through explicit assignments — instead of every workspace holding its
--      own duplicate row.
--   2. All provider rows become platform-owned (tenant_id = 0); a workspace
--      sees exactly the services assigned to it in the new
--      tenant_web_search_provider_assignments table.
--   3. Existing per-workspace providers are migrated losslessly: an
--      assignment to the original workspace (carrying the row's is_default)
--      is created BEFORE the tenant_id reset, so every workspace keeps the
--      search behaviour it had — including the per-workspace default and
--      agent-pinned provider IDs (custom_agents.config.web_search_provider_id).
--
-- Identical credentials configured in several workspaces stay separate rows
-- (no dedup); the admin can merge them manually in the console.
--
-- Assignment rows are hard-deleted on removal; the audit log
-- (system.web_search_provider_assigned) is the history of record.

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Starting web search provider platform rework...'; END $$;

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Creating table: tenant_web_search_provider_assignments'; END $$;
CREATE TABLE IF NOT EXISTS tenant_web_search_provider_assignments (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    provider_id VARCHAR(36) NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_tenant_web_search_provider_assignments UNIQUE (tenant_id, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_web_search_provider_assignments_provider_id
    ON tenant_web_search_provider_assignments(provider_id);

-- Preserve current per-workspace visibility (and defaults) before resetting
-- ownership.
DO $$ BEGIN RAISE NOTICE '[Migration 000095] Backfilling assignments from legacy provider ownership'; END $$;
INSERT INTO tenant_web_search_provider_assignments (tenant_id, provider_id, is_default, assigned_by)
SELECT DISTINCT p.tenant_id, p.id, COALESCE(p.is_default, false), 'system'
FROM web_search_providers p
WHERE p.deleted_at IS NULL AND p.tenant_id > 0
ON CONFLICT DO NOTHING;

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Resetting providers to platform ownership (tenant_id = 0)'; END $$;
UPDATE web_search_providers
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id > 0;
