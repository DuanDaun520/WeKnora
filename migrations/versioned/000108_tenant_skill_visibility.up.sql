-- 000108: space-level skill visibility.
--
-- A catalog row materialized from the platform library (or built by a sandbox)
-- is visible across the workspace by default. A space admin can hide one
-- (000108): hidden rows disappear from the Skills/MCP browser, the agent
-- editor picker and @mention/runtime in this workspace, while the 技能目录
-- management page keeps listing them (GET /skills/catalog?include_hidden=1) so
-- they stay manageable. Push re-registers preserve the flag (UpdateCatalog
-- writes it from the loaded row), so a platform update cannot resurrect a
-- deliberately hidden skill.
ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS visible BOOLEAN NOT NULL DEFAULT TRUE;
