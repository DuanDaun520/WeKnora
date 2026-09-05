-- 000099 down: revert the co-maintain model. Route guards go back to
-- admin/creator-only in code; here we only drop the two columns and the
-- partial index. Dropping knowledges.creator_id loses per-item ownership
-- data — acceptable for a down migration (permission metadata, not
-- content).

DROP INDEX IF EXISTS idx_knowledges_creator_id;
ALTER TABLE knowledges
    DROP COLUMN IF EXISTS creator_id;
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS allow_member_contribute;
