// auth.go — login, logout, and the session gate (D-01, D-06).
//
// Login and logout depend on the database and session store only through
// the small interfaces below (UserFetcher, SessionStore) rather than the
// concrete internal/db and internal/session types, so the handler tests
// stay a fast unit run behind httptest with no external services (the
// integration coverage lives in internal/db and internal/session
// themselves, and Task 3's end-to-end verify proves the real wiring).
package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/AuraBit/athena-app/src/media/internal/db"
)

// UserFetcher is the subset of internal/db's Pool that Login needs.
type UserFetcher interface {
	GetUserByUsername(ctx context.Context, username string) (*db.User, error)
}

// SessionStore is the subset of internal/session's Store that the auth
// handlers and the session gate need.
type SessionStore interface {
	Create(ctx context.Context, userID string) (string, error)
	Lookup(ctx context.Context, token string) (string, error)
	Delete(ctx context.Context, token string) error
}

// AuthHandlers holds the dependencies Login, Logout and RequireSession
// share. Constructed once in cmd/media/main.go from the same startup-built
// db.Pool and session.Store every other handler uses.
type AuthHandlers struct {
	Users    UserFetcher
	Sessions SessionStore
}

// loginRequest is the login endpoint's request body.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// unauthorizedBody is returned byte-identical for both an unknown username
// and a wrong password (T-03-18) — the response never reveals which
// occurred. Declared once so both call sites are guaranteed to match.
var unauthorizedBody = gin.H{"error": "invalid username or password"}

// dummyHash is a valid bcrypt hash of a value that is never a real
// password. When the requested username does not exist, Login still runs a
// bcrypt comparison against this hash before responding, so an unknown
// username and a wrong password take roughly the same amount of time —
// defense-in-depth against a timing side channel, on top of T-03-18's
// identical-response-body mitigation (Rule 2: missing critical security
// functionality for V2 Authentication).
var dummyHash = []byte("$2a$12$jD71l7ATXKhV5U.sthrNP.iV.lNCDe.8zhHEc8baUBmETHL6u88Fu")

// Login validates a username/password against the seeded bcrypt-hashed
// users and, on success, mints a Valkey-backed session token. It NEVER
// logs the submitted password, the stored hash, or the minted token
// (T-03-19) — only structural outcomes (success/failure) would ever be
// logged if request logging is added later.
func (h *AuthHandlers) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	user, err := h.Users.GetUserByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if errors.Is(err, db.ErrNotFound) || user == nil {
		// Run the comparison against the dummy hash anyway (see dummyHash's
		// comment) rather than returning immediately.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		c.JSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}

	token, err := h.Sessions.Create(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// Logout invalidates the caller's session server-side. Idempotent: a
// missing or already-invalid token still returns 204, matching
// session.Store.Delete's own idempotency — logging out twice returns the
// same result rather than erroring.
func (h *AuthHandlers) Logout(c *gin.Context) {
	token := bearerToken(c)
	if token != "" {
		// Best-effort: Delete is idempotent and a delete of an
		// already-gone key is not an error condition worth surfacing to
		// the caller.
		_ = h.Sessions.Delete(c.Request.Context(), token)
	}
	c.Status(http.StatusNoContent)
}

// contextUserIDKey is the gin context key RequireSession sets and any
// downstream handler (e.g. Plan 03-05's upload handler) reads.
const contextUserIDKey = "sessionUserID"

// RequireSession is the session gate (D-01, load-bearing per this plan):
// it rejects a request with 401 when the bearer token is absent,
// malformed, unknown or expired, and otherwise stores the resolved user ID
// in the gin context for downstream handlers. Registered on a route group
// in cmd/media/main.go that Plan 03-05's upload route joins without
// re-wiring.
func (h *AuthHandlers) RequireSession(c *gin.Context) {
	token := bearerToken(c)
	userID, err := h.Sessions.Lookup(c.Request.Context(), token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.Set(contextUserIDKey, userID)
	c.Next()
}

// bearerToken extracts the token from a "Bearer <token>" Authorization
// header. Returns "" for a missing or malformed header — RequireSession
// and Logout both treat that as session.ErrNotFound territory rather than
// a distinct error path.
func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimPrefix(header, prefix)
}
