-- 000095 (down): restore per-workspace provider ownership and drop
-- assignments.
--
-- Best-effort reverse of the up migration: a service that was assigned to
-- several workspaces cannot go back to exactly one owner, so it returns to
-- the lowest tenant_id that held an assignment (the legacy owner whenever
-- the up migration created the row). The row-level is_default column keeps
-- its pre-migration value — post-000095 writes never touch it.

UPDATE web_search_providers p
SET tenant_id = sub.owner_tenant, updated_at = CURRENT_TIMESTAMP
FROM (
    SELECT provider_id, MIN(tenant_id) AS owner_tenant
    FROM tenant_web_search_provider_assignments
    GROUP BY provider_id
) sub
WHERE p.id = sub.provider_id AND p.tenant_id = 0;

DROP TABLE IF EXISTS tenant_web_search_provider_assignments;
