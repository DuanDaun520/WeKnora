-- 000017 (sqlite, down): restore per-workspace provider ownership and drop
-- assignments. Best-effort mirror of Postgres 000095 down.

UPDATE web_search_providers
SET tenant_id = (
        SELECT MIN(a.tenant_id) FROM tenant_web_search_provider_assignments a
        WHERE a.provider_id = web_search_providers.id
    ),
    updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = 0
  AND EXISTS (SELECT 1 FROM tenant_web_search_provider_assignments a
              WHERE a.provider_id = web_search_providers.id);

DROP TABLE IF EXISTS tenant_web_search_provider_assignments;
