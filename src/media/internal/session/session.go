// Package session is the media service's Valkey-backed bearer-token session
// store (D-06). A session's transport is an opaque bearer token in the
// Authorization header; its state lives entirely server-side in Valkey with
// a server-side TTL — never a client-trusted expiry, never a JWT or any
// other self-describing/decodable structure. The cookie alternative is
// deliberately not implemented here; it belongs in the study note as the
// browser-app alternative, per this plan's own action text.
//
// Uses the Valkey project's own Go client (valkey-io/valkey-go), locked
// over redis/go-redis/v9 for thematic consistency with D-18's "why Valkey,
// not Redis" narrative (RESEARCH.md Open Question 2) — both are
// wire-compatible and would work identically at this scope.
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// ErrNotFound is returned by Lookup for an absent, malformed, unknown or
// expired token. Deliberately a single sentinel for all four cases — the
// session gate (internal/handlers) must reject all of them identically
// with 401, and telling them apart would only help an attacker enumerate
// which failure mode they hit.
var ErrNotFound = errors.New("session: not found or expired")

// tokenBytes is the amount of secure randomness (256 bits) behind each
// minted token, read from crypto/rand — never derived from a username or
// password, so the token cannot be predicted or forged from user data.
const tokenBytes = 32

// Store wraps a Valkey client and the session TTL (from Config, read once
// at startup — see internal/config). It is the single implementation the
// login/logout handlers and the session-gate middleware share.
type Store struct {
	client valkey.Client
	ttl    time.Duration
}

// NewStore connects to the Valkey instance at addr. Callers are expected to
// hold this Store for the process lifetime and Close it explicitly.
func NewStore(addr string, ttlSeconds int) (*Store, error) {
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		return nil, fmt.Errorf("session: new valkey client: %w", err)
	}
	return &Store{client: client, ttl: time.Duration(ttlSeconds) * time.Second}, nil
}

// Close releases the underlying Valkey client's connections.
func (s *Store) Close() {
	s.client.Close()
}

// Create mints a new opaque bearer token for userID and stores it in Valkey
// with a server-side TTL set at creation (Valkey's own EX expiry — never a
// client-supplied or client-checked value). Returns the token to hand back
// to the caller (the login handler).
func (s *Store) Create(ctx context.Context, userID string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", fmt.Errorf("session: generate token: %w", err)
	}

	cmd := s.client.B().Set().Key(sessionKey(token)).Value(userID).Ex(s.ttl).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return "", fmt.Errorf("session: create: %w", err)
	}

	return token, nil
}

// Lookup resolves a bearer token to the user ID it was minted for. Returns
// ErrNotFound uniformly for an empty, unknown, or expired token — Valkey's
// own EX expiry means an expired key simply no longer exists by the time
// this GET runs, so "unknown" and "expired" are naturally the same case
// here, never distinguished to the caller.
func (s *Store) Lookup(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrNotFound
	}

	cmd := s.client.B().Get().Key(sessionKey(token)).Build()
	userID, err := s.client.Do(ctx, cmd).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("session: lookup: %w", err)
	}

	return userID, nil
}

// Delete invalidates a session server-side by removing its Valkey key.
// Idempotent: deleting an already-deleted or never-existing token succeeds
// silently, so logging out twice returns the same result rather than
// erroring.
func (s *Store) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	cmd := s.client.B().Del().Key(sessionKey(token)).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("session: delete: %w", err)
	}

	return nil
}

// TTL returns the remaining server-side time-to-live on a token's Valkey
// key. Used only by the integration test to assert the TTL is genuinely
// enforced server-side (Valkey's own EX expiry), not just client-trusted.
func (s *Store) TTL(ctx context.Context, token string) (time.Duration, error) {
	cmd := s.client.B().Ttl().Key(sessionKey(token)).Build()
	seconds, err := s.client.Do(ctx, cmd).ToInt64()
	if err != nil {
		return 0, fmt.Errorf("session: ttl: %w", err)
	}
	return time.Duration(seconds) * time.Second, nil
}

func sessionKey(token string) string {
	return "session:" + token
}

// newToken generates a cryptographically random, URL-safe opaque token.
// Never a hash of user-supplied data, never a JWT or other structured/
// decodable format — a test in session_test.go asserts both properties.
func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
