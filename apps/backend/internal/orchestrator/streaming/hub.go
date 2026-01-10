// Package streaming handles WebSocket connections for real-time ACP message streaming.
package streaming

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/acp/protocol"
	"go.uber.org/zap"
)

// Client represents a WebSocket client connection
type Client struct {
	ID      string
	conn    *websocket.Conn
	taskIDs map[string]bool // Tasks this client is subscribed to
	send    chan []byte
	hub     *Hub
	mu      sync.RWMutex
	logger  *logger.Logger
}

// NewClient creates a new WebSocket client
func NewClient(id string, conn *websocket.Conn, hub *Hub, log *logger.Logger) *Client {
	return &Client{
		ID:      id,
		conn:    conn,
		taskIDs: make(map[string]bool),
		send:    make(chan []byte, 256),
		hub:     hub,
		logger:  log.WithFields(zap.String("client_id", id)),
	}
}

// Hub manages all WebSocket clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Clients by task ID for efficient message routing
	taskClients map[string]map[*Client]bool

	// Channels
	register   chan *Client
	unregister chan *Client
	broadcast  chan *BroadcastMessage

	mu     sync.RWMutex
	logger *logger.Logger
}

// BroadcastMessage contains a message to broadcast
type BroadcastMessage struct {
	TaskID  string
	Message *protocol.Message
}

// NewHub creates a new WebSocket hub
func NewHub(log *logger.Logger) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		taskClients: make(map[string]map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *BroadcastMessage, 256),
		logger:      log.WithFields(zap.String("component", "websocket_hub")),
	}
}

// Run starts the hub processing loop
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("WebSocket hub started")
	defer h.logger.Info("WebSocket hub stopped")

	for {
		select {
		case <-ctx.Done():
			// Close all client connections
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.taskClients = make(map[string]map[*Client]bool)
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("Client registered", zap.String("client_id", client.ID))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)

				// Remove from all task subscriptions
				for taskID := range client.taskIDs {
					if clients, ok := h.taskClients[taskID]; ok {
						delete(clients, client)
						if len(clients) == 0 {
							delete(h.taskClients, taskID)
						}
					}
				}
			}
			h.mu.Unlock()
			h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients := h.taskClients[msg.TaskID]
			h.mu.RUnlock()

			if len(clients) == 0 {
				continue
			}

			data, err := json.Marshal(msg.Message)
			if err != nil {
				h.logger.Error("Failed to marshal message", zap.Error(err))
				continue
			}

			for client := range clients {
				select {
				case client.send <- data:
				default:
					// Client send buffer is full, close connection
					h.mu.Lock()
					close(client.send)
					delete(h.clients, client)
					for taskID := range client.taskIDs {
						if taskClients, ok := h.taskClients[taskID]; ok {
							delete(taskClients, client)
							if len(taskClients) == 0 {
								delete(h.taskClients, taskID)
							}
						}
					}
					h.mu.Unlock()
				}
			}
		}
	}
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast sends a message to all clients subscribed to a task
func (h *Hub) Broadcast(taskID string, msg *protocol.Message) {
	h.broadcast <- &BroadcastMessage{
		TaskID:  taskID,
		Message: msg,
	}
}

// SubscribeClient subscribes a client to a task
func (h *Hub) SubscribeClient(client *Client, taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.taskClients[taskID]; !ok {
		h.taskClients[taskID] = make(map[*Client]bool)
	}
	h.taskClients[taskID][client] = true
	h.logger.Debug("Client subscribed to task",
		zap.String("client_id", client.ID),
		zap.String("task_id", taskID))
}

// UnsubscribeClient unsubscribes a client from a task
func (h *Hub) UnsubscribeClient(client *Client, taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.taskClients[taskID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.taskClients, taskID)
		}
	}
	h.logger.Debug("Client unsubscribed from task",
		zap.String("client_id", client.ID),
		zap.String("task_id", taskID))
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetTaskSubscriberCount returns the number of clients subscribed to a task
func (h *Hub) GetTaskSubscriberCount(taskID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if clients, ok := h.taskClients[taskID]; ok {
		return len(clients)
	}
	return 0
}

