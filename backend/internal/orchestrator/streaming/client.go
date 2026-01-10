package streaming

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024 * 1024 // 1MB
)

// SubscriptionMessage is sent by clients to subscribe/unsubscribe
type SubscriptionMessage struct {
	Action  string   `json:"action"`   // subscribe, unsubscribe
	TaskIDs []string `json:"task_ids"`
}

// ReadPump reads messages from the WebSocket connection
func (c *Client) ReadPump() {
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
				c.logger.Warn("WebSocket read error", zap.Error(err))
			}
			break
		}

		var subMsg SubscriptionMessage
		if err := json.Unmarshal(message, &subMsg); err != nil {
			c.logger.Warn("Invalid subscription message", zap.Error(err))
			continue
		}

		switch subMsg.Action {
		case "subscribe":
			for _, taskID := range subMsg.TaskIDs {
				c.Subscribe(taskID)
			}
		case "unsubscribe":
			for _, taskID := range subMsg.TaskIDs {
				c.Unsubscribe(taskID)
			}
		default:
			c.logger.Warn("Unknown action", zap.String("action", subMsg.Action))
		}
	}
}

// WritePump writes messages to the WebSocket connection
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

			// Add queued messages to the current websocket message
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

// Send sends a message to the client
func (c *Client) Send(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// Close closes the client connection
func (c *Client) Close() {
	c.hub.Unregister(c)
}

// Subscribe subscribes the client to a task
func (c *Client) Subscribe(taskID string) {
	c.mu.Lock()
	c.taskIDs[taskID] = true
	c.mu.Unlock()
	c.hub.SubscribeClient(c, taskID)
	c.logger.Debug("Subscribed to task", zap.String("task_id", taskID))
}

// Unsubscribe unsubscribes the client from a task
func (c *Client) Unsubscribe(taskID string) {
	c.mu.Lock()
	delete(c.taskIDs, taskID)
	c.mu.Unlock()
	c.hub.UnsubscribeClient(c, taskID)
	c.logger.Debug("Unsubscribed from task", zap.String("task_id", taskID))
}

// IsSubscribed returns true if the client is subscribed to a task
func (c *Client) IsSubscribed(taskID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.taskIDs[taskID]
}

