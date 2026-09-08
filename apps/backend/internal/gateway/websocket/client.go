package websocket

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	// Increased to support image attachments (base64 encoded images are ~33% larger)
	maxMessageSize = 32 * 1024 * 1024 // 32MB

	// Control frames (RPC responses and errors) use a separate bounded queue so
	// high-volume session notifications cannot fill the queue and make a user
	// action appear to time out.
	controlSendBufferSize = 256
)

// Client represents a single WebSocket connection
type Client struct {
	ID string
	// identity is the authenticated caller behind this connection. Zero for
	// anonymous connections (auth disabled and no synthetic identity set by
	// the HTTP middleware — e.g. direct hub tests); synthetic in disabled
	// mode; a real user when auth is enabled.
	identity                authn.Identity
	conn                    *websocket.Conn
	hub                     *Hub
	send                    chan []byte
	controlSend             chan []byte
	subscriptions           map[string]bool // Task IDs this client is subscribed to
	sessionSubscriptions    map[string]bool // Session IDs this client is subscribed to
	sessionFocus            map[string]bool // Session IDs this client has focused (a strict subset of subscriptions, conceptually — see hub_session_mode.go)
	userSubscriptions       map[string]bool // User IDs this client is subscribed to
	runSubscriptions        map[string]bool // Office run IDs this client is subscribed to (for run.event.appended)
	systemMetricsSubscribed bool
	mu                      sync.RWMutex
	closed                  bool
	logger                  *logger.Logger

	// Replaceable session.message.updated traffic is scheduled separately from
	// semantic notifications so one noisy session cannot fill the shared FIFO.
	// Each session queue contains replaceable segments separated by semantic
	// barriers. This prevents an update published after a barrier from overtaking
	// that barrier while still allowing coalescing within the current segment.
	replaceableByKey           map[queuedReplaceableKey]outboundNotification
	replaceableBySession       map[string][]sessionNotificationQueueItem
	replaceableCurrentByKey    map[replaceableNotificationKey]queuedReplaceableKey
	replaceableSessionOrder    []string
	replaceableRoundRobin      int
	nextReplaceableSequence    uint64
	scheduledSemantic          int
	notificationWake           chan struct{}
	replaceableReplacements    uint64
	replaceableEvictions       uint64
	replaceableRejected        uint64
	droppedSemantic            uint64
	replaceablePerSessionLimit int
	replaceableGlobalLimit     int
}

// NewClient creates a new WebSocket client
func NewClient(id string, identity authn.Identity, conn *websocket.Conn, hub *Hub, log *logger.Logger) *Client {
	return &Client{
		ID:                      id,
		identity:                identity,
		conn:                    conn,
		hub:                     hub,
		send:                    make(chan []byte, 256),
		controlSend:             make(chan []byte, controlSendBufferSize),
		subscriptions:           make(map[string]bool),
		sessionSubscriptions:    make(map[string]bool),
		sessionFocus:            make(map[string]bool),
		userSubscriptions:       make(map[string]bool),
		runSubscriptions:        make(map[string]bool),
		replaceableByKey:        make(map[queuedReplaceableKey]outboundNotification),
		replaceableBySession:    make(map[string][]sessionNotificationQueueItem),
		replaceableCurrentByKey: make(map[replaceableNotificationKey]queuedReplaceableKey),
		notificationWake:        make(chan struct{}, 1),
		logger:                  log.WithFields(zap.String("client_id", id)),
	}
}

// dispatchContext returns the hub's lifetime context carrying this client's
// identity, so dispatched RPC handlers (workspace.list, task CRUD, …) and
// subscription checks apply the same per-user scoping as HTTP requests.
func (c *Client) dispatchContext() context.Context {
	ctx := c.hub.DispatchContext()
	if c.identity.UserID != "" {
		ctx = authn.WithIdentity(ctx, c.identity)
	}
	return ctx
}

