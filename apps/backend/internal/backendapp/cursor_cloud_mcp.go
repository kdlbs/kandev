package backendapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	managedmcp "github.com/kandev/kandev/internal/mcp/managed"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func registerCursorCloudManagedMCP(p routeParams, scopeResolver managedMCPResolver) {
	transport, err := managedmcp.NewTransport(managedmcp.Config{
		Repository: p.taskRepo,
		Authority:  p.taskRepo,
		Scope:      scopeResolver,
		Enabled:    func() bool { return p.features.CursorCloud },
		HandlerFactory: func(
			_ context.Context,
			binding *models.ManagedAgentBinding,
			toolProfile mcpprofile.Context,
			endpointPath string,
		) (http.Handler, error) {
			backend := mcpserver.NewManagedDispatcherBackendClient(p.gateway.Dispatcher, p.log)
			return mcpserver.NewManagedHTTPHandler(
				backend, binding.SessionID, binding.TaskID, p.log, toolProfile, endpointPath,
			)
		},
	})
	if err != nil {
		p.log.Error("managed Cursor Cloud MCP callback is unavailable", zap.Error(err))
		return
	}
	p.router.Any(managedmcp.ManagedCallbackPath+":grant_id", gin.WrapH(transport))
}

type managedMCPResolver interface {
	ScopeOverridingIdentity(context.Context, string) (context.Context, error)
	ScopePrincipal(context.Context, string, string) (context.Context, error)
}
