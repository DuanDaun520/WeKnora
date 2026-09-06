-- 000105: category registry for the platform skill library (000104 rework).
--
-- Categories stop being free strings minted by whoever registers a skill and
-- become an independent registry the console's「分类管理」dialog owns: an admin
-- CREATES a category there first, and the register/edit drawer can then only
-- pick from the registry (its dropdown is no longer creatable). A skill still
-- references a category by name (platform_skills.category stays a plain string,
-- empty = uncategorized): no foreign key, because clearing — not cascading — is
-- what deleting a category means for skills.
--
-- The registry row count is the management surface; the usage count shown next
-- to each name is the number of live skills referencing it, computed by LEFT
-- JOIN in the repository (so a just-created, unused category still lists).

DO $$ BEGIN RAISE NOTICE '[Migration 000105] Creating table: platform_skill_categories'; END $$;
CREATE TABLE IF NOT EXISTS platform_skill_categories (
    id         VARCHAR(36)  PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- One live category per name: the name is what skills reference, so a rename is
-- an UPDATE of this row plus the referencing skills, never an insert.
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_skill_categories_name
    ON platform_skill_categories (name) WHERE deleted_at IS NULL;

-- Backfill 000104's free-text vocabulary so existing categories stay selectable
-- once the drawer stops minting new ones. Idempotent (NOT EXISTS + unique index).
INSERT INTO platform_skill_categories (id, name)
SELECT uuid_generate_v4(), d.category
FROM (
    SELECT DISTINCT category FROM platform_skills
    WHERE category <> '' AND deleted_at IS NULL
) d
WHERE NOT EXISTS (
    SELECT 1 FROM platform_skill_categories c
    WHERE c.deleted_at IS NULL AND c.name = d.category
);

COMMENT ON TABLE platform_skill_categories IS 'Registered skill categories (000105). Skills reference rows by name via platform_skills.category; deleting a category clears that string on its skills rather than cascading.';
