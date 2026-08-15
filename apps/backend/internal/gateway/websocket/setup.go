package websocket

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Gateway represents the unified WebSocket gateway
type Gateway struct {
	Hub                *Hub
	Dispatcher         *ws.Dispatcher
	Handler            *Handler
	TerminalHandler    *TerminalHandler
	LSPHandler         *LSPHandler
	VscodeProxyHandler *VscodeProxyHandler
	PortProxyHandler   *PortProxyHandler
	TunnelManager      *TunnelManager
	authPolicy         AuthPolicy
	logger             *logger.Logger
}

// NewGateway creates a new WebSocket gateway with all components initialized
func NewGateway(log *logger.Logger) *Gateway {
	dispatcher := ws.NewDispatcher()
	hub := NewHub(dispatcher, log)
	handler := NewHandler(hub, log)

	// Register health check handler
	RegisterHealthHandler(dispatcher)

	return &Gateway{
		Hub:        hub,
		Dispatcher: dispatcher,
		Handler:    handler,
		logger:     log,
	}
}

// SetLifecycleManager enables the dedicated terminal WebSocket handler for passthrough mode.
// This must be called before SetupRoutes if terminal passthrough is needed.
func (g *Gateway) SetLifecycleManager(lifecycleMgr *lifecycle.Manager, userService UserService, scriptService scripts.ScriptService) {
	g.TerminalHandler = NewTerminalHandler(lifecycleMgr, userService, scriptService, g.logger)
}

// SetLSPHandler enables the LSP WebSocket handler.
func (g *Gateway) SetLSPHandler(controller LSPAttachmentResolver) {
	g.LSPHandler = NewLSPHandler(controller, g.logger)
}

// SetVscodeProxy enables the VS Code reverse proxy handler.
func (g *Gateway) SetVscodeProxy(lifecycleMgr *lifecycle.Manager) {
	g.VscodeProxyHandler = NewVscodeProxyHandler(lifecycleMgr, g.logger)
}

// SetPortProxy enables the generic port reverse proxy handler.
func (g *Gateway) SetPortProxy(lifecycleMgr *lifecycle.Manager) {
	g.PortProxyHandler = NewPortProxyHandler(lifecycleMgr, g.logger)
}

// SetPortTunnel enables dedicated port tunnel management.
func (g *Gateway) SetPortTunnel(lifecycleMgr *lifecycle.Manager) {
	g.TunnelManager = NewTunnelManager(lifecycleMgr, g.logger)
}

// SetupRoutes adds the WebSocket routes to the Gin engine. Every route is
// guarded by requireConnectionAuth — the global HTTP auth middleware defers
// these paths (upgrades and iframe proxies can't be challenged with a JSON
// 401 mid-flow by the generic layer), so enforcement happens here, where the
// ?token=<PAT> fallback for headerless clients is also honored.
func (g *Gateway) SetupRoutes(router *gin.Engine) {
	connectionAuth := g.requireConnectionAuth()
	router.GET("/ws", connectionAuth, g.Handler.HandleConnection)

	// Add dedicated terminal WebSocket route if terminal handler is configured
	if g.TerminalHandler != nil {
		router.GET("/terminal/*target", connectionAuth, g.TerminalHandler.HandleTerminalWS)
	}

	// Add LSP routes if LSP handler is configured
	if g.LSPHandler != nil {
		router.GET("/lsp/tasks/:taskId/:language/attach", connectionAuth, g.LSPHandler.HandleLSPConnection)
	}

	// Add VS Code reverse proxy routes if configured
	if g.VscodeProxyHandler != nil {
		router.Any("/vscode/:sessionId/*path", connectionAuth, g.VscodeProxyHandler.HandleVscodeProxy)
	}

	// Add generic port proxy routes if configured
	if g.PortProxyHandler != nil {
		router.Any("/port-proxy/:sessionId/:port", connectionAuth, g.PortProxyHandler.HandlePortProxy)
		router.Any("/port-proxy/:sessionId/:port/*path", connectionAuth, g.PortProxyHandler.HandlePortProxy)
	}
}
