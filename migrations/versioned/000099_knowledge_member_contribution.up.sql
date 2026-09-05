-- 000099: co-maintain knowledge bases (member contribution model).
--
-- Product rules this backs:
--   1. Only workspace Admin+ may create/copy/duplicate KBs (route guard
--      change, no schema here).
--   2. A KB creator/Admin may mark the KB 允许空间成员共同维护 — that is
--      knowledge_bases.allow_member_contribute below. Default FALSE: a KB
--      stays admin/creator-only until explicitly opened up.
--   3. In a co-maintain KB, ordinary members (Contributor+) may ADD
--      knowledge and afterwards manage ONLY the items they added. That
--      per-item ownership is knowledges.creator_id, stamped by the
--      repository on every create from the acting user in the context.
--      Legacy rows keep '' = "added before tracking existed"; members
--      cannot manage those, Admin+/KB creator always can.
--
-- No backfill: the flag is opt-in per KB, and old knowledge rows belong
-- to the admin/creator path by definition.

DO $$ BEGIN RAISE NOTICE '[Migration 000099] Adding knowledge_bases.allow_member_contribute'; END $$;
ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS allow_member_contribute BOOLEAN NOT NULL DEFAULT FALSE;

DO $$ BEGIN RAISE NOTICE '[Migration 000099] Adding knowledges.creator_id'; END $$;
ALTER TABLE knowledges
    ADD COLUMN IF NOT EXISTS creator_id VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_creator_id
    ON knowledges(creator_id)
    WHERE creator_id <> '';

DO $$ BEGIN RAISE NOTICE '[Migration 000099] member contribution model ready'; END $$;
