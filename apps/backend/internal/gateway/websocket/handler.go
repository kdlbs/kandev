package websocket

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
}

// Handler handles WebSocket connections
type Handler struct {
	hub    *Hub
	logger *logger.Logger
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub, log *logger.Logger) *Handler {
	return &Handler{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws_handler")),
	}
}

// HandleConnection upgrades HTTP to WebSocket and handles messages.
//
// Authentication happens before this handler: the global HTTP middleware
// resolves cookie/bearer credentials, and the gateway's requireConnectionAuth
// route guard rejects unauthenticated attempts (and accepts ?token=<PAT>)
// when auth is enforced. Whatever identity survived lands on the client so
// subscriptions, dispatched actions, and broadcast routing are scoped to it.
func (h *Handler) HandleConnection(c *gin.Context) {
	identity, _ := authn.FromGin(c)

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}

	// Create a unique client ID
	clientID := uuid.New().String()

	h.logger.Debug("WebSocket connection established",
		zap.String("client_id", clientID),
		zap.String("user_id", identity.UserID),
		zap.String("remote_addr", c.Request.RemoteAddr),
	)

	// Create client and register with hub
	client := NewClient(clientID, identity, conn, h.hub, h.logger)

	// Register client with hub
	h.hub.Register(client)

	// Start read and write pumps
	go client.WritePump()
	client.ReadPump(c.Request.Context())
}

// RegisterHealthHandler registers the health check handler
func RegisterHealthHandler(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionHealthCheck, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"status":  "ok",
			"service": "kandev",
			"mode":    "unified",
		})
	})
}
