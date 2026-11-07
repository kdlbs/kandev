package api

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"net/http"
)

// The instance router's existing authentication/instance fence applies. This
// endpoint returns provider facts, never its canonical environment or secrets.
func (s *Server) handleExecutionUsage(c *gin.Context) {
	if c.FullPath() == "/api/v1/agent/usage/cached" {
		c.JSON(http.StatusOK, s.procMgr.PeekExecutionUsage(c.Request.Context()))
		return
	}
	c.JSON(http.StatusOK, s.procMgr.ReadExecutionUsage(c.Request.Context(), c.Query("model")))
}

func (s *Server) promptWithUsageSelection(ctx context.Context, agentAdapter adapter.AgentAdapter, request PromptRequest) error {
	if err := s.procMgr.ConsumeUsageSelectionModel(); err != nil {
		return err
	}
	return promptOrSteer(ctx, agentAdapter, request)
}