// ReadPump pumps messages from the WebSocket connection to the hub.
//
// The ctx argument is retained for API stability but is not consulted by the
// pump itself — Gorilla's ReadMessage blocks on the conn only, so teardown
// happens via the conn close path (driven by client disconnect, server
// shutdown closing all conns, or pong timeout). Dispatched handlers use the
// hub's lifetime context instead; see handleMessage.
func (c *Client) ReadPump(_ context.Context) {
	defer func() {
		c.hub.Unregister(c)
		if err := c.conn.Close(); err != nil {
			c.logger.Debug("failed to close websocket connection", zap.Error(err))
		}
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.logger.Debug("failed to set read deadline", zap.Error(err))
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// CloseNormalClosure (1000): Normal browser close
			// CloseGoingAway (1001): Client navigating away
			// CloseNoStatusReceived (1005): Client closed without status (normal browser close)
			// CloseAbnormalClosure (1006): Abnormal close (network drop)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket read error", zap.Error(err))
			}
			break
		}

		// Parse the message
		var msg ws.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.Error("Failed to parse message", zap.Error(err))
			c.sendError("", "", ws.ErrorCodeBadRequest, "Invalid message format", nil)
			continue
		}

		// Process the message in a goroutine to avoid blocking the read pump
		// This allows concurrent message handling so long-running handlers
		// (like orchestrator.prompt) don't block other requests (like workspace.tree.get)
		go c.handleMessage(&msg)
	}
}

// handleMessage processes an incoming message.
//
// Intentionally does NOT take the connection context. Dispatched handlers run
// under the hub's lifetime context so a mid-flight client disconnect doesn't
// abort in-progress side effects (see the comment on Dispatch below).
func (c *Client) handleMessage(msg *ws.Message) {
	c.logger.Debug("Received message",
		zap.String("action", msg.Action),
		zap.String("id", msg.ID))

	if strings.HasPrefix(msg.Action, "mcp.") {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"MCP actions are not available over the raw WebSocket", nil)
		return
	}

	// Handle subscription actions specially (they need access to the client)
	switch msg.Action {
	case ws.ActionTaskSubscribe:
		c.handleSubscribe(msg)
		return
	case ws.ActionTaskUnsubscribe:
		c.handleUnsubscribe(msg)
		return
	case ws.ActionSessionSubscribe:
		c.handleSessionSubscribe(msg)
		return
	case ws.ActionSessionUnsubscribe:
		c.handleSessionUnsubscribe(msg)
		return
	case ws.ActionSessionFocus:
		c.handleSessionFocus(msg)
		return
	case ws.ActionSessionGitRefresh:
		c.handleSessionGitRefresh(msg)
		return
	case ws.ActionSessionDataRefresh:
		c.handleSessionDataRefresh(msg)
		return
	case ws.ActionSessionUnfocus:
		c.handleSessionUnfocus(msg)
		return
	case ws.ActionUserSubscribe:
		c.handleUserSubscribe(msg)
		return
	case ws.ActionUserUnsubscribe:
		c.handleUserUnsubscribe(msg)
		return
	case ws.ActionRunSubscribe:
		c.handleRunSubscribe(msg)
		return
	case ws.ActionRunUnsubscribe:
		c.handleRunUnsubscribe(msg)
		return
	case ws.ActionSystemMetricsSubscribe:
		c.handleSystemMetricsSubscribe(msg)
		return
	case ws.ActionSystemMetricsUnsubscribe:
		c.handleSystemMetricsUnsubscribe(msg)
		return
	}

	// Dispatch to handler using the hub's lifetime context, not the per-
	// connection one. The connection ctx is cancelled when the client
	// disconnects (page reload, nav, network drop). Using it here would
	// SIGKILL any exec.CommandContext subprocesses the handler spawned
	// (e.g. `gh pr`, `git`, agentctl HTTP requests) and abort
	// side-effecting work like session.launch mid-flight. We can't deliver
	// the response either way once the connection is gone, but the
	// handler's work should run to completion so it doesn't leave partial
	// state behind. The dispatch ctx still cancels on server shutdown.
	dispatchCtx := c.dispatchContext()

	// Per-user scoping backstop: refuse before the handler runs if the payload
	// names a task or session this client may not touch. See dispatch_scope.go
	// for why this lives here and not only in each handler.
	if err := c.authorizeAction(dispatchCtx, msg.Action, msg.Payload); err != nil {
		c.logger.Debug("denied out-of-scope action",
			zap.String("action", msg.Action),
			zap.String("user_id", c.identity.UserID),
			zap.Error(err))
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "not found", nil)
		return
	}

	response, err := c.hub.dispatcher.Dispatch(dispatchCtx, msg)
	if err != nil {
		c.logger.Error("Handler error",
			zap.String("action", msg.Action),
			zap.Error(err))
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return
	}

	if response != nil {
		c.sendMessage(response)
	}
}

