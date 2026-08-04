package db_test

// db_test.go — written first, watched to fail (RED, via a compile failure
// against a not-yet-existing db package) before db.go existed, then
// implemented (GREEN). Bundled into one atomic commit per the tracer-style
// execution pattern this module already established in Plan 03-01, Task 2.
//
// TestErrNotFound_IsDistinguishableSentinel runs unconditionally (no live
// database needed) and is covered by `go test ./... -short`. The
// TestIntegration_* tests need a live Postgres reachable at
// MEDIA_TEST_DATABASE_URL (default: the throwaway container this plan's
// README documents) with the full migration chain already applied; they
// short-circuit under `go test -short` via testing.Short(), so the fast
// unit run stays free of external services.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AuraBit/athena-app/src/media/internal/db"
)

func TestErrNotFound_IsDistinguishableSentinel(t *testing.T) {
	if db.ErrNotFound == nil {
		t.Fatal("db.ErrNotFound must not be nil")
	}
	if !strings.Contains(db.ErrNotFound.Error(), "not found") {
		t.Fatalf("db.ErrNotFound should read as a not-found error, got: %v", db.ErrNotFound)
	}
}

func testDatabaseURL() string {
	if url := os.Getenv("MEDIA_TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://media:media@localhost:15432/media?sslmode=disable"
}

func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a live Postgres; skipped under -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.NewPool(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("db.NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestIntegration_GetUserByUsername_SeededUserFound(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()

	user, err := pool.GetUserByUsername(ctx, "demo.reader")
	if err != nil {
		t.Fatalf("GetUserByUsername(demo.reader): %v", err)
	}
	if user.Username != "demo.reader" {
		t.Fatalf("expected username demo.reader, got %q", user.Username)
	}
	if user.ID == "" {
		t.Fatal("expected a non-empty user ID")
	}
	if !strings.HasPrefix(user.PasswordHash, "$2a$") && !strings.HasPrefix(user.PasswordHash, "$2b$") {
		t.Fatalf("expected a bcrypt hash (no plaintext password column exists), got %q", user.PasswordHash)
	}
}

func TestIntegration_GetUserByUsername_UnknownUserReturnsErrNotFound(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()

	_, err := pool.GetUserByUsername(ctx, "does-not-exist-"+uniqueSuffix())
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected db.ErrNotFound for an unknown user, got %v", err)
	}
}

func TestIntegration_InsertAndListMedia(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()

	owner, err := pool.GetUserByUsername(ctx, "demo.curator")
	if err != nil {
		t.Fatalf("GetUserByUsername(demo.curator): %v", err)
	}

	objectKey := "test-object-" + uniqueSuffix()
	inserted, err := pool.InsertMedia(ctx, objectKey, "image/png", 1024, owner.ID)
	if err != nil {
		t.Fatalf("InsertMedia: %v", err)
	}
	if inserted.ObjectKey != objectKey {
		t.Fatalf("expected object key %q, got %q", objectKey, inserted.ObjectKey)
	}
	if inserted.OwnerID != owner.ID {
		t.Fatalf("expected owner ID %q, got %q", owner.ID, inserted.OwnerID)
	}

	items, err := pool.ListMedia(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ObjectKey == objectKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected inserted media %q to appear in ListMedia(%q) results", objectKey, owner.ID)
	}
}

func uniqueSuffix() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}
