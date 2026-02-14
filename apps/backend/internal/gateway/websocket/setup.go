package websocket

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	"github.com/kandev/kandev/internal/lsp/installer"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Gateway represents the unified WebSocket gateway
type Gateway struct {
	Hub             *Hub
	Dispatcher      *ws.Dispatcher
	Handler         *Handler
	TerminalHandler *TerminalHandler
	LSPHandler      *LSPHandler
	logger          *logger.Logger
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
func (g *Gateway) SetLSPHandler(lifecycleMgr *lifecycle.Manager, userService LSPUserService, installerRegistry *installer.Registry) {
	g.LSPHandler = NewLSPHandler(lifecycleMgr, userService, installerRegistry, g.logger)
}

// SetupRoutes adds the WebSocket routes to the Gin engine
func (g *Gateway) SetupRoutes(router *gin.Engine) {
	router.GET("/ws", g.Handler.HandleConnection)

	// Add dedicated terminal WebSocket route if terminal handler is configured
	if g.TerminalHandler != nil {
		router.GET("/xterm.js/:sessionId", g.TerminalHandler.HandleTerminalWS)
	}

	// Add LSP routes if LSP handler is configured
	if g.LSPHandler != nil {
		router.GET("/lsp/:sessionId", g.LSPHandler.HandleLSPConnection)
	}
}

