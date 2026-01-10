// Package streaming provides WebSocket streaming handlers for ACP messages.
package streaming

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, validate origin
		return true
	},
}

// WSHandler handles WebSocket connections
type WSHandler struct {
	hub     *Hub
	service *orchestrator.Service
	logger  *logger.Logger
}

// NewWSHandler creates a new WebSocket handler
func NewWSHandler(hub *Hub, service *orchestrator.Service, log *logger.Logger) *WSHandler {
	return &WSHandler{
		hub:     hub,
		service: service,
		logger:  log.WithFields(zap.String("component", "ws_handler")),
	}
}

// StreamTask handles WebSocket connection for a specific task
// WS /api/v1/orchestrator/tasks/:taskId/stream
func (h *WSHandler) StreamTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "MISSING_TASK_ID",
				"message": "Task ID is required",
			},
		})
		return
	}

	// Validate JWT from query param or header (optional for now)
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Authorization")
	}
	// TODO: Implement proper JWT validation
	_ = token

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return
	}

	// Create a unique client ID
	clientID := uuid.New().String()

	h.logger.Info("WebSocket connection established for task",
		zap.String("client_id", clientID),
		zap.String("task_id", taskID),
	)

	// Create client and register with hub
	client := NewClient(clientID, conn, h.hub, h.logger)

	// Register client with hub first
	h.hub.Register(client)

	// Subscribe client to the specific task
	client.Subscribe(taskID)

	// Start read and write pumps in separate goroutines
	go client.WritePump()
	go client.ReadPump()
}

// StreamAll handles WebSocket connection for all tasks (with subscription)
// WS /api/v1/orchestrator/stream
func (h *WSHandler) StreamAll(c *gin.Context) {
	// Validate JWT from query param or header (optional for now)
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Authorization")
	}
	// TODO: Implement proper JWT validation
	_ = token

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}

	// Create a unique client ID
	clientID := uuid.New().String()

	h.logger.Info("WebSocket connection established for all tasks",
		zap.String("client_id", clientID),
	)

	// Create client and register with hub
	client := NewClient(clientID, conn, h.hub, h.logger)

	// Register client with hub
	h.hub.Register(client)

	// Start read and write pumps in separate goroutines
	// The ReadPump handles subscription messages from the client
	go client.WritePump()
	go client.ReadPump()
}

// SetupWebSocketRoutes adds WebSocket routes to the router
func SetupWebSocketRoutes(router *gin.RouterGroup, handler *WSHandler) {
	// Stream for a specific task
	router.GET("/tasks/:taskId/stream", handler.StreamTask)

	// Stream for all tasks (with dynamic subscription)
	router.GET("/stream", handler.StreamAll)
}

