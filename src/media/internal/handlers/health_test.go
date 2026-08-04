package handlers_test

// health_test.go — written first, watched to fail (RED) before health.go
// existed. Asserts the two load-bearing behaviors of this task's tracer
// slice (Plan 03-01, Task 2):
//   - GET /healthz returns HTTP 200 with a JSON body carrying a status field
//   - GET /metrics returns HTTP 404 — no metrics surface exists in this
//     phase (APP-03); this is proven structurally, by never registering a
//     /metrics route at all, not by an explicit handler that returns 404.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/handlers"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", handlers.Health)
	return r
}

func TestHealthz_Returns200WithStatusField(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}

	if _, ok := body["status"]; !ok {
		t.Fatalf("response JSON has no 'status' field: %v", body)
	}
}

func TestMetrics_Returns404(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for /metrics (no metrics surface in this phase, APP-03), got %d", rec.Code)
	}
}
