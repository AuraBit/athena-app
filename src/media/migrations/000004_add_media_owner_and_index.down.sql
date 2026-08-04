-- 000004_add_media_owner_and_index.down.sql
-- LOSSY DOWN: dropping owner_id discards which user owns each media row.
-- The up cannot recover this on a subsequent up from this point -- a fresh
-- up would re-backfill every row to 'demo.curator' again rather than
-- restoring whatever real per-row ownership existed before this down ran.
-- This is disclosed here deliberately (per this plan's prohibition on
-- silent lossy downs) rather than left for a future operator to discover
-- mid-rollback. Acceptable for this chain's purpose: CI's up/down/up drill
-- only asserts final SCHEMA equality, never row-level data equality, and a
-- real production rollback of this migration would carry the same
-- unavoidable trade-off (the column is being removed, not renamed).
DROP INDEX IF EXISTS idx_media_owner_id_created_at;
ALTER TABLE media DROP COLUMN IF EXISTS owner_id;
