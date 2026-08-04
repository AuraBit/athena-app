// fetch.go — the public list and fetch endpoints (D-01, T-03-28).
//
// D-01 makes read public and write protected, and that asymmetry is the
// point of this slice: it proves the session middleware (auth.go's
// RequireSession) is a real enforcement boundary rather than a blanket
// applied to everything. Both handlers here are registered outside the
// session-gated route group in cmd/media/main.go, and an authenticated
// caller gets an identical response to an anonymous one — neither handler
// ever inspects the Authorization header or the session gate's context
// key.
package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/db"
	"github.com/AuraBit/athena-app/src/media/internal/storage"
)

// MediaLister is the subset of internal/db's Pool that List and Fetch
// need.
type MediaLister interface {
	ListAllMedia(ctx context.Context) ([]db.MediaItem, error)
	GetMediaByKey(ctx context.Context, objectKey string) (*db.MediaItem, error)
}

// FetchHandlers holds the dependencies List and Fetch share. Constructed
// once in cmd/media/main.go from the same startup-built storage.Client and
// db.Pool every other handler uses.
type FetchHandlers struct {
	Storage   MediaStorer
	Media     MediaLister
	KeyPrefix string
}

// mediaListEntry is one row of List's JSON response body.
type mediaListEntry struct {
	ID          string `json:"id"`
	ObjectKey   string `json:"object_key"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	OwnerID     string `json:"owner_id"`
	CreatedAt   string `json:"created_at"`
}

// List returns every stored media entry, newest first, with no
// Authorization header required (D-01). An empty table returns 200 with an
// empty "media" array — never an error and never a null body, the
// empty-input edge this phase's coverage probe surfaced.
func (h *FetchHandlers) List(c *gin.Context) {
	items, err := h.Media.ListAllMedia(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list media"})
		return
	}

	entries := make([]mediaListEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, mediaListEntry{
			ID:          item.ID,
			ObjectKey:   item.ObjectKey,
			ContentType: item.ContentType,
			SizeBytes:   item.SizeBytes,
			OwnerID:     item.OwnerID,
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{"media": entries})
}

// Fetch streams a stored object's bytes back by its object key, with no
// Authorization header required (D-01). The content type returned is the
// one recorded on the media row at upload time, never one guessed at read
// time. A key containing traversal segments, a leading separator, or
// anything that would resolve outside this service's own key prefix is
// rejected with the SAME 404 a genuinely-missing key returns (T-03-28) —
// this endpoint never reveals which case occurred, so it cannot be used as
// an oracle for which keys exist.
func (h *FetchHandlers) Fetch(c *gin.Context) {
	raw := strings.TrimPrefix(c.Param("key"), "/")

	if !isSafeObjectKey(raw, h.KeyPrefix) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	ctx := c.Request.Context()
	item, err := h.Media.GetMediaByKey(ctx, raw)
	if err != nil {
		// Identical 404 whether the key was rejected above or is
		// genuinely unknown to the database — no branch here reveals
		// which case occurred (T-03-28).
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	reader, err := h.Storage.Get(ctx, item.ObjectKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch object"})
		return
	}
	// A close error here is a best-effort cleanup failure on an
	// already-served (or failed-mid-stream) response — nothing left to do
	// with it but note it, never a reason to change the response already
	// sent. The linter (errcheck) requires acknowledging the return value
	// explicitly rather than a bare `defer reader.Close()`.
	defer func() { _ = reader.Close() }()

	c.Header("Content-Type", item.ContentType)
	c.Status(http.StatusOK)
	// Streamed rather than buffered — same reasoning as the upload path,
	// and the same configuration (KeyPrefix) already loaded at startup.
	_, _ = io.Copy(c.Writer, reader)
}

// isSafeObjectKey validates a client-supplied key BEFORE it ever touches
// storage: it must clean to itself (no ".." segments, no redundant
// separators), it must not be absolute, and it must resolve inside
// keyPrefix. Deliberately never joins the raw client segment onto a
// filesystem-style path and re-derives safety from the joined result —
// that join-then-check pattern is exactly what lets a client-controlled
// ".." segment escape the intended prefix. This validates the raw string
// directly instead, and only a validated string is ever handed to
// storage.Get.
func isSafeObjectKey(raw, keyPrefix string) bool {
	if raw == "" {
		return false
	}
	cleaned := path.Clean(raw)
	if cleaned != raw {
		return false
	}
	if strings.HasPrefix(cleaned, "/") {
		return false
	}
	if strings.Contains(cleaned, "..") {
		return false
	}
	return strings.HasPrefix(cleaned, keyPrefix)
}
