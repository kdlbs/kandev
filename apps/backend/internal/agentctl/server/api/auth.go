package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid Authorization header",
			})
			return
		}

		if !tokenEqual(token, expectedToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid auth token",
			})
			return
		}

		c.Next()
	}
}

// extractBearerToken extracts the token from the Authorization header
// or from the "token" query parameter (for WebSocket clients).
func extractBearerToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) {
			return auth[len(prefix):]
		}
	}
	// Fallback for WebSocket clients that cannot set headers
	return r.URL.Query().Get("token")
}

// tokenEqual compares two tokens in constant time to prevent timing attacks.
func tokenEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
