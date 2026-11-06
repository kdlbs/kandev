package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

// The instance router's existing authentication/instance fence applies. This
// endpoint returns provider facts, never its canonical environment or secrets.
func (s *Server) handleExecutionUsage(c *gin.Context) {
	c.JSON(http.StatusOK, s.procMgr.ReadExecutionUsage(c.Request.Context(), c.Query("model")))
}
