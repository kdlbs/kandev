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
	// Clients subscribed to specific office run ids (for run.event.appended).
	runSubscribers map[string]map[*Client]bool
	// Clients subscribed to backend/resource metrics.
	systemMetricsSubscribers map[*Client]bool

	// Channels for client management
	register   chan *Client
	unregister chan *Client

	// Channel for broadcasting notifications
	broadcast chan *ws.Message

	// Message dispatcher
	dispatcher *ws.Dispatcher

	// Optional provider for session data on subscription (e.g., git status)
	sessionDataProvider SessionDataProvider

	// sessionMode tracks per-session focus state and fires listeners when
	// effective mode (paused/slow/fast) transitions. See hub_session_mode.go.
	sessionMode            *sessionModeTracker
	metricsInterestTracker SystemMetricsInterestTracker

	// dispatchCtx is the hub's lifetime context, set by Run. Dispatched
	// message handlers use it instead of the per-connection context so that
	// a client disconnecting mid-flight does not SIGKILL exec subprocesses
	// (gh, git, agentctl HTTP calls) or otherwise abort side-effecting work
	// like session.launch. It still cancels on server shutdown.
	dispatchCtx context.Context

	mu     sync.RWMutex
	logger *logger.Logger
}

// NewHub creates a new WebSocket hub
func NewHub(dispatcher *ws.Dispatcher, log *logger.Logger) *Hub {
	return &Hub{
		clients:                  make(map[*Client]bool),
		taskSubscribers:          make(map[string]map[*Client]bool),
		sessionSubscribers:       make(map[string]map[*Client]bool),
		userSubscribers:          make(map[string]map[*Client]bool),
		runSubscribers:           make(map[string]map[*Client]bool),
		systemMetricsSubscribers: make(map[*Client]bool),
		register:                 make(chan *Client),
		unregister:               make(chan *Client),
		broadcast:                make(chan *ws.Message, 256),
		dispatcher:               dispatcher,
		sessionMode:              newSessionModeTracker(),
		logger:                   log.WithFields(zap.String("component", "ws_hub")),
	}
}

type SystemMetricsInterestTracker interface {
	MetricsSubscribe(clientID string)
	MetricsUnsubscribe(clientID string)
}

// Run starts the hub's main processing loop
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("WebSocket hub started")
	defer h.logger.Info("WebSocket hub stopped")

	h.mu.Lock()
	h.dispatchCtx = ctx
	h.mu.Unlock()

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

// closeAllClients closes all client connections.
// Cancels any pending debounced session-mode transitions so timers don't fire
// after shutdown and call into listeners with stale state.
func (h *Hub) closeAllClients() {
	h.mu.Lock()
	metricClientIDs := make([]string, 0, len(h.systemMetricsSubscribers))
	for client := range h.clients {
		if client.systemMetricsSubscribed {
			metricClientIDs = append(metricClientIDs, client.ID)
			client.systemMetricsSubscribed = false
		}
		client.closeSend()
		delete(h.clients, client)
	}
	tracker := h.metricsInterestTracker
	h.taskSubscribers = make(map[string]map[*Client]bool)
	h.sessionSubscribers = make(map[string]map[*Client]bool)
	h.runSubscribers = make(map[string]map[*Client]bool)
	h.systemMetricsSubscribers = make(map[*Client]bool)
	h.sessionMode.focusByClient = make(map[string]map[*Client]bool)
	h.mu.Unlock()

	for _, clientID := range metricClientIDs {
		if tracker != nil {
			tracker.MetricsUnsubscribe(clientID)
		}
	}

	h.stopAllPendingTransitions()
}

// removeClient removes a client from the hub
func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()

	if _, ok := h.clients[client]; !ok {
		h.mu.Unlock()
		h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
		return
	}

	delete(h.clients, client)
	client.closeSend()

	// Remove from all task subscriptions
	for taskID := range client.subscriptions {
		removeClientFromSubscriberMap(h.taskSubscribers, taskID, client)
	}
	// Capture session IDs that need mode recomputation after we drop the lock.
	// Disconnect can change mode either way: removing the last subscriber drops
	// to paused, removing the last focuser drops fast → slow.
	affectedSessions := make([]string, 0, len(client.sessionSubscriptions)+len(client.sessionFocus))
	for sessionID := range client.sessionSubscriptions {
		removeClientFromSubscriberMap(h.sessionSubscribers, sessionID, client)
		affectedSessions = append(affectedSessions, sessionID)
	}
	for sessionID := range client.sessionFocus {
		removeClientFromSubscriberMap(h.sessionMode.focusByClient, sessionID, client)
		affectedSessions = append(affectedSessions, sessionID)
	}
	for userID := range client.userSubscriptions {
		removeClientFromSubscriberMap(h.userSubscribers, userID, client)
	}
	for runID := range client.runSubscriptions {
		removeClientFromSubscriberMap(h.runSubscribers, runID, client)
	}
	var metricClientID string
	var tracker SystemMetricsInterestTracker
	if client.systemMetricsSubscribed {
		delete(h.systemMetricsSubscribers, client)
		client.systemMetricsSubscribed = false
		metricClientID = client.ID
		tracker = h.metricsInterestTracker
	}
	h.mu.Unlock()

	if tracker != nil && metricClientID != "" {
		tracker.MetricsUnsubscribe(metricClientID)
	}

	for _, sessionID := range dedupStrings(affectedSessions) {
		h.recomputeSessionMode(sessionID)
	}

	h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
}

