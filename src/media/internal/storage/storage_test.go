package storage_test

// storage_test.go — TestErrNotFound_IsDistinguishableSentinel runs
// unconditionally (no external services) and is covered by
// `go test ./... -short`. TestIntegration_* needs a live LocalStack
// reachable at MEDIA_TEST_S3_ENDPOINT with MEDIA_TEST_S3_BUCKET already
// existing (the real dev bucket Terraform's data-storage stack creates,
// by default); it short-circuits under -short via testing.Short(), the
// same pattern internal/db/db_test.go and internal/session/session_test.go
// already established.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AuraBit/athena-app/src/media/internal/config"
	"github.com/AuraBit/athena-app/src/media/internal/storage"
)

func TestErrNotFound_IsDistinguishableSentinel(t *testing.T) {
	if storage.ErrNotFound == nil {
		t.Fatal("storage.ErrNotFound must not be nil")
	}
	if !strings.Contains(storage.ErrNotFound.Error(), "not found") {
		t.Fatalf("storage.ErrNotFound should read as a not-found error, got: %v", storage.ErrNotFound)
	}
}

func testConfig() *config.Config {
	endpoint := os.Getenv("MEDIA_TEST_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	bucket := os.Getenv("MEDIA_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "athena-media-dev"
	}
	accessKey := os.Getenv("MEDIA_TEST_S3_ACCESS_KEY_ID")
	if accessKey == "" {
		accessKey = "111111111111"
	}
	secretKey := os.Getenv("MEDIA_TEST_S3_SECRET_ACCESS_KEY")
	if secretKey == "" {
		secretKey = "111111111111"
	}
	return &config.Config{
		S3Bucket:          bucket,
		S3Endpoint:        endpoint,
		S3Region:          "us-east-1",
		S3KeyPrefix:       "test-storage/",
		S3AccessKeyID:     accessKey,
		S3SecretAccessKey: secretKey,
	}
}

func openTestClient(t *testing.T) *storage.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a live LocalStack; skipped under -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := storage.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return client
}

func uniqueKey() string {
	return "test-storage/roundtrip-" + time.Now().UTC().Format("20060102150405.000000000")
}

func TestIntegration_PutGet_ByteIdenticalRoundTrip(t *testing.T) {
	client := openTestClient(t)
	ctx := context.Background()

	key := uniqueKey()
	content := []byte("athena media storage integration round trip")

	if err := client.Put(ctx, key, "text/plain", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = client.Delete(context.Background(), key) })

	reader, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = reader.Close() }()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading object body: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("round-tripped bytes differ: put %q, got %q", content, got)
	}
}

func TestIntegration_Get_UnknownKeyReturnsErrNotFound(t *testing.T) {
	client := openTestClient(t)
	ctx := context.Background()

	_, err := client.Get(ctx, "test-storage/does-not-exist-"+time.Now().UTC().Format("20060102150405.000000000"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected storage.ErrNotFound for an unknown key, got %v", err)
	}
}

func TestIntegration_Delete_RemovesObject(t *testing.T) {
	client := openTestClient(t)
	ctx := context.Background()

	key := uniqueKey()
	content := []byte("to be deleted")
	if err := client.Put(ctx, key, "text/plain", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := client.Get(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected storage.ErrNotFound after Delete, got %v", err)
	}
}

func TestIntegration_List_ReturnsPutObjects(t *testing.T) {
	client := openTestClient(t)
	ctx := context.Background()

	key := uniqueKey()
	content := []byte("listed")
	if err := client.Put(ctx, key, "text/plain", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = client.Delete(context.Background(), key) })

	objects, err := client.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, obj := range objects {
		if obj.Key == key {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %q to appear in List() results", key)
	}
}
