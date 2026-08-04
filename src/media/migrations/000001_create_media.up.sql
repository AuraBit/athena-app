-- 000001_create_media.up.sql
-- The first link in the evolving golang-migrate chain (D-04). This creates
-- only the media table's original shape: an identifier, the object key the
-- upload path (Plan 03-05) writes into S3, a declared content type, a byte
-- size, and a creation timestamp. The owner column and its index arrive in
-- 000004 — deliberately not here, so that migration genuinely alters this
-- table rather than replaying a snapshot.
CREATE TABLE media (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_key  TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL CHECK (size_bytes >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
