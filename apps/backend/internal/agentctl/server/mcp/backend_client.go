package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// MCPRequest represents an MCP request to be sent to the backend.
type MCPRequest struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// MCPResponse represents an MCP response from the backend.
type MCPResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"` // "response" or "error"
	Payload json.RawMessage `json:"payload"`
}

// ChannelBackendClient implements BackendClient using channels.
// It sends MCP requests through a channel that will be read by the agent stream handler,
// and receives responses through a callback mechanism.
type ChannelBackendClient struct {
	requestCh chan *ws.Message
	pending   map[string]chan *ws.Message
	pendingMu sync.Mutex
}

// NewChannelBackendClient creates a new channel-based backend client.
func NewChannelBackendClient() *ChannelBackendClient {
	return &ChannelBackendClient{
		requestCh: make(chan *ws.Message, 100),
		pending:   make(map[string]chan *ws.Message),
	}
}

// GetRequestChannel returns the channel for outgoing MCP requests.
// The agent stream handler should read from this channel and forward to the backend.
func (c *ChannelBackendClient) GetRequestChannel() <-chan *ws.Message {
	return c.requestCh
}

// HandleResponse handles an incoming MCP response from the backend.
// This should be called by the agent stream handler when it receives a response.
func (c *ChannelBackendClient) HandleResponse(msg *ws.Message) {
	c.pendingMu.Lock()
	ch, ok := c.pending[msg.ID]
	delete(c.pending, msg.ID)
	c.pendingMu.Unlock()

	if ok {
		ch <- msg
	}
}

// RequestPayload sends a request to the backend and unmarshals the response.
func (c *ChannelBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	id := uuid.New().String()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Create response channel
	respChan := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	// Send request through channel
	select {
	case c.requestCh <- msg:
		// Request sent
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return ctx.Err()
	case <-time.After(5 * time.Second):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return fmt.Errorf("timeout sending request to agent stream")
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if resp.Type == ws.MessageTypeError {
			var ep struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
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
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return ctx.Err()
	}
}

// Close closes the request channel.
func (c *ChannelBackendClient) Close() {
	close(c.requestCh)
}
