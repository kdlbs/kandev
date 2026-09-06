package backendapp

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
)

const persistenceDiagnosticsPath = "/api/v1/system/diagnostics/persistence"

// requiredPersistenceMiddleware prevents stateful requests from reaching
// handlers while required local persistence is unavailable. Liveness,
// readiness, static assets, and the authenticated diagnostics endpoint remain
// reachable so operators can observe and recover the process.
func requiredPersistenceMiddleware(health *requiredstores.Health) gin.HandlerFunc {
	return func(c *gin.Context) {
		if health == nil || health.Healthy() || persistencePathExcluded(c.Request.URL.Path) {
			c.Next()
			return
		}
		c.Abort()
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":     "required persistence is unavailable",
			"code":      "persistence_unavailable",
			"store_ids": health.UnavailableStoreIDs(),
			"action":    "Check the database connection and the persistence diagnostics, then retry.",
		})
	}
}

func persistencePathExcluded(path string) bool {
	if path == "/health" || path == "/ready" || path == persistenceDiagnosticsPath {
		return true
	}
	return !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/mcp") && path != "/ws"
}
