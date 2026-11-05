package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	profile "github.com/kandev/kandev/internal/mcp/profile"
)

func (s *Server) handleSandboxMCP(c *gin.Context) {
	path, id := sandbox.MCPRootPath, profile.PackageCore
	if name := c.Param("package"); name != "" {
		path, id = sandbox.MCPPackagePathPrefix+name, profile.PackageID(name)
	}
	if !c.GetBool(authenticatedControlContextKey) || c.Request.URL.EscapedPath() != "/api/v1/agent/sandbox"+path || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery ||
		!s.procMgr.AdmitSandboxMCPPath(c.GetHeader("X-Kandev-Sandbox-Attempt"), c.GetHeader("X-Kandev-Sandbox-Generation"), c.GetHeader("X-Kandev-Sandbox-Policy"), path) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	switch c.Request.Method {
	case http.MethodPost, http.MethodGet, http.MethodDelete:
	default:
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	if invalidated := credentialInvalidatedFromContext(c); invalidated != nil {
		go func() {
			select {
			case <-ctx.Done():
			case <-invalidated:
				cancel()
			}
		}()
	}
	request := c.Request.Clone(ctx)
	request.URL.Path = path
	request.URL.RawPath = ""
	request.Header.Del("Authorization")
	request.Header.Del(sandboxmcp.MCPPathHeader)
	s.mcpServer.ServeBuiltinStreamableHTTP(id, c.Writer, request)
}
