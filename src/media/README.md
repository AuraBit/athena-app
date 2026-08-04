# media service

Athena's custom Go service: Postgres-backed media metadata, S3 uploads
(Plan 03-05), and Valkey-backed bearer-token sessions (this plan, 03-04).

## Demo credentials

Migration `000003_seed_demo_users.up.sql` seeds two demo users with
bcrypt-hashed passwords (cost factor 12). The plaintext passwords are
recorded here — not in the migration itself — so the demo is usable without
the credentials being an artefact of the schema (D-05).

| Username       | Password                    | Intended role                          |
|----------------|------------------------------|-----------------------------------------|
| `demo.reader`  | `athena-demo-reader-2026`    | Read-only demo login                    |
| `demo.curator` | `athena-demo-curator-2026`   | The account 000004's backfill assigns as owner of pre-existing media rows |

These are local-only demo credentials for a $0, non-production study
project — never use this pattern (fixed passwords committed to a public
docs file) for anything internet-facing.

## Local iteration

Migrations and the data/session layer's integration tests run against a
throwaway Postgres and Valkey, e.g.:

```bash
docker run -d --name athena-media-pg-dev -e POSTGRES_USER=media \
  -e POSTGRES_PASSWORD=media -e POSTGRES_DB=media -p 15432:5432 postgres:18
docker run -d --name athena-media-valkey-dev -p 16379:6379 valkey/valkey:8
```

Run the migration tool by hand against either the throwaway container or
the in-cluster Postgres (see `estate/athena-gitops/charts/athena/templates/datastores-postgres.yaml`'s
header comment for the exact in-cluster command, port-forwarded):

```bash
migrate -path migrations -database "$MEDIA_DATABASE_URL" up
```

## Environment variables

| Variable | Purpose |
|----------|---------|
| `MEDIA_HTTP_PORT` | HTTP listen port |
| `MEDIA_ENVIRONMENT` | Deployment environment name (dev/stg/prod), log context only |
| `MEDIA_DATABASE_URL` | Postgres connection string (`postgres://user:pass@host:port/db?sslmode=disable`) |
| `MEDIA_VALKEY_ADDR` | Media-session Valkey address (`host:port`) — never the cart instance |
| `MEDIA_SESSION_TTL_SECONDS` | Session lifetime in seconds (default `1800` — 30 minutes) |

All settings are read exactly once at process startup into a single
immutable `Config` struct — deliberately, per `internal/config/config.go`'s
own header comment (D-12). Do not add a hot-reload or lazy re-read path.
