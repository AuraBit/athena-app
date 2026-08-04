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
// connection string, and this task (Plan 03-04, Task 2) grows it further
// with the Valkey address and session lifetime — never a second config
// type or a second loader. Plan 03-05 will add S3 settings the same way.
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

	return &Config{
		HTTPPort:          port,
		Environment:       environment,
		DatabaseURL:       databaseURL,
		ValkeyAddr:        valkeyAddr,
		SessionTTLSeconds: sessionTTLSeconds,
	}, nil
}
