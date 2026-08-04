-- 000003_seed_demo_users.down.sql
-- Reverses 000003_seed_demo_users.up.sql by deleting exactly the two rows
-- it inserted, identified by username. Not lossy beyond removing the seed
-- data itself -- if a later migration or manual step created media rows
-- owned by these users, 000004's down runs before this one in a full-down
-- sequence and already drops the referencing column.
DELETE FROM users WHERE username IN ('demo.reader', 'demo.curator');
