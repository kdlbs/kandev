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

	// Request handler for agent-to-client requests (like session/request_permission)
	onRequest func(id interface{}, method string, params json.RawMessage)

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

// SetRequestHandler sets the handler for incoming requests from the agent
// (e.g., session/request_permission). The handler should call SendResponse to reply.
func (c *Client) SetRequestHandler(handler func(id interface{}, method string, params json.RawMessage)) {
	c.onRequest = handler
}

// SendResponse sends a response to an agent request
func (c *Client) SendResponse(id interface{}, result interface{}, err *Error) error {
	var resultJSON json.RawMessage
	if result != nil && err == nil {
		var marshalErr error
		resultJSON, marshalErr = json.Marshal(result)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal result: %w", marshalErr)
		}
	}

	resp := &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
		Error:   err,
	}

	return c.send(resp)
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

		// Parse the message to determine its type
		// JSON-RPC 2.0 message types:
		// - Response: has "id" + ("result" OR "error"), no "method"
		// - Request: has "id" + "method"
		// - Notification: has "method", no "id"
		var msg struct {
			ID     interface{}     `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *Error          `json:"error"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			c.logger.Warn("failed to parse message", zap.Error(err), zap.String("data", string(line)))
			continue
		}

		// Determine message type based on fields present
		hasID := msg.ID != nil
		hasMethod := msg.Method != ""
		hasResult := msg.Result != nil
		hasError := msg.Error != nil

		if hasID && !hasMethod && (hasResult || hasError) {
			// This is a response to our request
			resp := &Response{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  msg.Result,
				Error:   msg.Error,
			}
			c.handleResponse(resp)
		} else if hasID && hasMethod {
			// This is a request FROM the agent (e.g., session/request_permission)
			c.handleRequest(msg.ID, msg.Method, msg.Params)
		} else if hasMethod && !hasID {
			// This is a notification
			notif := &Notification{
				JSONRPC: "2.0",
				Method:  msg.Method,
				Params:  msg.Params,
			}
			c.handleNotification(notif)
		} else {
			c.logger.Warn("received unknown message format", zap.String("data", string(line)))
		}
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error("read loop error", zap.Error(err))
	}
}

func (c *Client) handleResponse(resp *Response) {
	// Normalize the ID - JSON unmarshals numbers as float64, but we store as int64
	id := normalizeID(resp.ID)

	c.mu.Lock()
	ch, ok := c.pending[id]
	c.mu.Unlock()

	if ok {
		ch <- resp
	} else {
		c.logger.Warn("received response for unknown request", zap.Any("id", resp.ID))
	}
}

// normalizeID converts JSON unmarshaled IDs to a consistent type for map lookup.
// JSON numbers are unmarshaled as float64, but we store request IDs as int64.
func normalizeID(id interface{}) interface{} {
	switch v := id.(type) {
	case float64:
		return int64(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
	}
	return id
}

func (c *Client) handleNotification(notif *Notification) {
	if c.onNotification != nil {
		c.onNotification(notif.Method, notif.Params)
	}
}

func (c *Client) handleRequest(id interface{}, method string, params json.RawMessage) {
	if c.onRequest != nil {
		c.onRequest(id, method, params)
	} else {
		// No handler registered, send error response
		c.logger.Warn("received request but no handler registered",
			zap.Any("id", id),
			zap.String("method", method))
		c.SendResponse(id, nil, &Error{
			Code:    MethodNotFound,
			Message: "Method not found",
		})
	}
}

