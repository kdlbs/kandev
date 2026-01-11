// Package websocket provides a unified WebSocket gateway for all API operations.
package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// HistoricalLogsProvider is a function that retrieves historical logs for a task
type HistoricalLogsProvider func(ctx context.Context, taskID string) ([]*ws.Message, error)

// Hub manages all WebSocket client connections
type Hub struct {
	// All registered clients
	clients map[*Client]bool

	// Clients subscribed to specific tasks (for ACP notifications)
	taskSubscribers map[string]map[*Client]bool

	// Channels for client management
	register   chan *Client
	unregister chan *Client

	// Channel for broadcasting notifications
	broadcast chan *ws.Message

	// Message dispatcher
	dispatcher *ws.Dispatcher

	// Optional provider for historical logs on subscription
	historicalLogsProvider HistoricalLogsProvider

	mu     sync.RWMutex
	logger *logger.Logger
}

// NewHub creates a new WebSocket hub
func NewHub(dispatcher *ws.Dispatcher, log *logger.Logger) *Hub {
	return &Hub{
		clients:         make(map[*Client]bool),
		taskSubscribers: make(map[string]map[*Client]bool),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		broadcast:       make(chan *ws.Message, 256),
		dispatcher:      dispatcher,
		logger:          log.WithFields(zap.String("component", "ws_hub")),
	}
}

// Run starts the hub's main processing loop
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("WebSocket hub started")
	defer h.logger.Info("WebSocket hub stopped")

	for {
		select {
		case <-ctx.Done():
			h.closeAllClients()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("Client registered", zap.String("client_id", client.ID))

		case client := <-h.unregister:
			h.removeClient(client)

		case msg := <-h.broadcast:
			h.broadcastMessage(msg)
		}
	}
}

// closeAllClients closes all client connections
func (h *Hub) closeAllClients() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		close(client.send)
		delete(h.clients, client)
	}
	h.taskSubscribers = make(map[string]map[*Client]bool)
}

// removeClient removes a client from the hub
func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)

		// Remove from all task subscriptions
		for taskID := range client.subscriptions {
			if clients, ok := h.taskSubscribers[taskID]; ok {
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.taskSubscribers, taskID)
				}
			}
		}
	}
	h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
}

// broadcastMessage sends a message to relevant clients
func (h *Hub) broadcastMessage(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal broadcast message", zap.Error(err))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// For now, broadcast to all clients
	// TODO: Add topic-based routing for task-specific notifications
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client buffer full, will be cleaned up by write pump
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

// Broadcast sends a notification to all connected clients
func (h *Hub) Broadcast(msg *ws.Message) {
	h.broadcast <- msg
}

// BroadcastToTask sends a notification to clients subscribed to a specific task
func (h *Hub) BroadcastToTask(taskID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}

	h.mu.RLock()
	clients := h.taskSubscribers[taskID]
	h.mu.RUnlock()

	for client := range clients {
		select {
		case client.send <- data:
		default:
			// Buffer full
		}
	}
}

// SubscribeToTask subscribes a client to task notifications
func (h *Hub) SubscribeToTask(client *Client, taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.taskSubscribers[taskID]; !ok {
		h.taskSubscribers[taskID] = make(map[*Client]bool)
	}
	h.taskSubscribers[taskID][client] = true
	client.subscriptions[taskID] = true

	h.logger.Debug("Client subscribed to task",
		zap.String("client_id", client.ID),
		zap.String("task_id", taskID))
}

// UnsubscribeFromTask unsubscribes a client from task notifications
func (h *Hub) UnsubscribeFromTask(client *Client, taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.subscriptions, taskID)
	if clients, ok := h.taskSubscribers[taskID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.taskSubscribers, taskID)
		}
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetDispatcher returns the message dispatcher
func (h *Hub) GetDispatcher() *ws.Dispatcher {
	return h.dispatcher
}

// SetHistoricalLogsProvider sets the provider for historical logs on subscription
func (h *Hub) SetHistoricalLogsProvider(provider HistoricalLogsProvider) {
	h.historicalLogsProvider = provider
}

// GetHistoricalLogs retrieves historical logs for a task if a provider is set
func (h *Hub) GetHistoricalLogs(ctx context.Context, taskID string) ([]*ws.Message, error) {
	if h.historicalLogsProvider == nil {
		return nil, nil
	}
	return h.historicalLogsProvider(ctx, taskID)
}