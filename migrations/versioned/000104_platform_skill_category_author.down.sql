ALTER TABLE platform_skills
    DROP COLUMN IF EXISTS author,
    DROP COLUMN IF EXISTS category;

ALTER TABLE tenant_skill_catalog
    DROP COLUMN IF EXISTS author;
