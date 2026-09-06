ALTER TABLE tenant_skill_catalog
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS category;

ALTER TABLE mcp_services
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS category;
