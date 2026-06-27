package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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
)

// Client represents a single WebSocket connection
type Client struct {
	ID                      string
	conn                    *websocket.Conn
	hub                     *Hub
	send                    chan []byte
	subscriptions           map[string]bool // Task IDs this client is subscribed to
	sessionSubscriptions    map[string]bool // Session IDs this client is subscribed to
	sessionFocus            map[string]bool // Session IDs this client has focused (a strict subset of subscriptions, conceptually — see hub_session_mode.go)
	userSubscriptions       map[string]bool // User IDs this client is subscribed to
	runSubscriptions        map[string]bool // Office run IDs this client is subscribed to (for run.event.appended)
	systemMetricsSubscribed bool
	mu                      sync.RWMutex
	closed                  bool
	logger                  *logger.Logger
	connectionSeq           atomic.Int64
	sentLog                 *wsSentLog
}

// NewClient creates a new WebSocket client
func NewClient(id string, conn *websocket.Conn, hub *Hub, log *logger.Logger) *Client {
	return &Client{
		ID:                   id,
		conn:                 conn,
		hub:                  hub,
		send:                 make(chan []byte, 256),
		subscriptions:        make(map[string]bool),
		sessionSubscriptions: make(map[string]bool),
		sessionFocus:         make(map[string]bool),
		userSubscriptions:    make(map[string]bool),
		runSubscriptions:     make(map[string]bool),
		logger:               log.WithFields(zap.String("client_id", id)),
		sentLog:              newWsSentLog(),
	}
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
			// CloseGoingAway (1001): Client navigating away
			// CloseNoStatusReceived (1005): Client closed without status (normal browser close)
			// CloseAbnormalClosure (1006): Abnormal close (network drop)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
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
	dispatchCtx := c.hub.DispatchContext()
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

func (c *Client) handleUserSubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = store.DefaultUserID
	}
	if userID != store.DefaultUserID {
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

	c.hub.SubscribeToSession(c, req.SessionID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)

	// Send initial session data (e.g., git status) if available
	c.sendSessionData(req.SessionID)
}

func (c *Client) handleUserUnsubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	userID := req.UserID
	if userID == "" {
		userID = store.DefaultUserID
	}
	if userID != store.DefaultUserID {
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

	// Replay data is delivered only to this connection; live session broadcasts
	// carry session_seq so multi-client subscribers share one logical sequence.
	for _, msg := range data {
		c.sendMessage(msg)
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
//
// Also pushes a fresh session data snapshot (git status, etc.) because the
// session.subscribe frame is often absorbed by the sidebar's bulk subscribe
// (ref-counted on the client) before the task-page hook can ask for it — so
// without this, switching tasks leaves the Changes panel showing stale/empty
// state until the next poll broadcast arrives.
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
	c.hub.FocusSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)

	c.sendSessionData(req.SessionID)
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

// sendMessage stamps and sends a connection-wide message to the client.
func (c *Client) sendMessage(msg *ws.Message) bool {
	return c.sendMessageForSession("", msg)
}

func (c *Client) sendMessageForSession(sessionID string, msg *ws.Message) bool {
	return c.sendMessageForSessionSeq(sessionID, 0, msg)
}

func (c *Client) sendMessageForSessionSeq(sessionID string, sessionSeq int64, msg *ws.Message) bool {
	stamped, ok := c.stampAndMarshalForSession(sessionID, sessionSeq, msg)
	if !ok {
		return false
	}
	if !c.sendStampedMessage(&stamped) {
		return false
	}
	if c.sentLog != nil {
		c.sentLog.Append(
			stamped.connectionSeq,
			stamped.sessionSeq,
			stamped.sessionID,
			stamped.msgType,
			stamped.action,
			stamped.sentAt,
		)
	}
	return true
}

func (c *Client) sendStampedCopyForSessionSeq(sessionID string, sessionSeq int64, msg *ws.Message) bool {
	if msg == nil {
		return false
	}
	clone := *msg
	return c.sendMessageForSessionSeq(sessionID, sessionSeq, &clone)
}

type stampedMessage struct {
	data          []byte
	message       ws.Message
	connectionSeq int64
	sessionSeq    int64
	sessionID     string
	msgType       string
	action        string
	sentAt        time.Time
}

func (c *Client) stampAndMarshalForSession(
	sessionID string,
	sessionSeq int64,
	msg *ws.Message,
) (stampedMessage, bool) {
	if msg == nil {
		return stampedMessage{}, false
	}
	stampedMsg := *msg
	stampedMsg.ConnectionID = c.ID
	stampedMsg.ConnectionSeq = 0
	stampedMsg.SessionSeq = 0
	if sessionID != "" && c.hub != nil {
		if sessionSeq <= 0 {
			sessionSeq = c.hub.nextSessionSeq(sessionID)
		}
		stampedMsg.SessionSeq = sessionSeq
	}
	data, err := json.Marshal(&stampedMsg)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return stampedMessage{}, false
	}
	return stampedMessage{
		data:       data,
		message:    stampedMsg,
		sessionSeq: stampedMsg.SessionSeq,
		sessionID:  sessionID,
		msgType:    string(stampedMsg.Type),
		action:     stampedMsg.Action,
	}, true
}

func (c *Client) sendStampedMessage(stamped *stampedMessage) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if len(c.send) >= cap(c.send) {
		c.logger.Warn("Client send buffer full")
		return false
	}
	connectionSeq := c.connectionSeq.Add(1)
	stamped.message.ConnectionSeq = connectionSeq
	data, err := json.Marshal(&stamped.message)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return false
	}
	c.send <- data
	stamped.data = data
	stamped.connectionSeq = connectionSeq
	stamped.sentAt = time.Now().UTC()
	return true
}

func (c *Client) sendBytes(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}

	select {
	case c.send <- data:
		return true
	default:
		c.logger.Warn("Client send buffer full")
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
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
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

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Debug("failed to set write deadline", zap.Error(err))
			}
			if !ok {
				// Hub closed the channel
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					c.logger.Debug("failed to write close message", zap.Error(err))
				}
				return
			}

			// Send each message in its own WebSocket frame
			// (previously batched with newlines, but clients expect one JSON object per frame)
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.logger.Debug("failed to write websocket message", zap.Error(err))
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Debug("failed to set write deadline", zap.Error(err))
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
