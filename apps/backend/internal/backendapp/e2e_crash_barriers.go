package backendapp

import (
	"errors"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// registerE2ECrashBarrierRoutes mounts the plugin-executor crash barrier
// controls. They exist only under KANDEV_E2E_MOCK=true.
func registerE2ECrashBarrierRoutes(router *gin.Engine) {
	if os.Getenv("KANDEV_E2E_MOCK") != e2eEnabledValue {
		return
	}
	api := router.Group("/api/v1/e2e/crash-barriers")
	api.POST("/:name/arm", func(c *gin.Context) {
		var request struct {
			Mode string `json:"mode"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		switch err := agentruntime.ArmCrashBarrier(c.Param("name"), request.Mode); {
		case err == nil:
			c.Status(http.StatusNoContent)
		case errors.Is(err, models.ErrExecutorConflict):
			c.Status(http.StatusConflict)
		case errors.Is(err, agentruntime.ErrCrashBarrierDisabled):
			c.Status(http.StatusNotFound)
		default:
			c.Status(http.StatusBadRequest)
		}
	})
	api.POST("/:name/release", func(c *gin.Context) {
		agentruntime.ReleaseCrashBarrier(c.Param("name"))
		c.Status(http.StatusNoContent)
	})
}
