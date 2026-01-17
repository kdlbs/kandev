package websocket

import (
	"context"
	"encoding/json"
	"sync"
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
	maxMessageSize = 512 * 1024 // 512KB
)

// Client represents a single WebSocket connection
type Client struct {
	ID                   string
	conn                 *websocket.Conn
	hub                  *Hub
	send                 chan []byte
	subscriptions        map[string]bool // Task IDs this client is subscribed to
	sessionSubscriptions map[string]bool // Session IDs this client is subscribed to
	userSubscriptions    map[string]bool // User IDs this client is subscribed to
	mu                   sync.RWMutex
	closed               bool
	logger               *logger.Logger
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
		userSubscriptions:    make(map[string]bool),
		logger:               log.WithFields(zap.String("client_id", id)),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump(ctx context.Context) {
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
		go c.handleMessage(ctx, &msg)
	}
}

// handleMessage processes an incoming message
func (c *Client) handleMessage(ctx context.Context, msg *ws.Message) {
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
	case ws.ActionUserSubscribe:
		c.handleUserSubscribe(msg)
		return
	case ws.ActionUserUnsubscribe:
		c.handleUserUnsubscribe(msg)
		return
	}

	// Dispatch to handler
	response, err := c.hub.dispatcher.Dispatch(ctx, msg)
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

	// Send success response first
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)

	// Send historical logs if available (now includes pending permission request messages)
	c.sendHistoricalLogs(req.TaskID)
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

// sendHistoricalLogs sends historical execution logs to the client
func (c *Client) sendHistoricalLogs(taskID string) {
	ctx := context.Background()
	logs, err := c.hub.GetHistoricalLogs(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to get historical logs",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	if len(logs) == 0 {
		return
	}

	c.logger.Debug("Sending historical logs",
		zap.String("task_id", taskID),
		zap.Int("count", len(logs)))

	// Send each historical log as a notification
	for _, log := range logs {
		c.sendMessage(log)
	}
}

// sendPendingPermissions is deprecated. Pending permissions are now stored as messages
// and sent via sendHistoricalLogs. This function is kept for reference but is unused.
// func (c *Client) sendPendingPermissions(taskID string) {}

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

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	c.sendBytes(data)
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

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				c.logger.Debug("failed to write websocket message", zap.Error(err))
				_ = w.Close()
				return
			}

			// Batch additional queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write([]byte{'\n'}); err != nil {
					c.logger.Debug("failed to write websocket delimiter", zap.Error(err))
					_ = w.Close()
					return
				}
				if _, err := w.Write(<-c.send); err != nil {
					c.logger.Debug("failed to write queued websocket message", zap.Error(err))
					_ = w.Close()
					return
				}
			}

			if err := w.Close(); err != nil {
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
