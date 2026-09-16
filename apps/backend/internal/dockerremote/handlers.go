// Package dockerremote serves the remote Docker executor's connection test.
// The probe itself lives behind the agent-runtime seam; this package only
// speaks HTTP.
package dockerremote

import (
	"net/http"

	"github.com/gin-gonic/gin"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
)

// Handler serves the remote Docker connection test.
type Handler struct {
	prober *agentruntime.RemoteDockerProber
}

// NewHandler builds the handler with the production probe operations.
func NewHandler(log *logger.Logger) *Handler {
	return &Handler{prober: agentruntime.NewRemoteDockerProber(log)}
}

// RegisterRoutes mounts the remote Docker routes.
func RegisterRoutes(router *gin.Engine, log *logger.Logger) {
	h := NewHandler(log)
	router.POST("/api/v1/remote-docker/test", h.httpTest)
}

func (h *Handler) httpTest(c *gin.Context) {
	var req agentruntime.RemoteDockerTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.prober.Run(c.Request.Context(), req))
}
