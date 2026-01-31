package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// RequestHandler handles incoming control requests from Claude Code CLI.
// It receives the request ID and control request, and should call SendControlResponse.
type RequestHandler func(requestID string, req *ControlRequest)

// MessageHandler handles streaming messages from Claude Code CLI.
type MessageHandler func(msg *CLIMessage)

// pendingRequest tracks a control request waiting for a response.
type pendingRequest struct {
	ch chan *IncomingControlResponse
}

// Client handles Claude Code CLI communication over stdin/stdout streams.
// It reads streaming JSON from stdout and writes control messages to stdin.
type Client struct {
	stdin  io.Writer
	stdout io.Reader
	logger *logger.Logger

	// Handlers for incoming messages
	requestHandler RequestHandler
	messageHandler MessageHandler

	// Pending control requests (requests we sent, waiting for responses)
	pendingRequests   map[string]*pendingRequest
	pendingRequestsMu sync.Mutex

	// Synchronization
	mu   sync.RWMutex
	done chan struct{}
}

// NewClient creates a new Claude Code CLI client.
func NewClient(stdin io.Writer, stdout io.Reader, log *logger.Logger) *Client {
	return &Client{
		stdin:           stdin,
		stdout:          stdout,
		logger:          log.WithFields(zap.String("component", "claudecode-client")),
		done:            make(chan struct{}),
		pendingRequests: make(map[string]*pendingRequest),
	}
}

// SetRequestHandler sets the handler for incoming control requests.
func (c *Client) SetRequestHandler(handler RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestHandler = handler
}

// SetMessageHandler sets the handler for streaming messages.
func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageHandler = handler
}

// Start begins reading from stdout in a goroutine.
// Returns a channel that is closed when the read loop is ready.
func (c *Client) Start(ctx context.Context) <-chan struct{} {
	ready := make(chan struct{})
	go c.readLoop(ctx, ready)
	return ready
}

// Stop stops the client and closes the done channel.
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		// Already closed
	default:
		close(c.done)
	}
}

// Initialize sends the initialize control request to Claude Code CLI and waits for the response.
// This must be called in streaming mode (input-format=stream-json) to get slash commands.
func (c *Client) Initialize(ctx context.Context, timeout time.Duration) (*InitializeResponseData, error) {
	requestID := uuid.New().String()

	// Create pending request channel
	pending := &pendingRequest{
		ch: make(chan *IncomingControlResponse, 1),
	}

	c.pendingRequestsMu.Lock()
	c.pendingRequests[requestID] = pending
	c.pendingRequestsMu.Unlock()

	defer func() {
		c.pendingRequestsMu.Lock()
		delete(c.pendingRequests, requestID)
		c.pendingRequestsMu.Unlock()
	}()

	// Send initialize control request
	req := &SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: requestID,
		Request: SDKControlRequestBody{
			Subtype: SubtypeInitialize,
			Hooks:   nil, // We don't use SDK hooks
		},
	}

	c.logger.Info("sending initialize control request", zap.String("request_id", requestID))

	if err := c.send(req); err != nil {
		return nil, fmt.Errorf("failed to send initialize request: %w", err)
	}

	// Wait for response with timeout
	c.logger.Info("waiting for initialize response", zap.Duration("timeout", timeout))
	select {
	case <-ctx.Done():
		c.logger.Warn("initialize cancelled by context")
		return nil, ctx.Err()
	case <-time.After(timeout):
		c.logger.Warn("initialize request timed out", zap.Duration("timeout", timeout))
		return nil, fmt.Errorf("initialize request timed out after %v", timeout)
	case resp := <-pending.ch:
		if resp.Subtype == "error" {
			return nil, fmt.Errorf("initialize failed: %s", resp.Error)
		}
		c.logger.Info("initialize response received",
			zap.Int("commands", len(resp.Response.Commands)),
			zap.Int("agents", len(resp.Response.Agents)))
		return resp.Response, nil
	}
}

