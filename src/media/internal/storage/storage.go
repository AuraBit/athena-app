// Package storage wraps the AWS SDK for Go v2's S3 client, configured to
// target LocalStack rather than real AWS (Plan 03-05, D-02, D-07).
//
// The endpoint this client is built from (Config.S3Endpoint) is the
// in-cluster host alias fixed in Plan 03-01 — host.k3d.internal — not the
// loopback address (localhost:4566) that only resolves from the host
// itself. This is exactly why D-02 rejects presigned URLs for this
// service: a URL the service signs against host.k3d.internal carries a
// signature the client's own browser or curl request, made against
// media-dev.athena.net from outside the cluster, cannot satisfy — the
// hostname baked into the signature and the hostname the request actually
// travels through would differ. The production-scale answer (a CDN or the
// client uploading directly to a presigned URL against the real public S3
// hostname) is recorded in README.md as the pattern this estate's local
// topology cannot reproduce; this package instead proxies every byte
// through the service itself.
//
// Every operation streams rather than buffers: Put takes an io.Reader and
// hands it to the SDK's PutObject call without ever materialising the full
// body in this process's memory, and Get returns the SDK's own streaming
// body reader rather than reading it to completion first. Whether the SDK
// itself buffers regardless of how this package streams into it is an
// open, honestly-carried question — see the plan's backstop truth on peak
// memory under a cap-sized upload.
package storage

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/AuraBit/athena-app/src/media/internal/config"
)

// Object is one entry returned by List — just enough to drive the public
// list endpoint without leaking the SDK's own richer type across the
// package boundary.
type Object struct {
	Key          string
	SizeBytes    int64
	LastModified string
}

// Client wraps the S3 client and the bucket/prefix this service's storage
// operations are scoped to. Constructed once at process startup from the
// same startup-loaded Config every other component uses (D-12) — never
// reconstructed or re-pointed at a different endpoint afterward.
type Client struct {
	s3     *s3.Client
	bucket string
	prefix string
}

// New builds a Client targeting LocalStack: path-style addressing (a
// LocalStack requirement — virtual-hosted-style bucket addressing does not
// resolve against a single endpoint the way it does against real AWS's
// per-region DNS), the configured region, and the simulated
// per-environment static credentials (cfg.S3AccessKeyID/S3SecretAccessKey
// — non-secret by construction, D-16).
func New(ctx context.Context, cfg *config.Config) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		o.UsePathStyle = true
	})

	return &Client{
		s3:     s3Client,
		bucket: cfg.S3Bucket,
		prefix: cfg.S3KeyPrefix,
	}, nil
}

// Put streams body into the bucket under key (already assumed to carry
// this Client's prefix — key generation is the caller's responsibility,
// internal/handlers/upload.go, never derived from client input) with the
// given content type. The SDK reads from body as it writes to S3; this
// call never reads body to completion into a local buffer first.
func (c *Client) Put(ctx context.Context, key, contentType string, body io.Reader, sizeBytes int64) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(sizeBytes),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

// Get returns a streaming reader over the object at key. The caller
// (internal/handlers/fetch.go) is responsible for closing the returned
// io.ReadCloser. ErrNotFound is returned when the key does not exist —
// Fetch translates that into the same 404 it returns for a rejected
// (traversal) key, never distinguishing the two to the caller.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	return out.Body, nil
}

// List returns every object under this Client's key prefix, ordered by key
// so two identical calls return the same sequence (the ordering property
// internal/handlers/fetch.go's List handler depends on to be deterministic
// end-to-end alongside its own database ORDER BY). An empty bucket returns
// an empty (never nil) slice and a nil error.
func (c *Client) List(ctx context.Context) ([]Object, error) {
	out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(c.prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list: %w", err)
	}

	objects := make([]Object, 0, len(out.Contents))
	for _, item := range out.Contents {
		objects = append(objects, Object{
			Key:          aws.ToString(item.Key),
			SizeBytes:    aws.ToInt64(item.Size),
			LastModified: item.LastModified.String(),
		})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

// Delete removes the object at key. Used only for cleanup on a failed
// write (internal/handlers/upload.go: if the database insert fails after
// the S3 put succeeded, Delete removes the now-orphaned object rather than
// leaving it behind) — never exposed on any HTTP route.
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}
