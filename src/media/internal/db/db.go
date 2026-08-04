// Package db is the media service's pgx-backed Postgres data layer. It
// exposes exactly the queries this phase needs (D-01's minimal vertical
// slice): fetch a user by username for login, insert a media row, and list
// a user's media rows. Every query is parameterised through pgx — never
// string-concatenated SQL (T-03-21).
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by GetUserByUsername when no row matches. It is a
// distinguishable sentinel — never a generic error — so the login handler
// (internal/handlers/auth.go) can tell "unknown user" apart from "wrong
// password" internally without leaking which case occurred to the caller
// (T-03-18): both must produce an identical 401 response body.
var ErrNotFound = errors.New("db: not found")

// Pool wraps a pgx connection pool constructed from the media service's
// startup-loaded Config (internal/config). It is created once at process
// startup, alongside the config it is built from — never reconstructed or
// re-pointed at a different DSN afterward.
type Pool struct {
	pool *pgxpool.Pool
}

// User is a row from the users table (created by migration 000002, seeded
// by 000003). PasswordHash is always a bcrypt hash — never plaintext or a
// reversibly-encoded value; that invariant is enforced at the migration
// layer, not here.
type User struct {
	ID           string
	Username     string
	PasswordHash string
}

// MediaItem is a row from the media table, including the owner_id column
// migration 000004 added.
type MediaItem struct {
	ID          string
	ObjectKey   string
	ContentType string
	SizeBytes   int64
	OwnerID     string
	CreatedAt   time.Time
}

// NewPool constructs and pings a pgx connection pool against databaseURL.
// Callers are expected to hold this Pool for the lifetime of the process
// (or the lifetime of a single test) and Close it explicitly.
func NewPool(ctx context.Context, databaseURL string) (*Pool, error) {
	pgxCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("db: new pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return &Pool{pool: pool}, nil
}

// Close releases the underlying pgx pool's connections.
func (p *Pool) Close() {
	p.pool.Close()
}

// GetUserByUsername fetches a single user row by its unique username. It
// returns ErrNotFound (never a generic/wrapped "no rows" error) when no
// user matches, and never returns a partially-populated User — either every
// field is populated from a full row scan, or an error (possibly
// ErrNotFound) is returned and the *User is nil.
func (p *Pool) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := p.pool.QueryRow(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: get user by username: %w", err)
	}
	return &u, nil
}

// InsertMedia inserts one media row owned by ownerID and returns the fully
// populated row (including the id and created_at the database assigns).
// Plan 03-05's upload handler is the intended caller.
func (p *Pool) InsertMedia(ctx context.Context, objectKey, contentType string, sizeBytes int64, ownerID string) (*MediaItem, error) {
	var m MediaItem
	err := p.pool.QueryRow(ctx,
		`INSERT INTO media (object_key, content_type, size_bytes, owner_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, object_key, content_type, size_bytes, owner_id, created_at`,
		objectKey, contentType, sizeBytes, ownerID,
	).Scan(&m.ID, &m.ObjectKey, &m.ContentType, &m.SizeBytes, &m.OwnerID, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("db: insert media: %w", err)
	}
	return &m, nil
}

// ListMedia returns ownerID's media rows, newest first — the query
// migration 000004's idx_media_owner_id_created_at index exists to serve.
// An owner with no media rows gets an empty (not nil-error) slice.
func (p *Pool) ListMedia(ctx context.Context, ownerID string) ([]MediaItem, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, object_key, content_type, size_bytes, owner_id, created_at
		 FROM media
		 WHERE owner_id = $1
		 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list media: %w", err)
	}
	defer rows.Close()

	items := make([]MediaItem, 0)
	for rows.Next() {
		var m MediaItem
		if err := rows.Scan(&m.ID, &m.ObjectKey, &m.ContentType, &m.SizeBytes, &m.OwnerID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan media row: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: list media rows: %w", err)
	}
	return items, nil
}