// SendControlRequest sends a control request to Claude Code CLI.
func (c *Client) SendControlRequest(req *SDKControlRequest) error {
	return c.send(req)
}

// SendControlResponse sends a control response to Claude Code CLI.
func (c *Client) SendControlResponse(resp *ControlResponseMessage) error {
	return c.send(resp)
}

// SendUserMessage sends a user message (prompt) to Claude Code CLI.
func (c *Client) SendUserMessage(content string) error {
	msg := &UserMessage{
		Type: MessageTypeUser,
		Message: UserMessageBody{
			Role:    "user",
			Content: content,
		},
	}
	return c.send(msg)
}

func (c *Client) send(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	c.logger.Debug("claudecode: sent message", zap.String("data", string(data)))
	return nil
}

func (c *Client) readLoop(ctx context.Context, ready chan<- struct{}) {
	scanner := bufio.NewScanner(c.stdout)
	// Allow for large JSON messages (up to 10MB)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	// Signal that we're ready to read
	c.logger.Info("claudecode: read loop starting")
	close(ready)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		c.handleLine(line)
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error("read loop error", zap.Error(err))
	}
}

func (c *Client) handleLine(line []byte) {
	// Log all incoming lines for debugging
	c.logger.Debug("claudecode: received raw line",
		zap.String("line", string(line)))

	// First, parse the basic structure to determine message type
	var msg CLIMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.logger.Warn("failed to parse message", zap.Error(err), zap.String("line", string(line)))
		return
	}

	c.logger.Debug("claudecode: parsed message",
		zap.String("type", msg.Type),
		zap.String("request_id", msg.RequestID),
		zap.Bool("has_response", msg.Response != nil))

	// Handle control requests (from Claude to us, e.g., permission requests)
	if msg.Type == MessageTypeControlRequest && msg.Request != nil {
		c.handleControlRequest(msg.RequestID, msg.Request)
		return
	}

	// Handle control responses (from Claude back to us, e.g., initialize response)
	// Note: request_id is inside the response object, not at the message level
	if msg.Type == MessageTypeControlResponse && msg.Response != nil {
		c.handleControlResponse(msg.Response)
		return
	}

	// Forward other messages to the message handler
	c.mu.RLock()
	handler := c.messageHandler
	c.mu.RUnlock()

	if handler != nil {
		// Store raw line for advanced parsing if needed
		msg.RawContent = line
		handler(&msg)
	}
}

func (c *Client) handleControlRequest(requestID string, req *ControlRequest) {
	c.mu.RLock()
	handler := c.requestHandler
	c.mu.RUnlock()

	if handler != nil {
		handler(requestID, req)
	} else {
		c.logger.Warn("received control request but no handler registered",
			zap.String("request_id", requestID),
			zap.String("subtype", req.Subtype))
		// Auto-deny if no handler
		if err := c.SendControlResponse(&ControlResponseMessage{
			Type:      MessageTypeControlResponse,
			RequestID: requestID,
			Response: &ControlResponse{
				Subtype: "error",
				Error:   "no handler registered",
			},
		}); err != nil {
			c.logger.Warn("failed to send error response", zap.Error(err))
		}
	}
}

func (c *Client) handleControlResponse(resp *IncomingControlResponse) {
	requestID := resp.RequestID

	c.pendingRequestsMu.Lock()
	pending, ok := c.pendingRequests[requestID]
	c.pendingRequestsMu.Unlock()

	if !ok {
		c.logger.Warn("received control response for unknown request",
			zap.String("request_id", requestID),
			zap.String("subtype", resp.Subtype))
		return
	}

	c.logger.Info("received control response",
		zap.String("request_id", requestID),
		zap.String("subtype", resp.Subtype))

	// Send response to waiting goroutine
	select {
	case pending.ch <- resp:
	default:
		c.logger.Warn("pending request channel full", zap.String("request_id", requestID))
	}
}
