package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"go.uber.org/zap"
)

// SetPollModeRequest is the body for POST /api/v1/workspace/poll-mode.
type SetPollModeRequest struct {
	Mode string `json:"mode"`
}

// handleSetPollMode updates the workspace tracker's poll mode based on the
// gateway's view of UI subscription/focus state for sessions in this workspace.
//
// See plan: focus-gated git polling. The gateway computes a workspace-level
// mode (fast if any session focused, slow if any subscribed, paused otherwise)
// and pushes it here so the tracker can throttle expensive git scans.
func (s *Server) handleSetPollMode(c *gin.Context) {
	var req SetPollModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	mode := process.PollMode(req.Mode)
	if !mode.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode: must be one of fast, slow, paused"})
		return
	}

	s.procMgr.GetWorkspaceTracker().SetPollMode(mode)
	s.logger.Debug("workspace poll mode updated", zap.String("mode", req.Mode))

	c.JSON(http.StatusOK, gin.H{"mode": req.Mode})
}
