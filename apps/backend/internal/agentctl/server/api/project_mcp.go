package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) registerProjectMCPRoutes(api *gin.RouterGroup) {
	api.GET("/mcp/project", s.handleProjectMCPTarget)
	api.POST("/mcp/project/prepare", s.handleProjectMCPPrepare)
	api.POST("/mcp/project/cleanup", s.handleProjectMCPCleanup)
}

func (s *Server) projectMCPFiles() (*process.ProjectMCPFilesOwner, bool) {
	owner := s.procMgr.ProjectMCPFiles()
	if owner == nil {
		return nil, false
	}
	return owner, true
}

func requireProjectMCPControl(c *gin.Context) bool {
	if c.GetBool(authenticatedControlContextKey) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{errKey: "authenticated control is required for project MCP files"})
	return false
}

func (s *Server) handleProjectMCPTarget(c *gin.Context) {
	if !requireProjectMCPControl(c) {
		return
	}
	owner, ok := s.projectMCPFiles()
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "project MCP file owner is unavailable"})
		return
	}
	c.JSON(http.StatusOK, owner.Target())
}

func (s *Server) handleProjectMCPPrepare(c *gin.Context) {
	if !requireProjectMCPControl(c) {
		return
	}
	owner, ok := s.projectMCPFiles()
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "project MCP file owner is unavailable"})
		return
	}
	var request types.ProjectMCPPrepareRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid project MCP preparation request"})
		return
	}
	response, err := owner.Prepare(c.Request.Context(), request)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, process.ErrProjectMCPInvalid):
			status = http.StatusBadRequest
		case errors.Is(err, process.ErrProjectMCPConflict):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleProjectMCPCleanup(c *gin.Context) {
	if !requireProjectMCPControl(c) {
		return
	}
	owner, ok := s.projectMCPFiles()
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "project MCP file owner is unavailable"})
		return
	}
	if err := owner.Cleanup(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cleaned"})
}
