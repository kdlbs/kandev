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

type backendResponse struct {
	message *ws.Message
	err     error
}

type pendingRequest struct {
	result   chan backendResponse
	streamID string
}

// ChannelBackendClient implements BackendClient using channels.
// It sends MCP requests through a channel that will be read by the agent stream handler,
// and receives responses through a callback mechanism.
type ChannelBackendClient struct {
	requestCh chan *ws.Message
	pending   map[string]*pendingRequest
	pendingMu sync.Mutex
	done      chan struct{}
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
	publishWG sync.WaitGroup
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
		requestCh: make(chan *ws.Message),
		pending:   make(map[string]*pendingRequest),
		done:      make(chan struct{}),
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
	if c.completeRequest(msg.ID, backendResponse{message: msg}) {
		return
	}
	c.logger.Debug("dropping MCP response with no pending request",
		zap.String("request_id", msg.ID),
		zap.String("type", string(msg.Type)),
		zap.String("action", msg.Action))
}

// BindRequestToStream records which backend stream delivered a request.
func (c *ChannelBackendClient) BindRequestToStream(requestID, streamID string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if pending, ok := c.pending[requestID]; ok {
		pending.streamID = streamID
	}
}

// FailRequest releases one pending request after a transport failure.
func (c *ChannelBackendClient) FailRequest(requestID string, err error) {
	c.completeRequest(requestID, backendResponse{err: err})
}

// FailStreamRequests releases requests delivered by one disconnected stream.
func (c *ChannelBackendClient) FailStreamRequests(streamID string, err error) {
	c.pendingMu.Lock()
	failed := make([]*pendingRequest, 0)
	for id, pending := range c.pending {
		if pending.streamID == streamID {
			delete(c.pending, id)
			failed = append(failed, pending)
		}
	}
	c.pendingMu.Unlock()

	for _, pending := range failed {
		pending.result <- backendResponse{err: err}
	}
}

func (c *ChannelBackendClient) completeRequest(requestID string, response backendResponse) bool {
	c.pendingMu.Lock()
	pending, ok := c.pending[requestID]
	if ok {
		delete(c.pending, requestID)
	}
	c.pendingMu.Unlock()
	if ok {
		pending.result <- response
	}
	return ok
}

// RequestPayload sends a request to the backend and unmarshals the response.
// The request will be cancelled if the context is cancelled or if Reset() is called.
func (c *ChannelBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	if !c.beginPublish() {
		return fmt.Errorf("MCP backend client is closed")
	}
	publishing := true
	defer func() {
		if publishing {
			c.publishWG.Done()
		}
	}()

	id := uuid.New().String()
	start := time.Now()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Create response channel
	respChan := make(chan backendResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = &pendingRequest{result: respChan}
	c.pendingMu.Unlock()

	c.logger.Debug("sending MCP request through agent stream",
		zap.String("request_id", id),
		zap.String("action", action),
		zap.Any("payload", backendPayloadForLog(action, payload)))

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
	case <-c.done:
		return fmt.Errorf("MCP backend client is closed")
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
	publishing = false
	c.publishWG.Done()

	// Wait for response
	select {
	case response := <-respChan:
		if response.err != nil {
			c.logger.Warn("MCP request failed after publication",
				zap.String("request_id", id),
				zap.String("action", action),
				zap.Duration("duration", time.Since(start)),
				zap.Error(response.err))
			return response.err
		}
		resp := response.message
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
	case <-c.done:
		return fmt.Errorf("MCP backend client is closed")
	}
}

func (c *ChannelBackendClient) beginPublish() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return false
	}
	c.publishWG.Add(1)
	return true
}

func backendPayloadForLog(action string, payload interface{}) interface{} {
	if action != ws.ActionMCPInvokePluginTool {
		return payload
	}
	values, ok := payload.(map[string]any)
	if !ok {
		return "<redacted>"
	}
	safe := make(map[string]any, len(values))
	for key, value := range values {
		if key != pluginToolArgumentsKey {
			safe[key] = value
		}
	}
	return safe
}

// Reset clears all pending MCP requests.
// This should be called when starting a new ACP session to prevent
// stale requests from a previous session from interfering.
func (c *ChannelBackendClient) Reset() {
	c.pendingMu.Lock()
	pending := make([]*pendingRequest, 0, len(c.pending))
	for id, request := range c.pending {
		delete(c.pending, id)
		pending = append(pending, request)
	}
	c.pendingMu.Unlock()

	for _, request := range pending {
		request.result <- backendResponse{err: fmt.Errorf("MCP request cancelled: session reset")}
	}
}

// Close prevents new requests and cancels pending requests.
func (c *ChannelBackendClient) Close() {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		close(c.done)
		c.closeMu.Unlock()
	})
	c.publishWG.Wait()
}
