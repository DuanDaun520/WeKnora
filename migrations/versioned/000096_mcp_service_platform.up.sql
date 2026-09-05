-- 000096: platform-wide MCP service catalog + per-workspace assignments.
--
-- MCP platform rework (mirrors the 000094/000095 governance designs):
--   1. An MCP service is configured ONCE at platform level (transport, auth,
--      credentials live in a single row) and then shared with workspaces
--      through explicit assignments — instead of every workspace holding its
--      own duplicate row.
--   2. Non-builtin service rows become platform-owned (tenant_id = 0); a
--      workspace sees exactly the services assigned to it in the new
--      tenant_mcp_service_assignments table, plus the is_builtin rows that
--      remain visible to every workspace by design.
--   3. Existing per-workspace services are migrated losslessly: an assignment
--      to the original workspace is created BEFORE the tenant_id reset, so
--      every workspace keeps the tool servers it had — including agents that
--      pin service IDs (custom_agents.config.mcp_services /
--      PinnedMCPServiceIDs).
--
-- Unlike web search there is no per-workspace "default service" concept, so
-- the assignment table carries no is_default flag.
--
-- Identical servers configured in several workspaces stay separate rows (no
-- dedup); the admin can merge them manually in the console.
--
-- Assignment rows are hard-deleted on removal; the audit log
-- (system.mcp_service_tenants_assigned) is the history of record.

DO $$ BEGIN RAISE NOTICE '[Migration 000096] Starting MCP service platform rework...'; END $$;

DO $$ BEGIN RAISE NOTICE '[Migration 000096] Creating table: tenant_mcp_service_assignments'; END $$;
CREATE TABLE IF NOT EXISTS tenant_mcp_service_assignments (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    service_id  VARCHAR(36) NOT NULL,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_tenant_mcp_service_assignments UNIQUE (tenant_id, service_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_mcp_service_assignments_service_id
    ON tenant_mcp_service_assignments(service_id);

-- Preserve current per-workspace visibility before resetting ownership.
-- Builtin rows are skipped: they stay visible to every workspace through the
-- is_builtin flag and never need an assignment.
DO $$ BEGIN RAISE NOTICE '[Migration 000096] Backfilling assignments from legacy service ownership'; END $$;
INSERT INTO tenant_mcp_service_assignments (tenant_id, service_id, assigned_by)
SELECT DISTINCT s.tenant_id, s.id, 'system'
FROM mcp_services s
WHERE s.deleted_at IS NULL AND s.tenant_id > 0 AND COALESCE(s.is_builtin, false) = false
ON CONFLICT DO NOTHING;

DO $$ BEGIN RAISE NOTICE '[Migration 000096] Resetting services to platform ownership (tenant_id = 0)'; END $$;
UPDATE mcp_services
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id > 0;
