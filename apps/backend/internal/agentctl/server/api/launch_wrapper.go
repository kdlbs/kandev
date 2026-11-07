package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func (s *Server) handleLaunchWrapper(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) {
		c.JSON(http.StatusForbidden, gin.H{errKey: "authenticated control is required for launch wrappers"})
		return
	}
	spec, err := s.procMgr.LaunchWrapperSnapshot()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{errKey: "launch wrapper recovery state is incomplete"})
		return
	}
	c.JSON(http.StatusOK, struct {
		Version int                `json:"version"`
		Wrapper launchwrapper.Spec `json:"launch_wrapper,omitzero"`
	}{Version: 1, Wrapper: spec})
}
