package httpmw

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InterimSettingsInterlockHeader carries the replayable, per-boot SPA
// interlock used to reduce accidental settings mutations. It is not an
// authentication credential.
const InterimSettingsInterlockHeader = "X-Kandev-Interim-Settings-Interlock"

// NewInterimSettingsInterlockToken returns a random token for one backend
// boot. Callers must fail closed when token generation fails.
func NewInterimSettingsInterlockToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// InterimSettingsInterlock rejects agent/settings mutations without the
// current per-boot token and rejects Office bearer credentials outright. The
// token is intentionally replayable by a client that can read the SPA boot
// payload, so this is an interim CSRF/accidental-mutation interlock only.
func InterimSettingsInterlock(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(token) == "" || hasBearerAuthorization(c.GetHeader("Authorization")) ||
			!constantTimeEqual(c.GetHeader(InterimSettingsInterlockHeader), token) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "interim settings interlock required"})
			return
		}
		c.Next()
	}
}

func hasBearerAuthorization(value string) bool {
	parts := strings.Fields(value)
	return len(parts) > 0 && strings.EqualFold(parts[0], "Bearer")
}

func constantTimeEqual(value, expected string) bool {
	if value == "" || expected == "" || len(value) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1
}
