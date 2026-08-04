package handlers_test

// fetch_test.go — written first, watched to fail (RED) before fetch.go
// existed, then implemented (GREEN). Runs entirely behind fakes (httptest,
// no external services) — covers the anonymous list, the empty list, the
// successful fetch, the unknown-key 404, the traversal rejection, and the
// identical-response-for-authenticated-and-anonymous property this task's
// <behavior> block describes.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/db"
	"github.com/AuraBit/athena-app/src/media/internal/handlers"
	"github.com/AuraBit/athena-app/src/media/internal/storage"
)

// fakeLister is a minimal in-memory handlers.MediaLister.
type fakeLister struct {
	all   []db.MediaItem
	byKey map[string]*db.MediaItem
}

func newFakeLister(items ...db.MediaItem) *fakeLister {
	f := &fakeLister{byKey: make(map[string]*db.MediaItem)}
	for i := range items {
		item := items[i]
		f.all = append(f.all, item)
		f.byKey[item.ObjectKey] = &item
	}
	return f
}

func (f *fakeLister) ListAllMedia(_ context.Context) ([]db.MediaItem, error) {
	return f.all, nil
}

func (f *fakeLister) GetMediaByKey(_ context.Context, objectKey string) (*db.MediaItem, error) {
	item, ok := f.byKey[objectKey]
	if !ok {
		return nil, db.ErrNotFound
	}
	return item, nil
}

// fetchOnlyStorer is a minimal handlers.MediaStorer for fetch tests — Put
// and Delete are never exercised here, only Get.
type fetchOnlyStorer struct {
	objects map[string][]byte
}

func (f *fetchOnlyStorer) Put(_ context.Context, key, _ string, body io.Reader, _ int64) error {
	data, _ := io.ReadAll(body)
	f.objects[key] = data
	return nil
}

func (f *fetchOnlyStorer) Get(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fetchOnlyStorer) Delete(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

// newFetchRouter registers List/Fetch exactly as cmd/media/main.go does —
// outside any session-gated group — plus a stand-in "authenticated" route
// carrying the same handlers behind a fake session-setting middleware, so
// a single test can assert identical responses for both callers.
func newFetchRouter(h *handlers.FetchHandlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/media", h.List)
	r.GET("/media/*key", h.Fetch)
	authed := r.Group("/authed", func(c *gin.Context) {
		c.Set(testUserIDKey, "owner-1")
		c.Next()
	})
	authed.GET("/media", h.List)
	return r
}

func TestList_NoAuthorizationHeader_Returns200WithEntries(t *testing.T) {
	item := db.MediaItem{ID: "1", ObjectKey: "uploads/abc", ContentType: "image/png", SizeBytes: 10, OwnerID: "owner-1", CreatedAt: time.Now()}
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(item), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/media", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Media []map[string]any `json:"media"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(body.Media) != 1 {
		t.Fatalf("expected 1 media entry, got %d", len(body.Media))
	}
}

func TestList_EmptyBucket_Returns200WithEmptyCollectionNotError(t *testing.T) {
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/media", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty list, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Media []map[string]any `json:"media"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Media == nil {
		t.Fatal("expected an empty array, got a null 'media' field")
	}
	if len(body.Media) != 0 {
		t.Fatalf("expected zero entries for an empty bucket, got %d", len(body.Media))
	}
}

func TestList_AuthenticatedAndAnonymous_ReceiveIdenticalResponses(t *testing.T) {
	item := db.MediaItem{ID: "1", ObjectKey: "uploads/abc", ContentType: "image/png", SizeBytes: 10, OwnerID: "owner-1", CreatedAt: time.Now()}
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(item), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	anonReq := httptest.NewRequest(http.MethodGet, "/media", nil)
	anonRec := httptest.NewRecorder()
	router.ServeHTTP(anonRec, anonReq)

	authReq := httptest.NewRequest(http.MethodGet, "/authed/media", nil)
	authReq.Header.Set("Authorization", "Bearer irrelevant-to-these-handlers")
	authRec := httptest.NewRecorder()
	router.ServeHTTP(authRec, authReq)

	if anonRec.Code != authRec.Code {
		t.Fatalf("expected identical status codes, got anon=%d authed=%d", anonRec.Code, authRec.Code)
	}
	if anonRec.Body.String() != authRec.Body.String() {
		t.Fatalf("expected identical bodies, got anon=%q authed=%q", anonRec.Body.String(), authRec.Body.String())
	}
}

func TestFetch_StoredKey_Returns200WithBytesAndRecordedContentType(t *testing.T) {
	key := "uploads/abc"
	content := []byte("fake-png-bytes")
	item := db.MediaItem{ID: "1", ObjectKey: key, ContentType: "image/png", SizeBytes: int64(len(content)), OwnerID: "owner-1", CreatedAt: time.Now()}
	storer := &fetchOnlyStorer{objects: map[string][]byte{key: content}}
	h := &handlers.FetchHandlers{Storage: storer, Media: newFakeLister(item), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/media/"+key, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("expected body %q, got %q", content, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected Content-Type recorded on the media row (image/png), got %q", got)
	}
}

func TestFetch_UnknownKey_Returns404(t *testing.T) {
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/media/uploads/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown key, got %d", rec.Code)
	}
}

func TestFetch_TraversalKey_Returns404IdenticalToMissingKey(t *testing.T) {
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	missingReq := httptest.NewRequest(http.MethodGet, "/media/uploads/does-not-exist", nil)
	missingRec := httptest.NewRecorder()
	router.ServeHTTP(missingRec, missingReq)

	traversalReq := httptest.NewRequest(http.MethodGet, "/media/uploads/../../etc/passwd", nil)
	traversalRec := httptest.NewRecorder()
	router.ServeHTTP(traversalRec, traversalReq)

	if traversalRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a traversal key, got %d (body: %s)", traversalRec.Code, traversalRec.Body.String())
	}
	if traversalRec.Code != missingRec.Code || traversalRec.Body.String() != missingRec.Body.String() {
		t.Fatalf("expected a traversal key to return the SAME response as a missing key (no oracle), got traversal=%d/%q missing=%d/%q",
			traversalRec.Code, traversalRec.Body.String(), missingRec.Code, missingRec.Body.String())
	}
}

func TestFetch_KeyEscapingPrefix_Returns404(t *testing.T) {
	h := &handlers.FetchHandlers{Storage: &fetchOnlyStorer{objects: map[string][]byte{}}, Media: newFakeLister(), KeyPrefix: "uploads/"}
	router := newFetchRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/media/other-prefix/secret", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a key outside this service's prefix, got %d", rec.Code)
	}
}
