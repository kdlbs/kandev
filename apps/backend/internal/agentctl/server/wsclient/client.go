// Package wsclient provides a WebSocket client for connecting back to the Kandev backend.
package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type Client struct {
	url       string
	conn      *websocket.Conn
	logger    *logger.Logger
	pending   map[string]chan *ws.Message
	pendingMu sync.RWMutex
	connected bool
	connMu    sync.RWMutex
	writeMu   sync.Mutex
	reconnectInterval time.Duration
	maxReconnectTries int
}

func New(url string, log *logger.Logger) *Client {
	return &Client{
		url:               url,
		logger:            log.WithFields(zap.String("component", "wsclient")),
		pending:           make(map[string]chan *ws.Message),
		reconnectInterval: 5 * time.Second,
		maxReconnectTries: 10,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.connected {
		return nil
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	c.conn = conn
	c.connected = true
	c.logger.Info("connected to backend WebSocket", zap.String("url", c.url))
	go c.readLoop()
	return nil
}

func (c *Client) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if !c.connected {
		return nil
	}
	c.connected = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connected
}

func (c *Client) Request(ctx context.Context, action string, payload interface{}) (*ws.Message, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to backend")
	}
	id := uuid.New().String()
	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	respChan := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()
	c.writeMu.Lock()
	err = c.conn.WriteJSON(msg)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	c.logger.Debug("sent request", zap.String("action", action), zap.String("id", id))
	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	resp, err := c.Request(ctx, action, payload)
	if err != nil {
		return err
	}
	if resp.Type == ws.MessageTypeError {
		var ep struct{ Code, Message string }
		if json.Unmarshal(resp.Payload, &ep) == nil {
			return fmt.Errorf("backend error [%s]: %s", ep.Code, ep.Message)
		}
		return fmt.Errorf("backend error: %s", string(resp.Payload))
	}
	if result != nil && len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}
	return nil
}

func (c *Client) readLoop() {
	for {
		c.connMu.RLock()
		conn, connected := c.conn, c.connected
		c.connMu.RUnlock()
		if !connected || conn == nil {
			return
		}
		var msg ws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Error("read error", zap.Error(err))
			}
			c.handleDisconnect()
			return
		}
		c.handleMessage(&msg)
	}
}

func (c *Client) handleMessage(msg *ws.Message) {
	if msg.Type == ws.MessageTypeResponse || msg.Type == ws.MessageTypeError {
		c.pendingMu.Lock()
		ch, ok := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.pendingMu.Unlock()
		if ok {
			ch <- msg
		}
		return
	}
}

func (c *Client) handleDisconnect() {
	c.connMu.Lock()
	c.connected = false
	c.conn = nil
	c.connMu.Unlock()
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		errMsg, _ := ws.NewError(id, "", ws.ErrorCodeInternalError, "connection lost", nil)
		ch <- errMsg
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
}

