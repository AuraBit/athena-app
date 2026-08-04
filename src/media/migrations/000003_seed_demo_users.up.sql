-- 000003_seed_demo_users.up.sql
-- Seeds a small fixed set of demo users (D-05). The password_hash values
-- below are bcrypt hashes (cost factor 12) generated ONCE, offline, with
-- golang.org/x/crypto/bcrypt -- never computed at migration time from a
-- literal in this file, and never a plaintext or reversibly-encoded
-- password. The plaintext passwords these hashes correspond to are NOT
-- recorded here; they live in
-- estate/athena-app/src/media/README.md's "Demo credentials" section, so
-- the demo is usable without the credentials being an artefact of the
-- schema itself.
INSERT INTO users (username, password_hash) VALUES
    ('demo.reader',  '$2a$12$gpK35yom5qWUGO2vj9AFD.qzjpJ2YPHFznKW2oMM6hsJb/oazOXLu'),
    ('demo.curator', '$2a$12$deerZ/8tJ.WYnBTjRgq2VuKAzmn4RILbya2zehzXwd52rJAXqTMqe');
