// Command media is the Athena media service's entrypoint. This phase's
// slice (Plan 03-01, Task 2) serves exactly one route, /healthz — no
// database, no cache, no S3, no metrics registry, no metrics route
// (APP-03: the absent metrics surface is deliberate, a Phase 4
// instrumentation exercise target, not an oversight).
package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/config"
	"github.com/AuraBit/athena-app/src/media/internal/handlers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: %v\n", err)
		os.Exit(1)
	}

	// Release mode: suppresses gin's debug-route dump on every startup —
	// still logs every request via gin.Default()'s Logger middleware below,
	// just without the extra noise. Basic request logging only — no
	// structured logging library, no metrics registry, no metrics route.
	// APP-03 is a deliberate scope boundary for this phase, not a gap to
	// quietly fill in.
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

	addr := ":" + cfg.HTTPPort
	fmt.Printf("media: listening on %s (environment=%s)\n", addr, cfg.Environment)
	if err := router.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "media: fatal: server error: %v\n", err)
		os.Exit(1)
	}
}
