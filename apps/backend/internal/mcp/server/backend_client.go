package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
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
	logger    *logger.Logger
}

// NewChannelBackendClient creates a new channel-based backend client.
func NewChannelBackendClient(log *logger.Logger) *ChannelBackendClient {
	clientLogger := logger.Default()
	if log != nil {
		clientLogger = log
	}
	clientLogger = clientLogger.WithFields(zap.String("component", "mcp-backend-client"))
	return &ChannelBackendClient{
		requestCh: make(chan *ws.Message, 100),
		pending:   make(map[string]chan *ws.Message),
		logger:    clientLogger,
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
		return
	}
	c.logger.Debug("dropping MCP response with no pending request",
		zap.String("request_id", msg.ID),
		zap.String("type", string(msg.Type)),
		zap.String("action", msg.Action))
}

// RequestPayload sends a request to the backend and unmarshals the response.
// The request will be cancelled if the context is cancelled or if Reset() is called.
func (c *ChannelBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	id := uuid.New().String()
	start := time.Now()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Create response channel
	respChan := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	c.logger.Debug("sending MCP request through agent stream",
		zap.String("request_id", id),
		zap.String("action", action),
		zap.Any("payload", payload))

	// Ensure cleanup on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// Send request through channel
	select {
	case c.requestCh <- msg:
		// Request sent
	case <-ctx.Done():
		c.logger.Debug("MCP request cancelled before send",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.Duration("duration", time.Since(start)),
			zap.Error(ctx.Err()))
		return ctx.Err()
	case <-time.After(5 * time.Second):
		c.logger.Warn("timed out sending MCP request to agent stream",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.Duration("duration", time.Since(start)))
		return fmt.Errorf("timeout sending request to agent stream")
	}

	// Wait for response
	select {
	case resp, ok := <-respChan:
		if !ok {
			// Channel was closed by Reset() - session was cancelled/reset
			c.logger.Warn("MCP request cancelled by session reset",
				zap.String("request_id", id),
				zap.String("action", action),
				zap.Duration("duration", time.Since(start)))
			return fmt.Errorf("MCP request cancelled: session reset")
		}
		c.logger.Debug("received MCP response from backend",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.String("type", string(resp.Type)),
			zap.Duration("duration", time.Since(start)))
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
		c.logger.Warn("MCP request context cancelled while waiting for response",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.Duration("duration", time.Since(start)),
			zap.Error(ctx.Err()))
		return ctx.Err()
	}
}

// Reset clears all pending MCP requests.
// This should be called when starting a new ACP session to prevent
// stale requests from a previous session from interfering.
func (c *ChannelBackendClient) Reset() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	// Close all pending response channels to unblock waiting goroutines
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// Close closes the request channel.
func (c *ChannelBackendClient) Close() {
	close(c.requestCh)
}
