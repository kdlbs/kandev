package websocket

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Gateway represents the unified WebSocket gateway
type Gateway struct {
	Hub        *Hub
	Dispatcher *ws.Dispatcher
	Handler    *Handler
	logger     *logger.Logger
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

// SetupRoutes adds the WebSocket routes to the Gin engine
func (g *Gateway) SetupRoutes(router *gin.Engine) {
	router.GET("/ws", g.Handler.HandleConnection)
}

