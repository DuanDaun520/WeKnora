-- Description: Category/author metadata for the platform skill library and the
-- author column its materialized rows carry into workspaces.
--
-- platform_skills.category/author are admin-managed definition metadata: seeded
-- from SKILL.md frontmatter (or the register form) on create, edited only
-- through the meta endpoint, and never touched by a bundle re-register. The
-- updated_at bump those edits make is what lights assignment drift so a push
-- propagates them. tenant_skill_catalog.author receives the platform value on
-- materialize; empty = unknown author.
ALTER TABLE platform_skills
    ADD COLUMN IF NOT EXISTS category VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS author VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS author VARCHAR(255) NOT NULL DEFAULT '';
