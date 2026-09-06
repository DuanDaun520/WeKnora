-- 000105 down: drop the category registry. Skill rows keep their category
-- strings (back to 000104's free-text behavior); only the registry table is
-- removed.
DROP TABLE IF EXISTS platform_skill_categories;
