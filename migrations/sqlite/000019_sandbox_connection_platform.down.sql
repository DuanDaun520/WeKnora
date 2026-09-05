-- 000019 (sqlite) down: drop the platform sandbox connection catalog.
-- Mirror of the Postgres 000097 down migration — materialized configs keep
-- working as self-built rows; only the provenance link columns go away.

DROP INDEX IF EXISTS idx_tenant_sandbox_configs_source_connection;
ALTER TABLE tenant_sandbox_configs DROP COLUMN source_pushed_at;
ALTER TABLE tenant_sandbox_configs DROP COLUMN source_connection_id;
DROP INDEX IF EXISTS uq_sandbox_connections_name;
DROP TABLE IF EXISTS sandbox_connections;
