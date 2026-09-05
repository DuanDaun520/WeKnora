-- 000096 down: restore per-workspace MCP service ownership.
--
-- Non-builtin rows are returned to the workspace of their FIRST assignment
-- (earliest assigned_at); rows with no assignment left stay platform-owned —
-- a workspace cannot be inferred for them. Builtin rows keep tenant_id = 0;
-- their visibility never depended on the column.

UPDATE mcp_services s
SET tenant_id = a.tenant_id, updated_at = CURRENT_TIMESTAMP
FROM (
    SELECT DISTINCT ON (service_id) service_id, tenant_id
    FROM tenant_mcp_service_assignments
    ORDER BY service_id, assigned_at ASC, id ASC
) a
WHERE s.id = a.service_id AND s.deleted_at IS NULL AND COALESCE(s.is_builtin, false) = false;

DROP TABLE IF EXISTS tenant_mcp_service_assignments;
