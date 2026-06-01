// Package websocket provides a unified WebSocket gateway for all API operations.
package websocket

import (
	"context"
	"sync"
	"time"

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
	sessionMode *sessionModeTracker

	// dispatchCtx is the hub's lifetime context, set by Run. Dispatched
	// message handlers use it instead of the per-connection context so that
	// a client disconnecting mid-flight does not SIGKILL exec subprocesses
	// (gh, git, agentctl HTTP calls) or otherwise abort side-effecting work
	// like session.launch. It still cancels on server shutdown.
	dispatchCtx context.Context

	// sessionSeqs holds the per-session monotonic counter used to stamp
	// SessionSeq on session-routed outbound envelopes (Phase 2 WS
	// accounting). The value type is *atomic.Int64 so stamping is
	// lock-free. Entries are created on SubscribeToSession and removed
	// by sessionSubscriberCounts dropping to zero — see hub_session_seq.go.
	sessionSeqs sync.Map // map[string]*atomic.Int64
	// sessionSubscriberCounts tracks how many clients are currently
	// subscribed to each session ID so the per-session counter can be
	// dropped when the last subscriber leaves. Independent of
	// sessionSubscribers (which is a per-session set of *Client used for
	// fan-out) to keep the lifecycle decoupled from the focus tracker.
	sessionSubscriberCounts sync.Map // map[string]*atomic.Int64

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
		runSubscribers:     make(map[string]map[*Client]bool),
		register:           make(chan *Client),
		unregister:         make(chan *Client),
		broadcast:          make(chan *ws.Message, 256),
		dispatcher:         dispatcher,
		sessionMode:        newSessionModeTracker(),
		logger:             log.WithFields(zap.String("component", "ws_hub")),
	}
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
	for client := range h.clients {
		client.closeSend()
		delete(h.clients, client)
	}
	h.taskSubscribers = make(map[string]map[*Client]bool)
	h.sessionSubscribers = make(map[string]map[*Client]bool)
	h.runSubscribers = make(map[string]map[*Client]bool)
	h.sessionMode.focusByClient = make(map[string]map[*Client]bool)
	h.mu.Unlock()

	// Drain the session-seq lifecycle maps. sync.Map can't be reset by
	// reassigning a new instance because consumers hold method receivers, so
	// walk both maps and Delete the keys we see. Hub shutdown is a single-
	// threaded event from the harness's perspective so we don't race with
	// concurrent Sub/Unsub here.
	h.sessionSeqs.Range(func(key, _ any) bool {
		h.sessionSeqs.Delete(key)
		return true
	})
	h.sessionSubscriberCounts.Range(func(key, _ any) bool {
		h.sessionSubscriberCounts.Delete(key)
		return true
	})

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
	// Also capture the per-session-seq lifecycle decrements so they happen
	// outside the hub lock (sync.Map is internally synchronized but we still
	// shouldn't hold h.mu longer than necessary).
	sessionSeqDecrements := make([]string, 0, len(client.sessionSubscriptions))
	for sessionID := range client.sessionSubscriptions {
		removeClientFromSubscriberMap(h.sessionSubscribers, sessionID, client)
		affectedSessions = append(affectedSessions, sessionID)
		sessionSeqDecrements = append(sessionSeqDecrements, sessionID)
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
	h.mu.Unlock()

	// Decrement session-seq lifecycle counters outside the hub lock. Each
	// decrement deletes the counter entry when it reaches zero, ensuring
	// disconnects without explicit session.unsubscribe still drain the maps.
	for _, sessionID := range sessionSeqDecrements {
		h.decSessionSubscribers(sessionID)
	}

	for _, sessionID := range dedupStrings(affectedSessions) {
		h.recomputeSessionMode(sessionID)
	}

	h.logger.Debug("Client unregistered", zap.String("client_id", client.ID))
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

// broadcastMessage sends a message to every connected client. Each gets its
// own marshalled frame because seq is stamped per-connection — we can't share
// a pre-marshalled buffer the way we did before seq accounting landed.
func (h *Hub) broadcastMessage(msg *ws.Message) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	h.fanoutToClients(msg, clients)
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

// fanoutToClients stamps a per-connection seq onto a clone of msg for each
// client and enqueues the marshalled bytes. Used by non-session routing paths
// (BroadcastToTask, BroadcastToUser, BroadcastToRun, broadcastMessage) where
// the routing key isn't a session ID. Marshals per client (cost: O(N))
// because seq must be unique per connection.
func (h *Hub) fanoutToClients(msg *ws.Message, clients []*Client) {
	h.fanoutToClientsForSession("", msg, clients)
}

// fanoutToClientsForSession is fanoutToClients that additionally stamps a
// per-session SessionSeq on each clone using the hub's session-seq counter
// for sessionID. Used by BroadcastToSession so the per-session stream a
// single client receives is a strictly monotonic seq sequence — cross-session
// misrouting becomes a SessionSeq gap on the receiver.
func (h *Hub) fanoutToClientsForSession(sessionID string, msg *ws.Message, clients []*Client) {
	for _, client := range clients {
		if client.sendStampedCopyForSession(sessionID, msg) {
			h.logger.Debug("Sent message to client",
				zap.String("client_id", client.ID),
				zap.String("action", msg.Action))
		} else {
			h.logger.Warn("Client send buffer full, dropping message",
				zap.String("client_id", client.ID),
				zap.String("action", msg.Action))
		}
	}
}

// BroadcastToTask sends a notification to clients subscribed to a specific task
func (h *Hub) BroadcastToTask(taskID string, msg *ws.Message) {
	clients := h.getSubscribersLocked(h.taskSubscribers, taskID)
	h.logger.Debug("BroadcastToTask",
		zap.String("task_id", taskID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.fanoutToClients(msg, clients)
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
	clients := h.getSessionRecipientsLocked(sessionID)
	h.logger.Debug("BroadcastToSession",
		zap.String("session_id", sessionID),
		zap.String("action", msg.Action),
		zap.Int("recipient_count", len(clients)))
	h.fanoutToClientsForSession(sessionID, msg, clients)
}

// BroadcastToUser sends a notification to clients subscribed to a specific user
func (h *Hub) BroadcastToUser(userID string, msg *ws.Message) {
	clients := h.getSubscribersLocked(h.userSubscribers, userID)
	h.logger.Debug("BroadcastToUser",
		zap.String("user_id", userID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.fanoutToClients(msg, clients)
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
	alreadySubscribed := client.sessionSubscriptions[sessionID]
	h.sessionSubscribers[sessionID][client] = true
	client.sessionSubscriptions[sessionID] = true
	h.mu.Unlock()

	// Only bump the lifecycle counter on a fresh subscribe — a redundant
	// session.subscribe (e.g. resubscribe-after-reconnect when state is
	// already there) must not skew the counter and leak a session_seq
	// entry past last-unsubscribe.
	if !alreadySubscribed {
		h.incSessionSubscribers(sessionID)
	}

	h.logger.Debug("Client subscribed to session",
		zap.String("client_id", client.ID),
		zap.String("session_id", sessionID))

	h.recomputeSessionMode(sessionID)
}

// UnsubscribeFromSession unsubscribes a client from session notifications
func (h *Hub) UnsubscribeFromSession(client *Client, sessionID string) {
	h.mu.Lock()
	wasSubscribed := client.sessionSubscriptions[sessionID]
	delete(client.sessionSubscriptions, sessionID)
	if clients, ok := h.sessionSubscribers[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessionSubscribers, sessionID)
		}
	}
	h.mu.Unlock()

	if wasSubscribed {
		h.decSessionSubscribers(sessionID)
	}

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
	clients := h.getSubscribersLocked(h.runSubscribers, runID)
	h.logger.Debug("BroadcastToRun",
		zap.String("run_id", runID),
		zap.String("action", msg.Action),
		zap.Int("subscriber_count", len(clients)))
	h.fanoutToClients(msg, clients)
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

// WsSentEvent is the public shape exposed by GetSentEventsFor — mirrors the
// internal ring buffer entry but lives on Hub so external callers (the E2E
// endpoint, tests) don't have to reach into unexported types.
// SessionSeq and SessionID are non-zero/non-empty for events routed to a
// specific session (BroadcastToSession); zero/empty for connection-wide
// notifications. The E2E ws-sent endpoint exposes a `session_id` filter that
// returns just those entries sorted by SessionSeq ascending — that's the
// authoritative per-session stream a single client should have observed.
type WsSentEvent struct {
	Seq        int64     `json:"seq"`
	SessionSeq int64     `json:"session_seq,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	Type       string    `json:"type"`
	Action     string    `json:"action"`
	SentAt     time.Time `json:"sent_at"`
}

// GetSentEventsFor returns the recorded outbound envelopes for the given
// connection ID with seq > sinceSeq, plus the max seq ever stamped on that
// connection. The bool is false when no such connection is registered.
//
// Used by the E2E /api/v1/e2e/ws-sent endpoint so tests can diff the FE's
// received-seq list against the BE's authoritative send log. Pass sinceSeq=0
// to dump the whole ring buffer (last 5000 events).
func (h *Hub) GetSentEventsFor(connectionID string, sinceSeq int64) ([]WsSentEvent, int64, bool) {
	client, ok := h.getClientByID(connectionID)
	if !ok || client.sentLog == nil {
		return nil, 0, false
	}
	entries := client.sentLog.Since(sinceSeq)
	return entries, client.sentLog.Max(), true
}

// GetSentEventsForSession is GetSentEventsFor narrowed to a single session ID.
// Returns only entries whose stamped SessionID matches, sorted by SessionSeq
// ascending — i.e. the authoritative per-session stream a single subscriber
// should have observed. The second return value is the max SessionSeq stamped
// for this (connection, session) pair.
//
// Used by the E2E /api/v1/e2e/ws-sent endpoint when the `session_id` query
// param is set: per-session gap detection that the per-connection seq cannot
// catch on its own (cross-session misrouting).
func (h *Hub) GetSentEventsForSession(connectionID, sessionID string) ([]WsSentEvent, int64, bool) {
	client, ok := h.getClientByID(connectionID)
	if !ok || client.sentLog == nil {
		return nil, 0, false
	}
	entries := client.sentLog.SinceForSession(0, sessionID)
	var maxSessionSeq int64
	for _, e := range entries {
		if e.SessionSeq > maxSessionSeq {
			maxSessionSeq = e.SessionSeq
		}
	}
	return entries, maxSessionSeq, true
}

// getClientByID returns the registered client with the given ID. Multiple
// connections cannot share an ID under current handler wiring (the ID is
// derived from a server-side random token), so first-match is enough.
func (h *Hub) getClientByID(id string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.ID == id {
			return c, true
		}
	}
	return nil, false
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
