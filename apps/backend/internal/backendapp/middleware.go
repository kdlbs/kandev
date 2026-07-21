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
				// Disallowed cross-origin browser request. Reject the CORS
				// preflight *and* every state-changing method outright. A
				// browser "simple request" — e.g. a multipart/form-data POST,
				// which is CORS-safelisted and therefore skips preflight —
				// would otherwise still execute the handler's side effect even
				// though the browser can't read the (CORS-blocked) response.
				// That is a drive-by CSRF primitive against mutating endpoints
				// (a malicious page could POST to a loopback-bound backend).
				// A stray safe method (GET/HEAD) is still allowed to fall
				// through without CORS headers, so the browser cannot read the
				// response and no state is changed.
				if !isCORSSafeMethod(c.Request.Method) {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}

				c.Next()
				return
			}

			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isCORSSafeMethod reports whether an HTTP method is a CORS "safe" method that
// cannot mutate server state. Only these may fall through to the handler for a
// disallowed cross-origin browser request; OPTIONS (preflight) and every
// state-changing method (POST/PUT/PATCH/DELETE) are rejected with 403.
func isCORSSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead:
		return true
	default:
		return false
	}
}
