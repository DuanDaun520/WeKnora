-- 000098: platform-level skill library + per-workspace materialization.
--
-- A skill is registered ONCE here (bundle metadata + provenance source) and
-- assigned to 0..N workspaces. Like sandbox connections (000097) an assignment
-- cannot share the platform row, but for a stronger reason: skill installs
-- stamp a bundle snapshot into each sandbox config row's own payload
-- (switchImagePointer) and the tenant_skill_catalog row is fully owned by the
-- workspace (it may be deleted or re-registered locally), so a shared row
-- would leak one workspace's definition state into another. Assignment
-- therefore MATERIALIZES an ordinary tenant_skill_catalog row (bundle zip
-- copied into the workspace's own object storage via the existing
-- register-from-archive path) and records the link in a SEPARATE assignment
-- table:
--
--   assignment list  = platform_skill_assignments WHERE skill_id = ?
--                      AND deleted_at IS NULL
--   drift            = platform_skills.updated_at
--                      > platform_skill_assignments.pushed_at
--
-- The separate table (unlike 000097 where the materialized row is the
-- assignment) exists because a workspace may locally delete its catalog row:
-- the assignment row then survives as an empty shell that push "heals" by
-- re-materializing, rather than blocking the workspace's own delete flow.
--
-- Edits to a platform skill propagate only through the explicit push action;
-- installs onto sandbox configs stay a per-workspace decision in the existing
-- flow (no auto-install). The zip lives in the PLATFORM storage namespace
-- (sentinel tenant id), never in a workspace-configured backend.

DO $$ BEGIN RAISE NOTICE '[Migration 000098] Creating table: platform_skills'; END $$;
CREATE TABLE IF NOT EXISTS platform_skills (
    id            VARCHAR(36)   PRIMARY KEY,
    name          VARCHAR(255)  NOT NULL,
    version       VARCHAR(64),
    description   TEXT,
    instructions  TEXT,
    bundle_ref    VARCHAR(1024),
    bundle_sha256 VARCHAR(64),
    source        VARCHAR(1024),
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

-- Unique among live rows (mirrors platform_skills' siblings 000082/000097):
-- skill names are agent-visible identities and runtime directories
-- (/opt/weknora/tenant/skills/<name>), so duplicates at platform level would
-- collide in every workspace at once.
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_skills_name
    ON platform_skills (name) WHERE deleted_at IS NULL;

COMMENT ON COLUMN platform_skills.instructions IS 'SKILL.md body as parsed from the bundle; exposed to workspace admins through the file browser, not re-served to agents';
COMMENT ON COLUMN platform_skills.source IS 'Provenance only (@owner/slug, URL, or ''upload''); displayed as-is, never re-fetched automatically';
COMMENT ON COLUMN platform_skills.bundle_ref IS 'Object-storage ref of the bundle zip in the platform namespace (sentinel tenant id 1<<62)';

DO $$ BEGIN RAISE NOTICE '[Migration 000098] Creating table: platform_skill_assignments'; END $$;
CREATE TABLE IF NOT EXISTS platform_skill_assignments (
    id                     VARCHAR(36) PRIMARY KEY,
    tenant_id              BIGINT      NOT NULL,
    skill_id               VARCHAR(36) NOT NULL,
    materialized_catalog_id VARCHAR(36) NOT NULL DEFAULT '',
    pushed_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at             TIMESTAMPTZ
);

-- One live assignment per (skill, workspace).
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_skill_assignments
    ON platform_skill_assignments (skill_id, tenant_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_platform_skill_assignments_tenant
    ON platform_skill_assignments (tenant_id) WHERE deleted_at IS NULL;

COMMENT ON COLUMN platform_skill_assignments.materialized_catalog_id IS 'tenant_skill_catalog.id this assignment last materialized; the workspace may have deleted that row (empty or stale id) — push heals by re-materializing';
COMMENT ON COLUMN platform_skill_assignments.pushed_at IS 'platform_skills.updated_at snapshot at the last successful push/assignment; older than the skill updated_at means the assignment is drifting';

DO $$ BEGIN RAISE NOTICE '[Migration 000098] Linking materialized catalog rows to their source platform skill'; END $$;
ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS source_platform_skill_id VARCHAR(36) NULL;

COMMENT ON COLUMN tenant_skill_catalog.source_platform_skill_id IS 'platform_skills.id this row was materialized from; NULL = workspace self-built';

CREATE INDEX IF NOT EXISTS idx_tenant_skill_catalog_source_platform
    ON tenant_skill_catalog (source_platform_skill_id)
    WHERE deleted_at IS NULL AND source_platform_skill_id IS NOT NULL;
