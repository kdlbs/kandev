package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (s *Server) currentBuiltinContext() (p.Context, error) {
	if scope, scoped := s.procMgr.CurrentProviderMCPScope(); scoped {
		return s.mcpServer.BuiltinGenerationRuntimeProfile(scope.BindingID)
	}
	return s.mcpServer.Profile(), nil
}

func (s *Server) handleGetBuiltinContext(c *gin.Context) {
	profile, err := s.currentBuiltinContext()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "bound built-in MCP context is unavailable"})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (s *Server) setBuiltinContextMode(mode string) error {
	if scope, scoped := s.procMgr.CurrentProviderMCPScope(); scoped {
		if err := s.mcpServer.SetBuiltinGenerationMode(scope.BindingID, mode); err != nil {
			return err
		}
	} else {
		s.mcpServer.SetMode(mode)
	}
	profile, err := s.currentBuiltinContext()
	if err != nil {
		return err
	}
	return s.procMgr.RefreshBuiltinRuntimeContext(profile)
}

func (s *Server) setBuiltinContextProviders(providers []string) error {
	if scope, scoped := s.procMgr.CurrentProviderMCPScope(); scoped {
		if err := s.mcpServer.SetBuiltinGenerationProviders(scope.BindingID, providers); err != nil {
			return err
		}
	} else {
		s.mcpServer.SetProviders(providers)
	}
	profile, err := s.currentBuiltinContext()
	if err != nil {
		return err
	}
	return s.procMgr.RefreshBuiltinRuntimeContext(profile)
}