// SubscribeRequest is the payload for task.subscribe
type SubscribeRequest struct {
	TaskID string `json:"task_id"`
}

// handleSubscribe handles task.subscribe action
func (c *Client) handleSubscribe(msg *ws.Message) {
	var req SubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.TaskID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		return
	}

	// Per-user scoping: a client may only observe tasks in its own workspaces.
	if check := c.hub.authPolicy.Subscriptions.Task; check != nil {
		if err := check(c.dispatchContext(), req.TaskID); err != nil {
			c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this task", nil)
			return
		}
	}

	c.hub.SubscribeToTask(c, req.TaskID)

	// Send success response
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)
}

type UserSubscribeRequest struct {
	UserID string `json:"user_id,omitempty"`
}

type SessionSubscribeRequest struct {
	SessionID string `json:"session_id"`
}

// ownUserTopic resolves the user-topic this client may subscribe to: its own
// authenticated user, or the pre-auth default user for anonymous/synthetic
// connections (today's single-user behavior).
func (c *Client) ownUserTopic() string {
	if c.identity.UserID != "" && !c.identity.Synthetic {
		return c.identity.UserID
	}
	return store.DefaultUserID
}

func (c *Client) handleUserSubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = c.ownUserTopic()
	}
	if userID != c.ownUserTopic() {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to another user", nil)
		return
	}

	c.hub.SubscribeToUser(c, userID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"user_id": userID,
	})
	c.sendMessage(resp)
}

func (c *Client) handleSessionSubscribe(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}

	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	newMembership := c.hub.SubscribeToSession(c, req.SessionID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)

	// Send initial session data only when this client newly joins. Duplicate
	// subscribe requests are acknowledgements, not snapshot replay commands.
	if newMembership {
		c.sendSessionData(req.SessionID)
	}
}

// maySubscribeSession applies the per-user session scoping check, emitting the
// forbidden error itself. Returns true when the subscription may proceed.
func (c *Client) maySubscribeSession(msg *ws.Message, sessionID string) bool {
	check := c.hub.authPolicy.Subscriptions.Session
	if check == nil {
		return true
	}
	if err := check(c.dispatchContext(), sessionID); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this session", nil)
		return false
	}
	return true
}

func (c *Client) handleUserUnsubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	userID := req.UserID
	if userID == "" {
		userID = c.ownUserTopic()
	}
	if userID != c.ownUserTopic() {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot unsubscribe from another user", nil)
		return
	}
	c.hub.UnsubscribeFromUser(c, userID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"user_id": userID,
	})
	c.sendMessage(resp)
}

