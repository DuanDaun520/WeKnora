-- 000106: platform skill Chinese display metadata (中文名称 + 描述).
--
-- A platform skill's name/description come from SKILL.md (name immutable,
-- description overwritten by every bundle re-register). These two columns are
-- admin-managed Chinese display copy (000106): seeded from the register form,
-- edited only through the meta endpoint, never touched by a bundle re-register.
-- The console card shows zh_name as its title and zh_description (first 40
-- chars) as its summary, each falling back to the SKILL.md value when empty.
ALTER TABLE platform_skills
    ADD COLUMN IF NOT EXISTS zh_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS zh_description TEXT NOT NULL DEFAULT '';
