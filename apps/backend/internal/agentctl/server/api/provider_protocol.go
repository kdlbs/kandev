package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) providerProtocolGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.procMgr.ProviderMCPProtocolRequired() && providerProtocolRequired(c.Request.URL.Path, c.Request.Method) && c.GetHeader(types.ProviderMCPReplacementHeader) != "1" {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"code": "PROVIDER_MCP_PROTOCOL_REQUIRED", "capability": types.ProviderMCPReplacementCapability})
			return
		}
		c.Next()
	}
}

func providerProtocolRequired(path, method string) bool {
	if path == "/api/v1/agent/stream" {
		return true
	}
	if method != http.MethodPost && method != http.MethodPut {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/agent/submissions") {
		return true
	}
	switch path {
	case "/api/v1/start", "/api/v1/agent/configure", "/api/v1/agent/managed-runtime/cache-repair", "/api/v1/agent/background-work/action", "/api/v1/agent/provider-replacement", "/api/v1/mcp/mode", "/api/v1/mcp/builtin-binding", "/api/v1/mcp/providers":
		return true
	}
	return false
}
