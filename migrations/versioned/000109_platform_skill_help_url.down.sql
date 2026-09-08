ALTER TABLE platform_skills
    DROP COLUMN IF EXISTS help_url;

ALTER TABLE tenant_skill_catalog
    DROP COLUMN IF EXISTS help_url;
