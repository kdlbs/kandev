package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleSandboxMCP(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) || c.Request.URL.RawQuery != "" || !s.procMgr.AdmitSandboxMCP(c.GetHeader("X-Kandev-Sandbox-Attempt"), c.GetHeader("X-Kandev-Sandbox-Generation"), c.GetHeader("X-Kandev-Sandbox-Policy")) {
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
	request.URL.Path = "/mcp"
	request.URL.RawPath = ""
	request.Header.Del("Authorization")
	s.mcpServer.ServeStreamableHTTP(c.Writer, request)
}
