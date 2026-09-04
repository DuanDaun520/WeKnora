-- 000094 (down): restore per-workspace model ownership and drop assignments.
--
-- Best-effort reverse of the up migration: a model that was assigned to
-- several workspaces cannot go back to exactly one owner, so it returns to
-- the lowest tenant_id that held an assignment (the legacy owner whenever
-- the up migration created the row). Built-in models are untouched.

UPDATE models m
SET tenant_id = sub.owner_tenant, updated_at = CURRENT_TIMESTAMP
FROM (
    SELECT model_id, MIN(tenant_id) AS owner_tenant
    FROM tenant_model_assignments
    GROUP BY model_id
) sub
WHERE m.id = sub.model_id AND m.is_builtin = false AND m.tenant_id = 0;

DROP TABLE IF EXISTS tenant_model_assignments;
