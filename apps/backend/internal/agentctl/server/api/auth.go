package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Shared auth-rejection messages, reused by bearerTokenAuth, instanceAuth,
// and controlCredentialAuth (credential_rotation.go) so the three auth
// checks -- static token, per-instance dynamic credential, and control-plane
// rotating credential -- report identical wording (goconst: 3+ occurrences).
const (
	errMissingAuthHeader = "missing or invalid Authorization header"
	errInvalidAuthToken  = "invalid auth token"
)

// credentialInvalidatedContextKey is the gin context key instanceAuth uses to
// hand a stream handler the invalidation channel for the exact credential
// generation that authenticated the request. A stream handler must read it
// from here rather than calling credentialSource.Invalidated() itself: that
// would be a second, independent lock acquisition, and a rotation landing
// between the two could authenticate the request against the generation it
// just superseded while handing the handler a channel for the generation
// that replaced it -- a channel that never closes for the rotation that
// actually invalidated the accepted credential (AC-EXECUTORS-CONTROL-
// OWNERSHIP-002.2).
const credentialInvalidatedContextKey = "credentialInvalidated"

// afterInstanceAuthAccepted is invoked, if non-nil, immediately after
// instanceAuth accepts a credentialSource-backed request and captures its
// invalidation channel, before the request is allowed to proceed. It exists
// solely so a test can deterministically land a concurrent rotation in this
// exact window; always nil in production.
var afterInstanceAuthAccepted func()

// bearerTokenAuth returns a gin middleware that validates a Bearer token
// on every request except the exempted paths (e.g., /health).
// If expectedToken is empty, authentication is disabled (no-op middleware).
func bearerTokenAuth(expectedToken string, exemptPaths ...string) gin.HandlerFunc {
	if expectedToken == "" {
		return func(c *gin.Context) { c.Next() }
	}

	exempt := make(map[string]bool, len(exemptPaths))
	for _, p := range exemptPaths {
		exempt[p] = true
	}

	return func(c *gin.Context) {
		if exempt[c.Request.URL.Path] {
			c.Next()
			return
		}

		token := extractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errMissingAuthHeader})
			return
		}

		if !tokenEqual(token, expectedToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errInvalidAuthToken})
			return
		}

		c.Next()
	}
}

// instanceAuth returns a gin middleware validating a Bearer token on every
// request except the exempted paths, exactly like bearerTokenAuth, but reads
// its accepted token dynamically on each request rather than capturing one
// at registration time.
//
// When s.credentialSource is set (via SetCredentialSource), it authenticates
// against the control server's current highest-numbered rotation instead of
// staticToken -- the single-credential model of design 01 "Single driver"
// (AC-EXECUTORS-CONTROL-OWNERSHIP-002.6). s.credentialSource is read fresh
// per request rather than baked into a closure at Use()-time because
// SetCredentialSource is only ever called after NewServer returns, once the
// caller has a control server to wire it to.
//
// When credentialSource is nil, behavior is identical to
// bearerTokenAuth(staticToken, exemptPaths...), including "empty token
// disables auth" -- every existing test constructing a Server directly is
// unaffected.
func (s *Server) instanceAuth(staticToken string, exemptPaths ...string) gin.HandlerFunc {
	exempt := make(map[string]bool, len(exemptPaths))
	for _, p := range exemptPaths {
		exempt[p] = true
	}

	return func(c *gin.Context) {
		if exempt[c.Request.URL.Path] {
			c.Next()
			return
		}

		if s.credentialSource == nil && staticToken == "" {
			c.Next()
			return
		}

		token := extractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errMissingAuthHeader})
			return
		}

		if s.credentialSource != nil {
			accepted, invalidated := s.credentialSource.AcceptsFullWithInvalidation(token)
			if !accepted {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errInvalidAuthToken})
				return
			}
			if afterInstanceAuthAccepted != nil {
				afterInstanceAuthAccepted()
			}
			c.Set(credentialInvalidatedContextKey, invalidated)
			c.Next()
			return
		}

		if !tokenEqual(token, staticToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errInvalidAuthToken})
			return
		}
		c.Next()
	}
}

// credentialInvalidatedFromContext reads the invalidation channel instanceAuth
// captured atomically with its accept check (see credentialInvalidatedContextKey).
// Returns nil when no credentialSource is wired -- the caller's watcher
// goroutine is then correctly skipped, matching legacy no-control-server
// behavior.
func credentialInvalidatedFromContext(c *gin.Context) <-chan struct{} {
	v, ok := c.Get(credentialInvalidatedContextKey)
	if !ok {
		return nil
	}
	ch, _ := v.(<-chan struct{})
	return ch
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	const prefix = "Bearer "
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, prefix) {
		return auth[len(prefix):]
	}
	return ""
}

// tokenEqual compares two tokens in constant time to prevent timing attacks.
func tokenEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
