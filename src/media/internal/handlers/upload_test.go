package handlers_test

// upload_test.go — written first, watched to fail (RED) before upload.go
// existed, then implemented (GREEN). Runs entirely behind
// MediaStorer/MediaInserter fakes (httptest, no external services) — the
// fast unit run covers the 401 path, the spoofed-content-type rejection,
// the genuinely-mislabelled-image acceptance, the size-cap rejection, and
// the no-partial-write property this task's <behavior> block describes.
// The real LocalStack round trip lives in internal/storage's own
// integration tests and Plan 03-05 Task 3's live proof.

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/db"
	"github.com/AuraBit/athena-app/src/media/internal/handlers"
)

// fakeStorer is a minimal in-memory handlers.MediaStorer that also counts
// calls, so tests can assert "zero put calls" on a rejected upload.
type fakeStorer struct {
	mu       sync.Mutex
	objects  map[string][]byte
	putCalls int
	delCalls int
	putErr   error
}

func newFakeStorer() *fakeStorer {
	return &fakeStorer{objects: make(map[string][]byte)}
}

func (f *fakeStorer) Put(_ context.Context, key, _ string, body io.Reader, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putCalls++
	if f.putErr != nil {
		return f.putErr
	}
	data, _ := io.ReadAll(body)
	f.objects[key] = data
	return nil
}

func (f *fakeStorer) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, errors.New("not used in upload tests")
}

func (f *fakeStorer) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delCalls++
	delete(f.objects, key)
	return nil
}

// fakeMediaInserter is a minimal in-memory handlers.MediaInserter.
type fakeMediaInserter struct {
	mu          sync.Mutex
	insertCalls int
	insertErr   error
	inserted    []string // object keys, in insertion order
}

func (f *fakeMediaInserter) InsertMedia(_ context.Context, objectKey, contentType string, sizeBytes int64, ownerID string) (*db.MediaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insertCalls++
	if f.insertErr != nil {
		return nil, f.insertErr
	}
	f.inserted = append(f.inserted, objectKey)
	return &db.MediaItem{
		ID:          "media-id",
		ObjectKey:   objectKey,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		OwnerID:     ownerID,
	}, nil
}

const testUserIDKey = "sessionUserID" // must match auth.go's contextUserIDKey exactly

// newUploadRouter builds a router with a stand-in session-setting
// middleware (rather than the real RequireSession) so these tests can
// drive both the "authenticated" and "unauthenticated" paths directly
// without a live Valkey.
func newUploadRouter(h *handlers.UploadHandlers, authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/", func(c *gin.Context) {
		if !authenticated {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(testUserIDKey, "owner-1")
		c.Next()
	})
	group.POST("/upload", h.Upload)
	return r
}

func realPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

func realJPEGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding test JPEG: %v", err)
	}
	return buf.Bytes()
}

func newUploadHandlers(storer *fakeStorer, inserter *fakeMediaInserter, maxBytes int64) *handlers.UploadHandlers {
	return &handlers.UploadHandlers{
		Storage:            storer,
		Media:              inserter,
		KeyPrefix:          "uploads/",
		MaxUploadSizeBytes: maxBytes,
	}
}

func TestUpload_ValidTokenAndRealJPEG_Returns201AndStoresObject(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	body := realJPEGBytes(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if storer.putCalls != 1 {
		t.Fatalf("expected exactly 1 Put call, got %d", storer.putCalls)
	}
	if inserter.insertCalls != 1 {
		t.Fatalf("expected exactly 1 InsertMedia call, got %d", inserter.insertCalls)
	}
}

func TestUpload_NoAuthorizationHeader_Returns401AndWritesNothing(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), false)

	body := realJPEGBytes(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if storer.putCalls != 0 {
		t.Fatalf("expected zero Put calls on an unauthenticated request, got %d", storer.putCalls)
	}
	if inserter.insertCalls != 0 {
		t.Fatalf("expected zero InsertMedia calls on an unauthenticated request, got %d", inserter.insertCalls)
	}
}

func TestUpload_SpoofedContentType_RejectedAndWritesNothing(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	// Plain text bytes, but the client CLAIMS it is a JPEG. The header
	// must never be consulted for the accept/reject decision.
	body := []byte("this is definitely not an image, just plain text bytes")
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for spoofed content-type, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if storer.putCalls != 0 {
		t.Fatalf("expected zero Put calls on a rejected upload, got %d", storer.putCalls)
	}
	if inserter.insertCalls != 0 {
		t.Fatalf("expected zero InsertMedia calls on a rejected upload, got %d", inserter.insertCalls)
	}
}

func TestUpload_GenuinePNGMislabelledAsPlainText_IsAccepted(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	// A REAL png, but the client's header claims plain text — the bytes
	// decide, not the header, so this must be accepted.
	body := realPNGBytes(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for a genuine PNG mislabelled as text/plain, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if storer.putCalls != 1 {
		t.Fatalf("expected exactly 1 Put call, got %d", storer.putCalls)
	}
}

func TestUpload_ExceedsSizeCap_RejectedAndWritesNothing(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	// A tiny cap the acceptance criteria expects the test to drive down
	// cheaply, per the plan's flagged-assumption note.
	const cap = 16
	router := newUploadRouter(newUploadHandlers(storer, inserter, cap), true)

	body := realJPEGBytes(t) // guaranteed larger than 16 bytes
	if len(body) <= cap {
		t.Fatalf("test fixture JPEG (%d bytes) must exceed the test cap (%d bytes)", len(body), cap)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized upload, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if storer.putCalls != 0 {
		t.Fatalf("expected zero Put calls on an oversized upload, got %d", storer.putCalls)
	}
	if inserter.insertCalls != 0 {
		t.Fatalf("expected zero InsertMedia calls on an oversized upload, got %d", inserter.insertCalls)
	}
}

func TestUpload_DatabaseInsertFails_DeletesTheOrphanedObject(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{insertErr: errors.New("simulated insert failure")}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	body := realJPEGBytes(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the database insert fails, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if storer.putCalls != 1 {
		t.Fatalf("expected exactly 1 Put call (the write that gets orphaned), got %d", storer.putCalls)
	}
	if storer.delCalls != 1 {
		t.Fatalf("expected exactly 1 Delete call cleaning up the orphaned object, got %d", storer.delCalls)
	}
	if len(storer.objects) != 0 {
		t.Fatalf("expected the fake storer to hold zero objects after cleanup, got %d", len(storer.objects))
	}
}

func TestUpload_SameFilenameTwice_ProducesTwoDistinctKeys(t *testing.T) {
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	body := realJPEGBytes(t)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "image/jpeg")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload %d: expected 201, got %d", i, rec.Code)
		}
	}

	if len(inserter.inserted) != 2 {
		t.Fatalf("expected 2 inserted media rows, got %d", len(inserter.inserted))
	}
	if inserter.inserted[0] == inserter.inserted[1] {
		t.Fatalf("expected two distinct object keys for two uploads of the same filename, got the same key twice: %q", inserter.inserted[0])
	}
}

func TestUpload_NeverReadsContentTypeHeaderForAcceptDecision(t *testing.T) {
	// Structural guard alongside the plan's grep-based acceptance check:
	// a PNG declared as an unrelated, unsupported type must still be
	// accepted purely from its bytes.
	storer := newFakeStorer()
	inserter := &fakeMediaInserter{}
	router := newUploadRouter(newUploadHandlers(storer, inserter, 8*1024*1024), true)

	body := realPNGBytes(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 (bytes are a genuine PNG regardless of header), got %d", rec.Code)
	}
}
