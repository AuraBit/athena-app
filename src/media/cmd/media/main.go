// Command media is the Athena media service's entrypoint. This phase's
// slices add: /healthz (Plan 03-01, Task 2 — still no metrics registry, no
// metrics route; APP-03's absent metrics surface is deliberate, a Phase 4
// instrumentation exercise target, not an oversight), Postgres-backed
// users/media and Valkey-backed bearer sessions (Plan 03-04): /login,
// /logout, and a session-gated route group; and now (Plan 03-05) the
// session-gated upload route plus the public fetch/list routes, backed by
// a startup-built S3 client (internal/storage).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/config"
	"github.com/AuraBit/athena-app/src/media/internal/db"
	"github.com/AuraBit/athena-app/src/media/internal/handlers"
	"github.com/AuraBit/athena-app/src/media/internal/session"
	"github.com/AuraBit/athena-app/src/media/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// The pgx pool and the Valkey session store are constructed once here,
	// alongside the config they're built from — same "initialised once at
	// startup" shape as cfg itself (D-12). Neither is ever reconstructed
	// or re-pointed at a different address after this.
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: connecting to Postgres: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	sessions, err := session.NewStore(cfg.ValkeyAddr, cfg.SessionTTLSeconds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: connecting to Valkey: %v\n", err)
		os.Exit(1)
	}
	defer sessions.Close()

	// The S3 client is built once here too, alongside pool and sessions —
	// same "initialised once at startup" shape (D-12). Targets
	// cfg.S3Endpoint (the host.k3d.internal alias fixed in Plan 03-01),
	// never reconstructed or re-pointed afterward.
	objectStore, err := storage.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: connecting to S3: %v\n", err)
		os.Exit(1)
	}

	auth := &handlers.AuthHandlers{Users: pool, Sessions: sessions}
	upload := &handlers.UploadHandlers{
		Storage:            objectStore,
		Media:              pool,
		KeyPrefix:          cfg.S3KeyPrefix,
		MaxUploadSizeBytes: cfg.MaxUploadSizeBytes,
	}
	fetch := &handlers.FetchHandlers{Storage: objectStore, Media: pool, KeyPrefix: cfg.S3KeyPrefix}

	// Release mode: suppresses gin's debug-route dump on every startup —
	// still logs every request via gin.Default()'s Logger middleware below,
	// just without the extra noise. Basic request logging only — no
	// structured logging library, no metrics registry, no metrics route.
	// APP-03 is a deliberate scope boundary for this phase, not a gap to
	// quietly fill in. Request logging never includes a token, password or
	// hash (T-03-19) — gin's default Logger middleware logs only method,
	// path, status and latency, never the body or headers.
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	// This service is reached only through the in-cluster Envoy Gateway
	// (Task 2's HTTPRoute) — it never sees a real internet-facing client
	// directly, and nothing here reads X-Forwarded-For, so trusting no
	// proxies is the correct, minimal-surface default (Rule 2: gin warns
	// loudly that trusting all proxies by default is unsafe).
	if err := router.SetTrustedProxies(nil); err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: %v\n", err)
		os.Exit(1)
	}

	router.GET("/healthz", handlers.Health)
	router.POST("/login", auth.Login)
	router.POST("/logout", auth.Logout)

	// Public fetch/list endpoints (D-01: read is public, write is
	// protected) — registered directly on router, never on the
	// session-gated group below, and never inspecting the Authorization
	// header themselves.
	router.GET("/media", fetch.List)
	router.GET("/media/*key", fetch.Fetch)

	// The session-gated route group. Plan 03-05's upload endpoint is the
	// first (and, this phase, only) route on it — the gate runs before any
	// body byte is read (T-03-25).
	protected := router.Group("/", auth.RequireSession)
	protected.POST("/upload", upload.Upload)

	addr := ":" + cfg.HTTPPort
	fmt.Printf("media: listening on %s (environment=%s)\n", addr, cfg.Environment)
	if err := router.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: server error: %v\n", err)
		os.Exit(1)
	}
}
