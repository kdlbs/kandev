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

		accept := func(token string) bool { return tokenEqual(token, staticToken) }
		if s.credentialSource != nil {
			accept = s.credentialSource.AcceptsFull
		} else if staticToken == "" {
			c.Next()
			return
		}

		token := extractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errMissingAuthHeader})
			return
		}
		if !accept(token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errInvalidAuthToken})
			return
		}
		c.Next()
	}
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
