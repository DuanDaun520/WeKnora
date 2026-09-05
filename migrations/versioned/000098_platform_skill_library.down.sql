-- 000098 down: drop the platform skill library.
--
-- Materialized tenant_skill_catalog rows SURVIVE the down — they are ordinary
-- workspace catalog rows whose zips already live in each workspace's own
-- object storage; only the provenance column goes away (installs keep working
-- as self-built skills).

DROP INDEX IF EXISTS idx_tenant_skill_catalog_source_platform;
ALTER TABLE tenant_skill_catalog
    DROP COLUMN IF EXISTS source_platform_skill_id;
DROP INDEX IF EXISTS idx_platform_skill_assignments_tenant;
DROP INDEX IF EXISTS uq_platform_skill_assignments;
DROP TABLE IF EXISTS platform_skill_assignments;
DROP INDEX IF EXISTS uq_platform_skills_name;
DROP TABLE IF EXISTS platform_skills;