// sendSessionData sends initial session data (e.g., git status) to the client
func (c *Client) sendSessionData(sessionID string) {
	ctx := context.Background()
	data, err := c.hub.GetSessionData(ctx, sessionID)
	if err != nil {
		c.logger.Error("Failed to get session data",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	if len(data) == 0 {
		return
	}

	c.logger.Debug("Sending session data",
		zap.String("session_id", sessionID),
		zap.Int("count", len(data)))

	// Send each piece of session data as a notification. Snapshots are data
	// traffic, not control traffic, so they must not consume the response queue.
	for _, msg := range data {
		payload, err := json.Marshal(msg)
		if err != nil {
			c.logger.Error("Failed to marshal session data", zap.Error(err))
			continue
		}
		c.sendBytes(payload)
	}
}

// sendSessionGitData sends the bounded git snapshot used by the diff detail
// surface. The compatibility fallback may return the full legacy provider;
// filter it at the transport boundary so this action remains git-only.
func (c *Client) sendSessionGitData(sessionID string) {
	ctx := context.Background()
	data, err := c.hub.GetSessionGitData(ctx, sessionID)
	if err != nil {
		c.logger.Error("Failed to get session git data",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	for _, msg := range data {
		if msg == nil || msg.Action != ws.ActionSessionGitEvent {
			continue
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			c.logger.Error("Failed to marshal session git data", zap.Error(err))
			continue
		}
		c.sendBytes(payload)
	}
}

// handleUnsubscribe handles task.unsubscribe action
func (c *Client) handleUnsubscribe(msg *ws.Message) {
	var req SubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.TaskID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		return
	}

	c.hub.UnsubscribeFromTask(c, req.TaskID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)
}

// RunSubscribeRequest is the payload for run.subscribe / run.unsubscribe.
type RunSubscribeRequest struct {
	RunID string `json:"run_id"`
}

// handleRunSubscribe handles run.subscribe action — registers this
// client on the per-run topic so it receives run.event.appended
// notifications. Clients fetch the snapshot via REST and only need
// the diff stream from this point forward; we deliberately replay no
// state on subscribe.
func (c *Client) handleRunSubscribe(msg *ws.Message) {
	var req RunSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.RunID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "run_id is required", nil)
		return
	}

	// Per-user scoping: a client may only observe runs in its own
	// workspaces. Every hook error maps to the same fixed (code, message)
	// pair below so an unknown run id and a foreign run id are
	// indistinguishable on the wire (no existence leak).
	if check := c.hub.authPolicy.Subscriptions.Run; check != nil {
		if err := check(c.dispatchContext(), req.RunID); err != nil {
			c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this run", nil)
			return
		}
	}

	c.hub.SubscribeToRun(c, req.RunID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"run_id":  req.RunID,
	})
	c.sendMessage(resp)
}

// handleRunUnsubscribe handles run.unsubscribe action.
func (c *Client) handleRunUnsubscribe(msg *ws.Message) {
	var req RunSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.RunID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "run_id is required", nil)
		return
	}
	c.hub.UnsubscribeFromRun(c, req.RunID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"run_id":  req.RunID,
	})
	c.sendMessage(resp)
}

func (c *Client) handleSystemMetricsSubscribe(msg *ws.Message) {
	c.hub.SubscribeToSystemMetrics(c)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
	c.sendMessage(resp)
}

func (c *Client) handleSystemMetricsUnsubscribe(msg *ws.Message) {
	c.hub.UnsubscribeFromSystemMetrics(c)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
	c.sendMessage(resp)
}

// handleSessionUnsubscribe handles session.unsubscribe action
func (c *Client) handleSessionUnsubscribe(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}

	c.hub.UnsubscribeFromSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

// handleSessionFocus handles session.focus — marks the session as actively
// viewed by this client, lifting backend polling to fast mode for the workspace.
// It intentionally sends only the control acknowledgement. Snapshot/data
// refreshes belong to explicit detail-surface requests and must not be replayed
// on every focus transition.
func (c *Client) handleSessionFocus(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}
	c.hub.FocusSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

// handleSessionDataRefresh explicitly requests a fresh detail snapshot without
// changing subscription or focus state. Keeping this separate from focus
// makes repeated tab activation cheap and keeps focus acknowledgements small.
func (c *Client) handleSessionDataRefresh(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
	c.sendSessionData(req.SessionID)
}

// handleSessionGitRefresh explicitly requests only a fresh git-status
// snapshot for an already-focused session. Unlike the legacy generic data
// refresh, this does not replay session state, models, commands, or other
// detail-independent data.
func (c *Client) handleSessionGitRefresh(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
	c.sendSessionGitData(req.SessionID)
}

// handleSessionUnfocus handles session.unfocus — releases the focus mark for
// this client. The session falls back to slow mode (still subscribed) or
// paused (no subscribers), with a debounce to absorb tab churn.
func (c *Client) handleSessionUnfocus(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	c.hub.UnfocusSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	c.sendControlBytes(data)
}

func (c *Client) sendBytes(data []byte) bool {
	return c.enqueueNotification(newOutboundNotification(data, ""))
}

// sendNotification queues a pre-marshalled notification with its typed action
// so the client scheduler can classify it once without reparsing the envelope.
func (c *Client) sendNotification(data []byte, action string) bool {
	return c.sendNotificationFrame(newOutboundNotification(data, action))
}

