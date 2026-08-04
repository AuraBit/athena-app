-- 000002_create_users.down.sql
-- Reverses 000002_create_users.up.sql. 000004 adds a foreign key from
-- media.owner_id to this table; that migration's own down drops the
-- referencing column first, so by the time this down runs (during a full
-- down sequence) no other table still references users.
DROP TABLE IF EXISTS users;
