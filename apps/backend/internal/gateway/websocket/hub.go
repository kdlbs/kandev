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

// SessionDataProvider is a function that retrieves initial data for a session subscription (e.g., git status)
type SessionDataProvider func(ctx context.Context, sessionID string) ([]*ws.Message, error)

// Hub manages all WebSocket client connections
type Hub struct {
	// All registered clients
	clients map[*Client]bool

	// Clients subscribed to specific tasks (for ACP notifications)
	taskSubscribers map[string]map[*Client]bool
	// Clients subscribed to specific sessions
	sessionSubscribers map[string]map[*Client]bool
	// Clients subscribed to specific users (for user settings notifications)
	userSubscribers map[string]map[*Client]bool

	// Channels for client management
	register   chan *Client
	unregister chan *Client

	// Channel for broadcasting notifications
	broadcast chan *ws.Message

	// Message dispatcher
	dispatcher *ws.Dispatcher

	// Optional provider for session data on subscription (e.g., git status)
	sessionDataProvider SessionDataProvider

	mu     sync.RWMutex
	logger *logger.Logger
}

// NewHub creates a new WebSocket hub
func NewHub(dispatcher *ws.Dispatcher, log *logger.Logger) *Hub {
	return &Hub{
		clients:            make(map[*Client]bool),
		taskSubscribers:    make(map[string]map[*Client]bool),
		sessionSubscribers: make(map[string]map[*Client]bool),
		userSubscribers:    make(map[string]map[*Client]bool),
		register:           make(chan *Client),
		unregister:         make(chan *Client),
		broadcast:          make(chan *ws.Message, 256),
		dispatcher:         dispatcher,
		logger:             log.WithFields(zap.String("component", "ws_hub")),
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
		client.closeSend()
		delete(h.clients, client)
	}
	h.taskSubscribers = make(map[string]map[*Client]bool)
	h.sessionSubscribers = make(map[string]map[*Client]bool)
}

// removeClient removes a client from the hub
func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; !ok {
		h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
		return
	}

	delete(h.clients, client)
	client.closeSend()

	// Remove from all task subscriptions
	for taskID := range client.subscriptions {
		removeClientFromSubscriberMap(h.taskSubscribers, taskID, client)
	}
	for sessionID := range client.sessionSubscriptions {
		removeClientFromSubscriberMap(h.sessionSubscribers, sessionID, client)
	}
	for userID := range client.userSubscriptions {
		removeClientFromSubscriberMap(h.userSubscribers, userID, client)
	}

	h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
}

// removeClientFromSubscriberMap removes a client from a subscriber map entry,
// deleting the entry entirely when no subscribers remain.
func removeClientFromSubscriberMap(subscribers map[string]map[*Client]bool, key string, client *Client) {
	clients, ok := subscribers[key]
	if !ok {
		return
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(subscribers, key)
	}
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
		client.sendBytes(data)
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

// getSubscribersLocked reads subscribers for an ID from a subscriber map under the read lock.
func (h *Hub) getSubscribersLocked(m map[string]map[*Client]bool, id string) []*Client {
	h.mu.RLock()
	subscriberMap := m[id]
	clients := make([]*Client, 0, len(subscriberMap))
	for client := range subscriberMap {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	return clients
}

// sendToClients delivers a pre-marshalled message to a list of clients.
func (h *Hub) sendToClients(data []byte, clients []*Client, action string) {
	for _, client := range clients {
		if client.sendBytes(data) {
			h.logger.Debug("Sent message to client",
				zap.String("client_id", client.ID),
				zap.String("action", action))
		} else {
			h.logger.Warn("Client send buffer full, dropping message",
				zap.String("client_id", client.ID),
				zap.String("action", action))
		}
	}
}

// BroadcastToTask sends a notification to clients subscribed to a specific task
func (h *Hub) BroadcastToTask(taskID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	clients := h.getSubscribersLocked(h.taskSubscribers, taskID)
	h.logger.Debug("BroadcastToTask",
		zap.String("task_id", taskID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.sendToClients(data, clients, msg.Action)
}

// BroadcastToSession sends a notification to clients subscribed to a specific session
func (h *Hub) BroadcastToSession(sessionID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	clients := h.getSubscribersLocked(h.sessionSubscribers, sessionID)
	h.logger.Debug("BroadcastToSession",
		zap.String("session_id", sessionID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.sendToClients(data, clients, msg.Action)
}

// BroadcastToUser sends a notification to clients subscribed to a specific user
func (h *Hub) BroadcastToUser(userID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	clients := h.getSubscribersLocked(h.userSubscribers, userID)
	h.logger.Debug("BroadcastToUser",
		zap.String("user_id", userID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.sendToClients(data, clients, msg.Action)
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

// SubscribeToSession subscribes a client to session notifications
func (h *Hub) SubscribeToSession(client *Client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.sessionSubscribers[sessionID]; !ok {
		h.sessionSubscribers[sessionID] = make(map[*Client]bool)
	}
	h.sessionSubscribers[sessionID][client] = true
	client.sessionSubscriptions[sessionID] = true

	h.logger.Debug("Client subscribed to session",
		zap.String("client_id", client.ID),
		zap.String("session_id", sessionID))
}

// UnsubscribeFromSession unsubscribes a client from session notifications
func (h *Hub) UnsubscribeFromSession(client *Client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.sessionSubscriptions, sessionID)
	if clients, ok := h.sessionSubscribers[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessionSubscribers, sessionID)
		}
	}
}

// SubscribeToUser subscribes a client to user notifications
func (h *Hub) SubscribeToUser(client *Client, userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.userSubscribers[userID]; !ok {
		h.userSubscribers[userID] = make(map[*Client]bool)
	}
	h.userSubscribers[userID][client] = true
	client.userSubscriptions[userID] = true

	h.logger.Debug("Client subscribed to user",
		zap.String("client_id", client.ID),
		zap.String("user_id", userID))
}

// UnsubscribeFromUser unsubscribes a client from user notifications
func (h *Hub) UnsubscribeFromUser(client *Client, userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.userSubscriptions, userID)
	if clients, ok := h.userSubscribers[userID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.userSubscribers, userID)
		}
	}
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

// SetSessionDataProvider sets the provider for session data on subscription
func (h *Hub) SetSessionDataProvider(provider SessionDataProvider) {
	h.sessionDataProvider = provider
}

// GetSessionData retrieves session data (e.g., git status) if a provider is set
func (h *Hub) GetSessionData(ctx context.Context, sessionID string) ([]*ws.Message, error) {
	if h.sessionDataProvider == nil {
		return nil, nil
	}
	return h.sessionDataProvider(ctx, sessionID)
}