func (c *Client) sendNotificationFrame(frame outboundNotification) bool {
	return c.enqueueNotification(frame)
}

// sendControlBytes queues a response/error separately from notifications.
// Keeping this queue independent is enough to prevent a burst of stream
// frames from dropping the ACK for a user action. It remains bounded so a
// stalled peer cannot grow memory without limit.
func (c *Client) sendControlBytes(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if c.controlSend == nil {
		// Keep zero-value/in-package test clients safe while all production
		// clients use the dedicated control queue.
		select {
		case c.send <- data:
			return true
		default:
			if c.logger != nil {
				c.logger.Warn("Client control send buffer full; closing connection")
			}
			c.closeSendLocked()
			return false
		}
	}
	select {
	case c.controlSend <- data:
		return true
	default:
		if c.logger != nil {
			c.logger.Warn("Client control send buffer full; closing connection")
		}
		c.closeSendLocked()
		return false
	}
}

// sendError sends an error message to the client
func (c *Client) sendError(id, action, code, message string, details map[string]interface{}) {
	msg, err := ws.NewError(id, action, code, message, details)
	if err != nil {
		c.logger.Error("Failed to create error message", zap.Error(err))
		return
	}
	c.sendMessage(msg)
}

func (c *Client) closeSend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeSendLocked()
}

func (c *Client) closeSendLocked() {
	if c.closed {
		return
	}
	c.closed = true
	if c.send != nil {
		close(c.send)
	}
	if c.controlSend != nil {
		close(c.controlSend)
	}
	if c.notificationWake != nil {
		// The wake channel is deliberately left open: enqueueNotification checks
		// closed while holding c.mu, so closing it here would create a
		// send-on-closed race with a concurrent producer.
		select {
		case c.notificationWake <- struct{}{}:
		default:
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if err := c.conn.Close(); err != nil {
			c.logger.Debug("failed to close websocket connection", zap.Error(err))
		}
	}()

	controlCh := c.controlSend
	semanticCh := c.send
	semanticSinceReplaceable := 0

	writeMessage := func(message []byte) bool {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			c.logger.Debug("failed to write websocket message", zap.Error(err))
			return false
		}
		return true
	}
	writeClose := func() {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
			c.logger.Debug("failed to write close message", zap.Error(err))
		}
	}
	writePing := func() bool {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			c.logger.Debug("failed to write websocket ping", zap.Error(err))
			return false
		}
		return true
	}

	for {
		// Correlated responses/errors always run first. A bounded semantic burst
		// then gives each active session's replaceable queue a turn.
		select {
		case <-ticker.C:
			if !writePing() {
				return
			}
		default:
		}
		if controlCh != nil {
			select {
			case message, ok := <-controlCh:
				if !ok {
					controlCh = nil
					continue
				}
				if !writeMessage(message) {
					return
				}
				continue
			default:
			}
		}

		if semanticSinceReplaceable >= semanticPriorityBurst {
			if frame, ok := c.popNextReplaceable(); ok {
				semanticSinceReplaceable = 0
				if !writeMessage(frame.data) {
					return
				}
				continue
			}
		}

		if semanticCh != nil {
			select {
			case message, ok := <-semanticCh:
				if !ok {
					semanticCh = nil
					continue
				}
				semanticSinceReplaceable++
				if !writeMessage(message) {
					return
				}
				continue
			default:
			}
		}

		if frame, ok := c.popNextReplaceable(); ok {
			semanticSinceReplaceable = 0
			if !writeMessage(frame.data) {
				return
			}
			continue
		}

		if controlCh == nil && semanticCh == nil && !c.hasReplaceable() {
			writeClose()
			return
		}

		c.mu.RLock()
		wakeCh := c.notificationWake
		c.mu.RUnlock()
		select {
		case message, ok := <-controlCh:
			if !ok {
				controlCh = nil
				continue
			}
			if !writeMessage(message) {
				return
			}
		case message, ok := <-semanticCh:
			if !ok {
				semanticCh = nil
				continue
			}
			semanticSinceReplaceable++
			if !writeMessage(message) {
				return
			}
		case <-wakeCh:
		case <-ticker.C:
			if !writePing() {
				return
			}
		}
	}
}
