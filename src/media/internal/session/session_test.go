package session_test

// session_test.go — written first, watched to fail (RED, via a compile
// failure against a not-yet-existing session package) before session.go
// existed, then implemented (GREEN). Bundled into one atomic commit per
// this module's established tracer-style pattern.
//
// These are integration tests against a live Valkey (short-mode skip, no
// build tag) — the fast unit run stays free of external services.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AuraBit/athena-app/src/media/internal/session"
)

func testValkeyAddr() string {
	if addr := os.Getenv("MEDIA_TEST_VALKEY_ADDR"); addr != "" {
		return addr
	}
	return "localhost:16379"
}

func openTestStore(t *testing.T) *session.Store {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a live Valkey; skipped under -short")
	}
	store, err := session.NewStore(testValkeyAddr(), 30) // 30-second TTL, fast for tests
	if err != nil {
		t.Fatalf("session.NewStore: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestIntegration_CreateThenLookup_ReturnsSameUserID(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const userID = "11111111-1111-1111-1111-111111111111"
	token, err := store.Create(ctx, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("Create returned an empty token")
	}

	got, err := store.Lookup(ctx, token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != userID {
		t.Fatalf("expected userID %q, got %q", userID, got)
	}
}

func TestIntegration_Token_IsOpaqueNotDerivedFromUserID(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const username = "demo.reader"
	token, err := store.Create(ctx, username)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if strings.Contains(token, username) {
		t.Fatalf("token must not contain the user identifier verbatim, got %q", token)
	}
	// Not decodable as structured data (e.g. a JWT's three dot-separated
	// base64url segments, or a JSON object) — a bare opaque random string
	// has neither shape.
	if strings.Count(token, ".") >= 2 {
		t.Fatalf("token looks JWT-shaped (multiple '.' separators), got %q", token)
	}
	var anyJSON any
	if err := json.Unmarshal([]byte(token), &anyJSON); err == nil {
		t.Fatalf("token must not be decodable as JSON, got %q", token)
	}
}

func TestIntegration_Lookup_UnknownTokenReturnsErrNotFound(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	_, err := store.Lookup(ctx, "not-a-real-token")
	if err != session.ErrNotFound {
		t.Fatalf("expected session.ErrNotFound for an unknown token, got %v", err)
	}
}

func TestIntegration_Lookup_EmptyTokenReturnsErrNotFound(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	_, err := store.Lookup(ctx, "")
	if err != session.ErrNotFound {
		t.Fatalf("expected session.ErrNotFound for an empty token, got %v", err)
	}
}

func TestIntegration_Delete_InvalidatesSession(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	token, err := store.Create(ctx, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.Lookup(ctx, token); err != session.ErrNotFound {
		t.Fatalf("expected session.ErrNotFound after Delete, got %v", err)
	}
}

func TestIntegration_Delete_IsIdempotent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	token, err := store.Create(ctx, "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, token); err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	if err := store.Delete(ctx, token); err != nil {
		t.Fatalf("second Delete on an already-deleted token must not error: %v", err)
	}
}

func TestIntegration_SessionKey_CarriesServerSideTTL(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	token, err := store.Create(ctx, "44444444-4444-4444-4444-444444444444")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ttl, err := store.TTL(ctx, token)
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("expected a positive TTL of at most 30s, got %v", ttl)
	}
}
