-- 000109: platform skill help URL (介绍与帮助网址).
--
-- Optional admin-managed metadata, same regime as category/author (000104):
-- seeded from the register form, edited only through the meta endpoint, never
-- touched by a bundle re-register. Empty = none. The workspace copy rides the
-- materialize/push CatalogMeta like category/author do, and the user-side
-- Skills detail dialog renders it as an external link (new tab) when set.
ALTER TABLE platform_skills
    ADD COLUMN IF NOT EXISTS help_url VARCHAR(1024) NOT NULL DEFAULT '';

ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS help_url VARCHAR(1024) NOT NULL DEFAULT '';
