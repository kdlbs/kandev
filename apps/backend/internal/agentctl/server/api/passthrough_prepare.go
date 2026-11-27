package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) handlePassthroughPrepare(c *gin.Context) {
	var request types.PassthroughPrepareRequest
	if !bindPassthrough(c, &request) {
		return
	}
	if err := s.procMgr.Passthrough().Prepare(c.Request.Context(), request); err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "prepared"})
}

func (s *Server) handlePassthroughCleanup(c *gin.Context) {
	var request struct {
		Generation uint64 `json:"generation"`
	}
	if !bindPassthrough(c, &request) {
		return
	}
	if err := s.procMgr.Passthrough().CleanupConfig(c.Request.Context(), request.Generation); err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cleaned"})
}
