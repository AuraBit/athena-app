# media service

Athena's custom Go service: Postgres-backed media metadata, S3 uploads
(Plan 03-05), and Valkey-backed bearer-token sessions (Plan 03-04).

## API surface

| Method | Path        | Auth               | Purpose |
|--------|-------------|--------------------|---------|
| GET    | `/healthz`  | none               | Liveness only — never checks a downstream dependency (Plan 03-01) |
| POST   | `/login`    | none (credentials in body) | Returns a bearer token on success; identical 401 body for an unknown user and a wrong password (Plan 03-04) |
| POST   | `/logout`   | none (token best-effort) | Idempotent session invalidation (Plan 03-04) |
| POST   | `/upload`   | **required** — `Authorization: Bearer <token>` | Streams a JPEG or PNG into this environment's S3 bucket; rejected by magic bytes (not the client's `Content-Type` header) and bounded by a size cap (Plan 03-05) |
| GET    | `/media`    | none               | Lists every stored media entry, newest first (Plan 03-05, D-01: read is public) |
| GET    | `/media/<object_key>` | none      | Streams a stored object's bytes back with the content type recorded at upload time (Plan 03-05, D-01) |

`/upload` is the one route behind the session gate — everything else, including
`/media` and `/media/<object_key>`, is deliberately public (D-01): the
asymmetry proves the session middleware is a real enforcement boundary, not
a blanket applied to every route.

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
| `MEDIA_S3_BUCKET` | This environment's real media bucket name (Terraform's `media_bucket_name` output — never a name reconstructed by convention) |
| `MEDIA_S3_ENDPOINT` | LocalStack's address as seen from inside the app cluster (`http://host.k3d.internal:4566` — the in-cluster alias fixed in Plan 03-01, never the host's own loopback address) |
| `MEDIA_S3_REGION` | The simulated AWS region every LocalStack call targets |
| `MEDIA_S3_KEY_PREFIX` | Scopes every object this service writes and every list call it issues |
| `MEDIA_S3_ACCESS_KEY_ID` / `MEDIA_S3_SECRET_ACCESS_KEY` | LocalStack's simulated per-environment credentials (D-16) — non-secret by construction, the account-namespacing mechanism, not an authorisation credential. Plain values are acceptable **only** because of that; this pattern would be wrong against a real AWS account |
| `MEDIA_MAX_UPLOAD_SIZE_BYTES` | Upload size cap in bytes (default `8388608` — 8 MiB) |

All settings are read exactly once at process startup into a single
immutable `Config` struct — deliberately, per `internal/config/config.go`'s
own header comment (D-12). Do not add a hot-reload or lazy re-read path.

## Deliberate scope boundaries

Two things are missing from this service on purpose — not oversights, and
both are written down here so they read as visible scope rather than
apparent gaps:

- **No metrics endpoint, only basic logs (APP-03).** `/metrics` returns 404
  (proven by `internal/handlers/health_test.go#TestMetrics_Returns404`) and
  request logging is gin's default access log only — no structured logging
  library, no metrics registry. This is deliberate: Phase 4 instruments
  this exact service as its observability exercise, and this phase's job is
  to leave that instrumentation genuinely absent, not stubbed.
- **Uploads proxy through the service instead of using presigned S3 URLs
  (D-02).** The production-scale pattern for an upload endpoint like this
  is a presigned URL: the service asks S3 to mint a time-limited signed
  URL, and the client uploads directly to S3, never routing the bytes
  through the service at all. This estate cannot use that pattern here: the
  hostname LocalStack signs a URL against (`host.k3d.internal`, the
  in-cluster alias a pod uses to reach LocalStack) differs from the
  hostname a client actually reaches this service through
  (`media-<env>.athena.net`, via the Envoy Gateway) — a presigned URL
  minted by the service would carry a signature the client's own request,
  made from a different network view, cannot satisfy. `internal/upload.go`
  and `internal/storage/storage.go` both proxy every uploaded byte through
  the service instead, which is strictly more work for this process (and a
  worse production pattern at real scale) but is the only shape that
  actually works against this local topology.

## Demo walkthrough

```bash
# 1. Login as a seeded demo user (see "Demo credentials" above)
TOKEN=$(curl -sk -X POST https://media-dev.athena.net/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo.curator","password":"athena-demo-curator-2026"}' | jq -r .token)

# 2. Upload a real JPEG or PNG (protected — requires the token)
curl -sk -X POST https://media-dev.athena.net/upload \
  -H "Authorization: Bearer ${TOKEN}" \
  --data-binary @photo.jpg

# 3. List and fetch — both public, no token needed
curl -sk https://media-dev.athena.net/media
curl -sk https://media-dev.athena.net/media/<object_key> -o downloaded.jpg
```
