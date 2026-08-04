package handlers_test

// auth_test.go — written first, watched to fail (RED) before auth.go
// existed, then implemented (GREEN). Runs entirely against fakes behind
// the UserFetcher/SessionStore interfaces (httptest, no external
// services) — the fast unit run covers the 401 paths, the
// identical-failure-body property, the token-opacity property, and the
// logout-invalidation property described in this task's <behavior> block.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/AuraBit/athena-app/src/media/internal/db"
	"github.com/AuraBit/athena-app/src/media/internal/handlers"
	"github.com/AuraBit/athena-app/src/media/internal/session"
)

// fakeUsers is a minimal in-memory handlers.UserFetcher.
type fakeUsers struct {
	byUsername map[string]*db.User
}

func (f *fakeUsers) GetUserByUsername(_ context.Context, username string) (*db.User, error) {
	u, ok := f.byUsername[username]
	if !ok {
		return nil, db.ErrNotFound
	}
	return u, nil
}

// fakeSessions is a minimal in-memory handlers.SessionStore that mirrors
// internal/session.Store's contract (opaque tokens, ErrNotFound for an
// absent/unknown/deleted token, idempotent Delete) without needing a live
// Valkey.
type fakeSessions struct {
	mu       sync.Mutex
	sessions map[string]string // token -> userID
	nextID   int
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{sessions: make(map[string]string)}
}

func (f *fakeSessions) Create(_ context.Context, userID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	// Deliberately NOT derived from userID in any recoverable way beyond
	// this fake's own bookkeeping map — a real token (internal/session)
	// is crypto/rand-generated; this fake only needs distinct, unguessable
	// -enough-for-a-test values.
	token := "opaque-test-token-" + strconv.Itoa(f.nextID)
	f.sessions[token] = userID
	return token, nil
}

func (f *fakeSessions) Lookup(_ context.Context, token string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	userID, ok := f.sessions[token]
	if !ok {
		return "", session.ErrNotFound
	}
	return userID, nil
}

func (f *fakeSessions) Delete(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, token)
	return nil
}

const testUserID = "test-user-id-0001"
const testUsername = "test.user"
const testPassword = "correct-horse-battery-staple"

func newTestAuthRouter(t *testing.T) (*gin.Engine, *fakeSessions) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// bcrypt.MinCost keeps the unit test fast; production hashes (the
	// seeded demo users) use cost 12 per the migration's own comment —
	// the cost factor is orthogonal to the handler logic under test here.
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}

	users := &fakeUsers{byUsername: map[string]*db.User{
		testUsername: {ID: testUserID, Username: testUsername, PasswordHash: string(hash)},
	}}
	sessions := newFakeSessions()
	h := &handlers.AuthHandlers{Users: users, Sessions: sessions}

	r := gin.New()
	r.POST("/login", h.Login)
	r.POST("/logout", h.Logout)
	gated := r.Group("/gated", h.RequireSession)
	gated.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/public/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	return r, sessions
}

func doLogin(t *testing.T, router *gin.Engine, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestLogin_CorrectPassword_Returns200WithOpaqueToken(t *testing.T) {
	router, _ := newTestAuthRouter(t)
	rec := doLogin(t, router, testUsername, testPassword)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("expected a non-empty token in the response, got: %v", body)
	}
	if strings.Contains(token, testUsername) {
		t.Fatalf("token must not contain the username, got %q", token)
	}
	var anyJSON any
	if err := json.Unmarshal([]byte(token), &anyJSON); err == nil {
		t.Fatalf("token must not itself be decodable as structured JSON data, got %q", token)
	}
}

func TestLogin_WrongPasswordAndUnknownUser_ReturnIdentical401Bodies(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	wrongPassword := doLogin(t, router, testUsername, "not-the-right-password")
	unknownUser := doLogin(t, router, "does-not-exist", "irrelevant-password")

	if wrongPassword.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password login: expected 401, got %d", wrongPassword.Code)
	}
	if unknownUser.Code != http.StatusUnauthorized {
		t.Fatalf("unknown-user login: expected 401, got %d", unknownUser.Code)
	}
	if wrongPassword.Body.String() != unknownUser.Body.String() {
		t.Fatalf("expected byte-identical 401 bodies, got %q vs %q",
			wrongPassword.Body.String(), unknownUser.Body.String())
	}
}

func TestLogout_InvalidatesSession_SubsequentGatedRequestRejected(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	loginRec := doLogin(t, router, testUsername, testPassword)
	var loginBody map[string]string
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("json.Unmarshal login response: %v", err)
	}
	token := loginBody["token"]

	// Gated route succeeds before logout.
	gatedReq := httptest.NewRequest(http.MethodGet, "/gated/ping", nil)
	gatedReq.Header.Set("Authorization", "Bearer "+token)
	gatedRec := httptest.NewRecorder()
	router.ServeHTTP(gatedRec, gatedReq)
	if gatedRec.Code != http.StatusOK {
		t.Fatalf("expected the gated route to succeed with a fresh token, got %d", gatedRec.Code)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+token)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from logout, got %d", logoutRec.Code)
	}

	// Same token, presented again, is now rejected.
	afterReq := httptest.NewRequest(http.MethodGet, "/gated/ping", nil)
	afterReq.Header.Set("Authorization", "Bearer "+token)
	afterRec := httptest.NewRecorder()
	router.ServeHTTP(afterRec, afterReq)
	if afterRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", afterRec.Code)
	}
}

func TestLogout_IsIdempotent(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	loginRec := doLogin(t, router, testUsername, testPassword)
	var loginBody map[string]string
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginBody)
	token := loginBody["token"]

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("logout attempt %d: expected 204, got %d", i+1, rec.Code)
		}
	}
}

func TestSessionGate_RejectsAbsentMalformedUnknownToken(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	cases := map[string]string{
		"no header":         "",
		"malformed (no Bearer prefix)": "not-bearer-scheme abc123",
		"unknown token":      "Bearer this-token-was-never-issued",
	}

	for name, authHeader := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/gated/ping", nil)
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s: expected 401, got %d", name, rec.Code)
			}
		})
	}
}

func TestSessionGate_ValidTokenPasses(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	loginRec := doLogin(t, router, testUsername, testPassword)
	var loginBody map[string]string
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginBody)
	token := loginBody["token"]

	req := httptest.NewRequest(http.MethodGet, "/gated/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid token, got %d", rec.Code)
	}
}

func TestPublicEndpoint_ReachableWithoutToken(t *testing.T) {
	router, _ := newTestAuthRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/public/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected the public route to be reachable without a token, got %d", rec.Code)
	}
}

func TestSessionGate_ExpiredToken_IsRejectedLikeUnknown(t *testing.T) {
	// internal/session's real Store enforces expiry via Valkey's own EX
	// TTL, proven live in session_test.go's
	// TestIntegration_SessionKey_CarriesServerSideTTL. This handler-level
	// test proves the gate's OWN behavior is correct for that case using
	// the fake: an expired token is indistinguishable from an unknown one
	// once it's gone from the store, and RequireSession must reject both
	// identically with 401.
	router, sessions := newTestAuthRouter(t)

	loginRec := doLogin(t, router, testUsername, testPassword)
	var loginBody map[string]string
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginBody)
	token := loginBody["token"]

	// Simulate expiry by removing the session the same way Valkey's TTL
	// eviction would.
	sessions.mu.Lock()
	delete(sessions.sessions, token)
	sessions.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/gated/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an expired token, got %d", rec.Code)
	}
}
