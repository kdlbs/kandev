package backendapp

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
)

// registerE2EStartupPageFixtureRoute exposes writeStartupPage, the same
// renderer newBootstrapHandler uses while the real router is still coming
// up, as a fixture endpoint. The bootstrap window closes as soon as this
// process finishes starting, so a browser test has no other way to reach
// that markup and embedded poll script against a caller-supplied snapshot.
// Gated the same way as registerE2EResetRoutes.
func registerE2EStartupPageFixtureRoute(router *gin.Engine, log *logger.Logger) {
	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode != "true" && mockMode != "only" {
		return
	}
	router.POST("/api/v1/e2e/startup-page-fixture", func(c *gin.Context) {
		var snap startup.Snapshot
		if err := c.ShouldBindJSON(&snap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid startup snapshot"})
			return
		}
		writeStartupPage(c.Writer, c.Request, snap)
	})
	log.Info("registered E2E startup page fixture route (test-only)")
}
