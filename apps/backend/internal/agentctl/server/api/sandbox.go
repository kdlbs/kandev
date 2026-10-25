package api

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/sandbox"
	"io"
	"net/http"
)

func (s *Server) admitExecutionConfiguration(c *gin.Context, req AgentConfigureRequest) bool {
	if req.LaunchWrapper.IsZero() && req.Sandbox == nil {
		return true
	}
	if !c.GetBool(authenticatedControlContextKey) {
		c.JSON(http.StatusForbidden, AgentConfigureResponse{Error: "authenticated control is required for execution settings"})
		return false
	}
	invalid := !req.AgentArgs.Present || (!req.LaunchWrapper.IsZero() && req.LaunchWrapper.Validate() != nil)
	if req.Sandbox != nil && req.Sandbox.Validate() != nil {
		invalid = true
	}
	if invalid {
		c.JSON(http.StatusBadRequest, AgentConfigureResponse{Error: "invalid execution configuration"})
		return false
	}
	return true
}

func (s *Server) handleSandboxOwner(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "authenticated control is required"})
		return
	}
	owner, err := s.procMgr.SandboxOwner()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sandbox owner is unavailable"})
		return
	}
	c.JSON(http.StatusOK, owner)
}

func (s *Server) handleSetSandboxOwner(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var owner sandbox.ProcessOwner
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&owner) != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if s.procMgr.SetSandboxOwner(&owner) != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleSandboxStartReceipt(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	receipt, err := s.procMgr.SandboxStartReceipt(c.Param("attemptID"))
	if err != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	c.JSON(http.StatusOK, receipt)
}
