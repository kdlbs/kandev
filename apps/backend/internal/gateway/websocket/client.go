package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
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
	ID            string
	conn          *websocket.Conn
	hub           *Hub
	send          chan []byte
	subscriptions map[string]bool // Task IDs this client is subscribed to
	mu            sync.RWMutex
	logger        *logger.Logger
}

// NewClient creates a new WebSocket client
func NewClient(id string, conn *websocket.Conn, hub *Hub, log *logger.Logger) *Client {
	return &Client{
		ID:            id,
		conn:          conn,
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
		logger:        log.WithFields(zap.String("client_id", id)),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
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

		// Process the message
		c.handleMessage(ctx, &msg)
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

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)
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

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}

	select {
	case c.send <- data:
	default:
		c.logger.Warn("Client send buffer full")
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

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Batch additional queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}