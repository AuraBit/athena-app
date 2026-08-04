// Package config loads the media service's configuration exactly once, at
// process startup, into a single immutable struct — never re-read
// afterward.
//
// This shape is deliberate and load-bearing, not an oversight to be
// "helpfully" repaired by a future reader: it deliberately reproduces the
// staleness-prone class of bug behind the project owner's real "PHP caches
// config pre-startup" production war story. Phase 5's secret-rotation drill
// depends on this service failing to pick up a rotated credential without a
// restart — first demonstrating that failure live, then fixing it with a
// Vault Agent / reload pattern. Do not add a background reloader, a
// SIGHUP handler, or a periodic re-read here; that would silently remove
// the exact defect Phase 5 exists to find and fix (CONTEXT.md D-12).
//
// This struct started (Plan 03-01, Task 2) with only the HTTP listen port
// and the environment name. Plan 03-04's Task 1 grew it with the Postgres
// connection string, and Plan 03-04's Task 2 grew it further with the
// Valkey address and session lifetime. Plan 03-05 adds the S3 settings the
// upload/fetch/list handlers need — bucket, endpoint, region, key prefix,
// max upload size and the simulated per-environment credential pair — the
// same way: new required-or-defaulted fields on the one struct, never a
// second config type or a second loader.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the media service's entire runtime configuration, populated
// once by Load and never mutated after that.
type Config struct {
	// HTTPPort is the port the HTTP server listens on, e.g. "8080".
	HTTPPort string
	// Environment is this deployment's environment name (dev/stg/prod),
	// used only for identification (e.g. log context) — never branches
	// business logic on it.
	Environment string
	// DatabaseURL is the Postgres connection string
	// (postgres://user:pass@host:port/db?sslmode=disable) the pgx pool in
	// internal/db is constructed from.
	DatabaseURL string
	// ValkeyAddr is the media-session Valkey instance's address
	// (host:port) — deliberately never the cart instance's address; the
	// two are separately named services (D-18), and pointing this at the
	// cart instance is a configuration error a reader can see because the
	// env var name and the Service name it must match are both explicit.
	ValkeyAddr string
	// SessionTTLSeconds is the server-side session lifetime, in seconds,
	// applied as a Valkey key TTL at session creation (internal/session).
	// Exposed as configuration rather than a literal buried in that
	// package so it is visible and tunable.
	SessionTTLSeconds int
	// S3Bucket is this environment's real media bucket name, the exact
	// output Terraform's data-storage stack produces (media_bucket_name) —
	// never a name reconstructed by convention.
	S3Bucket string
	// S3Endpoint is LocalStack's address as seen from inside the app
	// cluster: the host.k3d.internal alias fixed in Plan 03-01, not the
	// loopback address that only works from the host. A presigned URL
	// minted against this endpoint would carry a signature the client's
	// own request (from a different network view) cannot satisfy — this is
	// exactly why D-02 proxies uploads through the service instead of
	// generating presigned URLs; see internal/storage/storage.go's header.
	S3Endpoint string
	// S3Region is the simulated AWS region every LocalStack call targets.
	S3Region string
	// S3KeyPrefix scopes every object this service writes and every list
	// call it issues, so storage.List never has to reason about objects
	// outside the service's own namespace.
	S3KeyPrefix string
	// S3AccessKeyID and S3SecretAccessKey are LocalStack's simulated
	// per-environment credentials (D-16): non-secret by construction, they
	// are the account-namespacing mechanism, not an authorisation
	// credential. Plain values are acceptable ONLY because of that —
	// this pattern would be wrong the moment these targeted a real AWS
	// account. See values-dev.yaml's comment for the same caveat stated at
	// the chart layer.
	S3AccessKeyID     string
	S3SecretAccessKey string
	// MaxUploadSizeBytes bounds the upload handler's body-limiting reader
	// (internal/handlers/upload.go) — configuration, not a literal, so the
	// acceptance test can drive it down to something cheap to exercise.
	MaxUploadSizeBytes int64
}

// Load reads every setting from environment variables a single time. A
// missing required variable returns an error naming that variable — the
// caller (cmd/media/main.go) is expected to treat that as fatal and exit
// non-zero rather than starting the service in a degraded state.
func Load() (*Config, error) {
	port := os.Getenv("MEDIA_HTTP_PORT")
	if port == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_HTTP_PORT is not set")
	}

	environment := os.Getenv("MEDIA_ENVIRONMENT")
	if environment == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_ENVIRONMENT is not set")
	}

	databaseURL := os.Getenv("MEDIA_DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_DATABASE_URL is not set")
	}

	valkeyAddr := os.Getenv("MEDIA_VALKEY_ADDR")
	if valkeyAddr == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_VALKEY_ADDR is not set")
	}

	// A documented default (thirty minutes) rather than a required
	// variable — a reasonable default for a demo service, but still a
	// config value, never a literal buried in internal/session.
	sessionTTLSeconds := 1800
	if raw := os.Getenv("MEDIA_SESSION_TTL_SECONDS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("MEDIA_SESSION_TTL_SECONDS must be an integer number of seconds: %w", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("MEDIA_SESSION_TTL_SECONDS must be positive, got %d", parsed)
		}
		sessionTTLSeconds = parsed
	}

	s3Bucket := os.Getenv("MEDIA_S3_BUCKET")
	if s3Bucket == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_BUCKET is not set")
	}

	s3Endpoint := os.Getenv("MEDIA_S3_ENDPOINT")
	if s3Endpoint == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_ENDPOINT is not set")
	}

	s3Region := os.Getenv("MEDIA_S3_REGION")
	if s3Region == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_REGION is not set")
	}

	s3KeyPrefix := os.Getenv("MEDIA_S3_KEY_PREFIX")
	if s3KeyPrefix == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_KEY_PREFIX is not set")
	}

	s3AccessKeyID := os.Getenv("MEDIA_S3_ACCESS_KEY_ID")
	if s3AccessKeyID == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_ACCESS_KEY_ID is not set")
	}

	s3SecretAccessKey := os.Getenv("MEDIA_S3_SECRET_ACCESS_KEY")
	if s3SecretAccessKey == "" {
		return nil, fmt.Errorf("required environment variable MEDIA_S3_SECRET_ACCESS_KEY is not set")
	}

	// A documented default (8 MiB) rather than a required variable — small
	// and proportionate for a demo image service, and still configuration,
	// never a literal buried in internal/handlers/upload.go.
	maxUploadSizeBytes := int64(8 * 1024 * 1024)
	if raw := os.Getenv("MEDIA_MAX_UPLOAD_SIZE_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("MEDIA_MAX_UPLOAD_SIZE_BYTES must be an integer number of bytes: %w", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("MEDIA_MAX_UPLOAD_SIZE_BYTES must be positive, got %d", parsed)
		}
		maxUploadSizeBytes = parsed
	}

	return &Config{
		HTTPPort:           port,
		Environment:        environment,
		DatabaseURL:        databaseURL,
		ValkeyAddr:         valkeyAddr,
		SessionTTLSeconds:  sessionTTLSeconds,
		S3Bucket:           s3Bucket,
		S3Endpoint:         s3Endpoint,
		S3Region:           s3Region,
		S3KeyPrefix:        s3KeyPrefix,
		S3AccessKeyID:      s3AccessKeyID,
		S3SecretAccessKey:  s3SecretAccessKey,
		MaxUploadSizeBytes: maxUploadSizeBytes,
	}, nil
}
