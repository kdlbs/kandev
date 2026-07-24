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
