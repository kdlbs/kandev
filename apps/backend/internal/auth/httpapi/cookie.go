package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// setSessionCookie writes the kandev session cookie: HttpOnly (no JS access),
// SameSite=Lax (sent on same-site navigations and the same-origin WS upgrade,
// blocked on cross-site POSTs), Secure when the request arrived over TLS
// (directly or via a reverse proxy that sets X-Forwarded-Proto).
func setSessionCookie(c *gin.Context, name, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, token, int(ttl.Seconds()), "/", "", requestIsTLS(c), true)
}

// SetSessionCookie writes the kandev session cookie with the same flags as the
// login/setup handlers. Exported for callers outside this package that mint a
// session and must set the cookie themselves — the plugin SSO login bridge
// (internal/backendapp) after an auth-capable plugin validates an external
// identity. Keeping this the single cookie-writing path means TLS/SameSite/
// HttpOnly flags never drift between the local and SSO login flows.
func SetSessionCookie(c *gin.Context, name, token string, ttl time.Duration) {
	setSessionCookie(c, name, token, ttl)
}

func clearSessionCookie(c *gin.Context, name string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", "", requestIsTLS(c), true)
}

func requestIsTLS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