func (h *Hub) SetSystemMetricsInterestTracker(tracker SystemMetricsInterestTracker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metricsInterestTracker = tracker
}

// dedupStrings returns the input with duplicates removed, preserving order.
// Used to call recomputeSessionMode at most once per affected session when a
// client is both subscribed and focused.
func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
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

// getSessionRecipientsLocked returns the deduped set of clients that should
// receive a session-scoped broadcast: those subscribed to the session OR
// focused on it.
//
// Focus is the stable "actively viewing this session" signal — it's held for
// the whole time the task page is open. The ref-counted session.subscribe, by
// contrast, churns to 0 during task-switch/resume (the sidebar hands the
// active session off to the task-page hooks, and the resume state transitions
// re-run the subscription effects). If a client is focused but its subscribe
// ref-count was transiently dropped, it must still receive session events
// (e.g. the session.message.updated that marks an agent_boot script_execution
// completed) — otherwise the UI is stuck until a manual refetch.
func (h *Hub) getSessionRecipientsLocked(sessionID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	subs := h.sessionSubscribers[sessionID]
	focus := h.sessionMode.focusByClient[sessionID]
	clients := make([]*Client, 0, len(subs)+len(focus))
	seen := make(map[*Client]struct{}, len(subs)+len(focus))
	for client := range subs {
		seen[client] = struct{}{}
		clients = append(clients, client)
	}
	for client := range focus {
		if _, ok := seen[client]; ok {
			continue
		}
		clients = append(clients, client)
	}
	return clients
}

// BroadcastToSession sends a notification to clients subscribed to OR focused on
// a specific session. See getSessionRecipientsLocked for why focus is included.
func (h *Hub) BroadcastToSession(sessionID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	clients := h.getSessionRecipientsLocked(sessionID)
	h.logger.Debug("BroadcastToSession",
		zap.String("session_id", sessionID),
		zap.String("action", msg.Action),
		zap.Int("recipient_count", len(clients)))
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
	if _, ok := h.sessionSubscribers[sessionID]; !ok {
		h.sessionSubscribers[sessionID] = make(map[*Client]bool)
	}
	h.sessionSubscribers[sessionID][client] = true
	client.sessionSubscriptions[sessionID] = true
	h.mu.Unlock()

	h.logger.Debug("Client subscribed to session",
		zap.String("client_id", client.ID),
		zap.String("session_id", sessionID))

	h.recomputeSessionMode(sessionID)
}

// UnsubscribeFromSession unsubscribes a client from session notifications
func (h *Hub) UnsubscribeFromSession(client *Client, sessionID string) {
	h.mu.Lock()
	delete(client.sessionSubscriptions, sessionID)
	if clients, ok := h.sessionSubscribers[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessionSubscribers, sessionID)
		}
	}
	h.mu.Unlock()

	h.recomputeSessionMode(sessionID)
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

// BroadcastToRun sends a notification to clients subscribed to a specific office run id.
func (h *Hub) BroadcastToRun(runID string, msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	clients := h.getSubscribersLocked(h.runSubscribers, runID)
	h.logger.Debug("BroadcastToRun",
		zap.String("run_id", runID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.sendToClients(data, clients, msg.Action)
}

func (h *Hub) BroadcastToSystemMetrics(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.systemMetricsSubscribers))
	for client := range h.systemMetricsSubscribers {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	h.sendToClients(data, clients, msg.Action)
}

// SubscribeToRun subscribes a client to office run-event notifications.
func (h *Hub) SubscribeToRun(client *Client, runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.runSubscribers[runID]; !ok {
		h.runSubscribers[runID] = make(map[*Client]bool)
	}
	h.runSubscribers[runID][client] = true
	client.runSubscriptions[runID] = true

	h.logger.Debug("Client subscribed to run",
		zap.String("client_id", client.ID),
		zap.String("run_id", runID))
}

// UnsubscribeFromRun unsubscribes a client from office run-event notifications.
func (h *Hub) UnsubscribeFromRun(client *Client, runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.runSubscriptions, runID)
	if clients, ok := h.runSubscribers[runID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.runSubscribers, runID)
		}
	}
}

func (h *Hub) SubscribeToSystemMetrics(client *Client) {
	h.mu.Lock()
	if client.systemMetricsSubscribed {
		h.mu.Unlock()
		return
	}
	h.systemMetricsSubscribers[client] = true
	client.systemMetricsSubscribed = true
	tracker := h.metricsInterestTracker
	h.mu.Unlock()

	if tracker != nil {
		tracker.MetricsSubscribe(client.ID)
	}
}

func (h *Hub) UnsubscribeFromSystemMetrics(client *Client) {
	h.mu.Lock()
	if !client.systemMetricsSubscribed {
		h.mu.Unlock()
		return
	}
	delete(h.systemMetricsSubscribers, client)
	client.systemMetricsSubscribed = false
	tracker := h.metricsInterestTracker
	h.mu.Unlock()

	if tracker != nil {
		tracker.MetricsUnsubscribe(client.ID)
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

// DispatchContext returns a context whose lifetime is tied to the hub (and
// therefore the server) rather than any single client connection. Dispatched
// handlers should use this so that a client disconnecting mid-flight does not
// cancel in-progress writes, exec subprocesses, or downstream HTTP calls.
// Falls back to context.Background when Run has not been called (test setups).
func (h *Hub) DispatchContext() context.Context {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.dispatchCtx == nil {
		return context.Background()
	}
	return h.dispatchCtx
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
