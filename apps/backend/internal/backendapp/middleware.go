package backendapp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/httpmw"
)

// corsMiddleware returns a CORS middleware for HTTP and WebSocket connections.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.Request.Header.Get("Origin"); origin != "" {
			if !httpmw.AllowedOrigin(origin, c.Request.Host) {
				// Disallowed cross-origin browser request: reject every method
				// (including GET/HEAD) with 403. Letting one reach the handler
				// yields nothing legitimate — a response the browser could read
				// cross-origin needs CORS headers we only emit for allowed
				// origins — but it does expose side-effecting endpoints to
				// drive-by CSRF: a browser "simple request" (e.g. a
				// multipart/form-data POST, CORS-safelisted so it skips
				// preflight) or even a plain GET (plugin webhooks are
				// registered for GET as well as POST, see plugins/handlers.go)
				// would still trigger the side effect even though the page
				// can't read the response. Non-browser clients (CLI/curl) and
				// resource loads (<img>/<script>) send no Origin and take the
				// branch below, so this doesn't break them.
				c.AbortWithStatus(http.StatusForbidden)
				return
			}

			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		// httpmw.BootTokenHeader must be allowed so split-origin (Vite dev)
		// plugin mutations — which the SPA now sends with this custom header —
		// survive the browser's CORS preflight instead of being blocked before
		// they reach the guarded routes.
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, "+httpmw.BootTokenHeader+", Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
