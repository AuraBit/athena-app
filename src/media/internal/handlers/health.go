// Package handlers holds the media service's HTTP handlers. This phase
// (Plan 03-01, Task 2) ships exactly one: the liveness handler behind
// /healthz.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health is a pure liveness signal: it performs no database, cache or S3
// call, and never will for the healthz route specifically — checking a
// downstream dependency here would make Kubernetes kill a perfectly healthy
// process just because e.g. Postgres is briefly unreachable. It always
// returns 200 with a small JSON body carrying a status field.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// verify build+scan complete cleanly
