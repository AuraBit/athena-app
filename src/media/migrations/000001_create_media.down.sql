-- 000001_create_media.down.sql
-- Reverses 000001_create_media.up.sql exactly: drops the media table this
-- migration created. Not lossy in any way this file needs to disclose --
-- everything the up created is removed here and nothing else exists yet
-- for this table to have accumulated.
DROP TABLE IF EXISTS media;
