package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (s *Server) handleBackgroundWorkAction(c *gin.Context) {
	var req streams.BackgroundWorkActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, streams.BackgroundWorkActionResponse{
			Success: false,
			Error:   "invalid request body: " + err.Error(),
		})
		return
	}
	adpt := s.procMgr.GetAdapter()
	if adpt == nil {
		c.JSON(http.StatusOK, streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "agent process not running or adapter not connected",
		})
		return
	}
	bwp, ok := adpt.(adapter.BackgroundWorkProvider)
	if !ok {
		c.JSON(http.StatusOK, streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "background actions unsupported by agent adapter",
		})
		return
	}
	resp, err := bwp.PerformBackgroundWorkAction(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   err.Error(),
		})
		return
	}
	if resp == nil {
		c.JSON(http.StatusOK, streams.BackgroundWorkActionResponse{
			Success: true,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
