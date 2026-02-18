package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// sendStreamRequest sends a request over the agent WebSocket stream and waits for a response.
// It creates a ws.Message with a UUID, registers a pending response channel,
// writes the message to the stream, and blocks until a response arrives or context is cancelled.
func (c *Client) sendStreamRequest(ctx context.Context, action string, payload interface{}) (*ws.Message, error) {
	c.mu.RLock()
	conn := c.agentStreamConn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("agent stream not connected")
	}

	reqID := uuid.New().String()
	msg, err := ws.NewRequest(reqID, action, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request message: %w", err)
	}

	// Register pending request
	respCh := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pendingRequests[reqID] = respCh
	c.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingRequests, reqID)
		c.pendingMu.Unlock()
	}()

	// Serialize write to stream
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.streamWriteMu.Lock()
	writeErr := conn.WriteMessage(websocket.TextMessage, data)
	c.streamWriteMu.Unlock()
	if writeErr != nil {
		return nil, fmt.Errorf("failed to write request to stream: %w", writeErr)
	}

	// Wait for response or context cancellation
	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("agent stream disconnected while waiting for response")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// resolvePendingRequest matches a response message to a pending request by ID.
// Returns true if the message was matched to a pending request.
func (c *Client) resolvePendingRequest(msg *ws.Message) bool {
	if msg.ID == "" {
		return false
	}

	c.pendingMu.Lock()
	ch, ok := c.pendingRequests[msg.ID]
	c.pendingMu.Unlock()

	if !ok {
		return false
	}

	// Send response to waiting caller (non-blocking since channel is buffered)
	select {
	case ch <- msg:
	default:
	}
	return true
}

// cleanupPendingRequests unblocks all pending requests with nil (signaling disconnect).
func (c *Client) cleanupPendingRequests() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pendingRequests {
		close(ch)
		delete(c.pendingRequests, id)
	}
}
