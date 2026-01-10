package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Client handles JSON-RPC 2.0 communication over stdin/stdout streams
type Client struct {
	stdin  io.Writer
	stdout io.Reader

	requestID atomic.Int64
	pending   map[interface{}]chan *Response
	mu        sync.Mutex

	// Notification handler
	onNotification func(method string, params json.RawMessage)

	logger *logger.Logger
	done   chan struct{}
}

// NewClient creates a new JSON-RPC client
func NewClient(stdin io.Writer, stdout io.Reader, log *logger.Logger) *Client {
	return &Client{
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[interface{}]chan *Response),
		logger:  log.WithFields(zap.String("component", "jsonrpc-client")),
		done:    make(chan struct{}),
	}
}

// SetNotificationHandler sets the handler for incoming notifications
func (c *Client) SetNotificationHandler(handler func(method string, params json.RawMessage)) {
	c.onNotification = handler
}

// Start begins reading responses from stdout
func (c *Client) Start(ctx context.Context) {
	go c.readLoop(ctx)
}

// Stop stops the client
func (c *Client) Stop() {
	close(c.done)
}

// Call sends a request and waits for a response
func (c *Client) Call(ctx context.Context, method string, params interface{}) (*Response, error) {
	id := c.requestID.Add(1)

	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	req := &Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	// Create response channel
	respCh := make(chan *Response, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send request
	if err := c.send(req); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	}
}

// Notify sends a notification (no response expected)
func (c *Client) Notify(method string, params interface{}) error {
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	notif := &Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}

	return c.send(notif)
}

func (c *Client) send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	c.logger.Debug("sent message", zap.String("data", string(data)))
	return nil
}

func (c *Client) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(c.stdout)
	// Increase buffer size for large messages
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

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

		c.logger.Debug("received message", zap.String("data", string(line)))

		// Try to parse as response first
		var resp Response
		if err := json.Unmarshal(line, &resp); err == nil && resp.ID != nil {
			c.handleResponse(&resp)
			continue
		}

		// Try to parse as notification
		var notif Notification
		if err := json.Unmarshal(line, &notif); err == nil && notif.Method != "" {
			c.handleNotification(&notif)
			continue
		}

		c.logger.Warn("received unknown message format", zap.String("data", string(line)))
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error("read loop error", zap.Error(err))
	}
}

func (c *Client) handleResponse(resp *Response) {
	c.mu.Lock()
	ch, ok := c.pending[resp.ID]
	c.mu.Unlock()

	if ok {
		ch <- resp
	} else {
		c.logger.Warn("received response for unknown request", zap.Any("id", resp.ID))
	}
}

func (c *Client) handleNotification(notif *Notification) {
	if c.onNotification != nil {
		c.onNotification(notif.Method, notif.Params)
	}
}

