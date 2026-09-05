-- 000018 (sqlite) down: restore per-workspace MCP service ownership.
-- Mirror of the Postgres 000096 down migration.

UPDATE mcp_services
SET tenant_id = (
    SELECT a.tenant_id
    FROM tenant_mcp_service_assignments a
    WHERE a.service_id = mcp_services.id
    ORDER BY a.assigned_at ASC, a.id ASC
    LIMIT 1
), updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND COALESCE(is_builtin, 0) = 0;

DROP TABLE IF EXISTS tenant_mcp_service_assignments;
