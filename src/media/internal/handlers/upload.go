// upload.go — the session-gated upload endpoint (D-01, D-02, D-11).
//
// D-02 fixes the shape: this service proxies the upload rather than
// handing the client a presigned URL. The hostname LocalStack signs
// against (host.k3d.internal, the in-cluster alias fixed in Plan 03-01)
// differs from the hostname a client actually reaches the service through
// (media-<env>.athena.net, via the Envoy Gateway) — a presigned URL minted
// by the service would carry a signature the client's own request cannot
// satisfy. The presigned pattern is the production-scale answer (a real
// public S3/CloudFront hostname the client and the signer agree on) and is
// recorded in README.md as the deliberate scope boundary this local
// topology cannot reproduce, not an oversight.
package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AuraBit/athena-app/src/media/internal/db"
)

// MediaStorer is the subset of internal/storage's Client that Upload and
// Fetch/List (fetch.go) need.
type MediaStorer interface {
	Put(ctx context.Context, key, contentType string, body io.Reader, sizeBytes int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// MediaInserter is the subset of internal/db's Pool that Upload needs.
type MediaInserter interface {
	InsertMedia(ctx context.Context, objectKey, contentType string, sizeBytes int64, ownerID string) (*db.MediaItem, error)
}

// allowedContentTypes is the magic-byte allowlist (D-11): JPEG and PNG
// only, decided from http.DetectContentType's own MIME strings — never
// from the request's Content-Type header, which this handler never reads
// (T-03-24). A payload whose bytes are not one of these two is rejected
// even when the client's header claims otherwise; a genuine image
// mislabelled by the client is accepted for the identical reason.
var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
}

// UploadHandlers holds the dependencies Upload needs. Constructed once in
// cmd/media/main.go from the same startup-built storage.Client and db.Pool
// every other handler uses.
type UploadHandlers struct {
	Storage            MediaStorer
	Media              MediaInserter
	KeyPrefix          string
	MaxUploadSizeBytes int64
}

// sniffLen is the number of leading bytes http.DetectContentType reads to
// determine a MIME type from real file signatures (the stdlib documents
// 512 bytes as sufficient for every signature it recognises).
const sniffLen = 512

// Upload is registered on the session-gated route group in
// cmd/media/main.go — auth.go's RequireSession runs first, so the gate is
// enforced (and a request rejected) before this handler, and therefore
// before any body byte, is ever reached (T-03-25). The order inside this
// handler is itself the security property (D-11, T-03-24, T-03-26,
// T-03-27):
//
//  1. Bound the body with a limiting reader — an oversized payload is cut
//     off rather than read in full.
//  2. Sniff the leading bytes and reject anything that is not a real JPEG
//     or PNG.
//  3. Stream the validated body into the bucket under a server-generated
//     key — never one derived from client input.
//  4. Insert the media row. If this fails after the object was already
//     written, delete the object: a failed upload must never leave an
//     orphaned S3 object. This ordering (put, then insert) is deliberate;
//     the opposite (insert, then put) would instead risk an orphaned
//     database row on a failed put, which this estate would rather avoid
//     by choosing put-then-insert with a cleanup-on-failure step.
func (h *UploadHandlers) Upload(c *gin.Context) {
	userIDRaw, exists := c.Get(contextUserIDKey)
	ownerID, ok := userIDRaw.(string)
	if !exists || !ok || ownerID == "" {
		// Defensive only: RequireSession already rejects any request that
		// reaches here without a resolved user ID. This path exists so a
		// future refactor that accidentally reorders the middleware chain
		// fails loudly (401) rather than inserting a row with no owner.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Step 1: bound the body. MaxBytesReader makes any Read past the cap
	// return a *http.MaxBytesError rather than letting the handler buffer
	// an unbounded payload — the cap is enforced by the reader itself, not
	// by reading everything and checking a length afterward.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.MaxUploadSizeBytes)

	// Step 2: sniff the leading bytes. io.ReadFull short-reads (rather
	// than erroring) when the body is smaller than sniffLen — a genuinely
	// tiny payload is still sniffable from whatever bytes it has, and
	// http.DetectContentType tolerates a short slice.
	sniff := make([]byte, sniffLen)
	n, err := io.ReadFull(c.Request.Body, sniff)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		if isMaxBytesError(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload exceeds the maximum allowed size"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	sniff = sniff[:n]

	contentType := http.DetectContentType(sniff)
	if !allowedContentTypes[contentType] {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "only JPEG and PNG images are accepted"})
		return
	}

	// Step 3: reassemble the full body (the sniffed prefix plus whatever
	// remains of the capped reader) and stream it into the bucket under a
	// server-generated key. The size cap bounds this handler's own memory
	// use to at most MaxUploadSizeBytes — a small number of megabytes for
	// this demo service — rather than true zero-copy streaming with an
	// unknown Content-Length. Whether the AWS SDK itself buffers
	// regardless, given a non-seekable body, is the open backstop question
	// this plan carries rather than claims solved.
	full := io.MultiReader(bytes.NewReader(sniff), c.Request.Body)
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, full); err != nil {
		if isMaxBytesError(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload exceeds the maximum allowed size"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	key := h.KeyPrefix + generateObjectKey()

	ctx := c.Request.Context()
	if err := h.Storage.Put(ctx, key, contentType, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store upload"})
		return
	}

	// Step 4: insert the media row. On failure, delete the now-orphaned
	// object before returning the error (see the func comment above).
	item, err := h.Media.InsertMedia(ctx, key, contentType, int64(buf.Len()), ownerID)
	if err != nil {
		_ = h.Storage.Delete(ctx, key)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record upload"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":           item.ID,
		"object_key":   item.ObjectKey,
		"content_type": item.ContentType,
		"size_bytes":   item.SizeBytes,
	})
}

// isMaxBytesError reports whether err originates from http.MaxBytesReader
// exceeding its configured limit.
func isMaxBytesError(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// generateObjectKey mints a server-side object key suffix: 16 bytes of
// crypto/rand entropy, hex-encoded. Never derived from the client-supplied
// filename or any other client input (T-03-26) — two uploads sharing the
// same filename always produce two distinct keys. crypto/rand.Read failing
// indicates no secure entropy source is available on this host, which is
// effectively unrecoverable; this panics rather than silently falling back
// to a predictable key, which would violate the server-generated-key
// guarantee this function exists for.
func generateObjectKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("handlers: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
