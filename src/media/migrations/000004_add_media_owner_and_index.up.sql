-- 000004_add_media_owner_and_index.up.sql
-- The migration that makes this chain a genuine evolution, not a snapshot:
-- it alters a table 000001 created, referencing a table 000002 created and
-- 000003 seeded. Adds the owner column the upload path (Plan 03-05) sets on
-- insert, and the index the list query uses.
--
-- Column added nullable first so ALTER TABLE succeeds against any existing
-- rows, backfilled to the seeded 'demo.curator' user (the account intended
-- for uploads, per the README's "Demo credentials" section), then tightened
-- to NOT NULL once every row has an owner. This order is the standard
-- add-nullable / backfill / tighten pattern for altering a populated table
-- without a lock-window failure.
ALTER TABLE media ADD COLUMN owner_id UUID REFERENCES users(id);

UPDATE media
SET owner_id = (SELECT id FROM users WHERE username = 'demo.curator')
WHERE owner_id IS NULL;

ALTER TABLE media ALTER COLUMN owner_id SET NOT NULL;

-- The list query filters/orders by owner and creation time; this index is
-- what makes that query use an index scan rather than a sequential scan as
-- the media table grows.
CREATE INDEX idx_media_owner_id_created_at ON media (owner_id, created_at DESC);
