-- 000002_create_users.up.sql
-- Creates the users table the login handler queries and 000003 seeds.
-- No profile fields: there is no registration endpoint, so nothing but an
-- identifier, a unique username, and a password-hash column is needed.
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL
);
