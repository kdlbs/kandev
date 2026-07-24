package httpmw

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BootTokenHeader is the request header the SPA sends to prove it read the
// same-origin boot payload. A cross-origin page cannot read another origin's
// boot payload, and a CORS "simple" request cannot set a custom header without
// triggering a preflight the CORS layer then blocks — so requiring this header
// on state-changing routes defeats both the CSRF drive-by and the
// unauthenticated LAN caller (who never sees the token).
const BootTokenHeader = "X-Kandev-Boot-Token" //nolint:gosec // header name, not a credential

// NewBootToken mints a fresh, unguessable per-boot token (256 bits of
// crypto/rand, hex-encoded). Generated once per backend process and embedded
// in the SPA boot payload; RequireBootToken validates inbound requests against
// it.
func NewBootToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate boot token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// RequireBootToken returns middleware that rejects any request whose
// BootTokenHeader does not match token (constant-time compare). It is
// deliberately fail-closed: an empty token means the process was misconfigured
// (token generation should never fail), so every request is refused rather
// than silently left open. Apply only to state-changing operator routes — not
// to browser-native asset loads (dynamic import()/stylesheet fetches cannot
// set custom headers) or to endpoints intended for external unauthenticated
// callers (e.g. inbound webhook relays).
func RequireBootToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "server missing operator token",
			})
			return
		}
		// No early-out on an empty/absent header: ConstantTimeCompare already
		// returns 0 for a length mismatch, so folding both cases into one
		// branch avoids an application-level timing distinguisher between
		// "header absent" and "header present but wrong".
		got := c.GetHeader(BootTokenHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "missing or invalid operator token",
			})
			return
		}
		c.Next()
	}
}
