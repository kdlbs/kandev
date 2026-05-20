package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InstanceIDHeader is the HTTP header carrying the expected instance
// identifier on requests issued by the kandev lifecycle client. The
// instance-server middleware validates it against its own config so a
// stale client request (e.g. one issued by a goroutine that survived its
// owning execution's deletion) targeting a recycled port is rejected
// before it can configure / start the wrong agent.
const InstanceIDHeader = "X-Instance-ID"

// instanceIDGuard returns a gin middleware that 404s any request whose
// X-Instance-ID header is set but doesn't match expectedID. Missing
// header is allowed for backward compatibility (the agent subprocess,
// raw curl probes, and tests do not send it).
//
// When expectedID is empty the middleware is a no-op so legacy startup
// paths that don't set InstanceConfig.InstanceID keep working.
func instanceIDGuard(expectedID string) gin.HandlerFunc {
	if expectedID == "" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.Request.Header.Get(InstanceIDHeader))
		if got != "" && got != expectedID {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error":           "instance no longer exists at this port",
				"expected_id":     expectedID,
				"got_id":          got,
				"recycled_port":   true,
				"port_reuse_hint": "the previous instance was deleted and this port was reallocated to a different instance",
			})
			return
		}
		c.Next()
	}
}
