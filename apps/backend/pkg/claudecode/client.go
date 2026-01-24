package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// RequestHandler handles incoming control requests from Claude Code CLI.
// It receives the request ID and control request, and should call SendControlResponse.
type RequestHandler func(requestID string, req *ControlRequest)

// MessageHandler handles streaming messages from Claude Code CLI.
type MessageHandler func(msg *CLIMessage)

// Client handles Claude Code CLI communication over stdin/stdout streams.
// It reads streaming JSON from stdout and writes control messages to stdin.
type Client struct {
	stdin  io.Writer
	stdout io.Reader
	logger *logger.Logger

	// Handlers for incoming messages
	requestHandler RequestHandler
	messageHandler MessageHandler

	// Synchronization
	mu   sync.RWMutex
	done chan struct{}
}

// NewClient creates a new Claude Code CLI client.
func NewClient(stdin io.Writer, stdout io.Reader, log *logger.Logger) *Client {
	return &Client{
		stdin:  stdin,
		stdout: stdout,
		logger: log.WithFields(zap.String("component", "claudecode-client")),
		done:   make(chan struct{}),
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
func (c *Client) Start(ctx context.Context) {
	go c.readLoop(ctx)
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

func (c *Client) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(c.stdout)
	// Allow for large JSON messages (up to 10MB)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

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
	// First, parse the basic structure to determine message type
	var msg CLIMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.logger.Warn("failed to parse message", zap.Error(err), zap.String("line", string(line)))
		return
	}

	c.logger.Debug("claudecode: received message",
		zap.String("type", msg.Type),
		zap.String("request_id", msg.RequestID))

	// Handle control requests specially - they need immediate response
	if msg.Type == MessageTypeControlRequest && msg.Request != nil {
		c.handleControlRequest(msg.RequestID, msg.Request)
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
