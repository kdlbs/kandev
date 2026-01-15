package codex

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

// Client handles Codex JSON-RPC communication over stdin/stdout streams.
// Unlike standard JSON-RPC 2.0, Codex omits the "jsonrpc":"2.0" field.
type Client struct {
	stdin  io.Writer
	stdout io.Reader

	requestID atomic.Int64
	pending   map[interface{}]chan *Response
	mu        sync.Mutex

	onNotification func(method string, params json.RawMessage)
	onRequest      func(id interface{}, method string, params json.RawMessage)

	logger *logger.Logger
	done   chan struct{}
}

// NewClient creates a new Codex JSON-RPC client
func NewClient(stdin io.Writer, stdout io.Reader, log *logger.Logger) *Client {
	return &Client{
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[interface{}]chan *Response),
		logger:  log.WithFields(zap.String("component", "codex-client")),
		done:    make(chan struct{}),
	}
}

// SetNotificationHandler sets the handler for incoming notifications
func (c *Client) SetNotificationHandler(handler func(method string, params json.RawMessage)) {
	c.onNotification = handler
}

// SetRequestHandler sets the handler for incoming requests from the agent
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
	resp := &Response{ID: id, Result: resultJSON, Error: err}
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

	req := &Request{ID: id, Method: method, Params: paramsJSON}

	respCh := make(chan *Response, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.send(req); err != nil {
		return nil, err
	}

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
	notif := &Notification{Method: method, Params: paramsJSON}
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
	c.logger.Debug("codex: sent message", zap.String("data", string(data)))
	return nil
}

func (c *Client) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(c.stdout)
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

		var msg struct {
			ID     interface{}     `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *Error          `json:"error"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			c.logger.Warn("failed to parse message", zap.Error(err))
			continue
		}

		hasID := msg.ID != nil
		hasMethod := msg.Method != ""
		hasResult := msg.Result != nil
		hasError := msg.Error != nil

		if hasID && !hasMethod && (hasResult || hasError) {
			c.handleResponse(&Response{ID: msg.ID, Result: msg.Result, Error: msg.Error})
		} else if hasID && hasMethod {
			c.handleRequest(msg.ID, msg.Method, msg.Params)
		} else if hasMethod && !hasID {
			c.handleNotification(msg.Method, msg.Params)
		}
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error("read loop error", zap.Error(err))
	}
}

func (c *Client) handleResponse(resp *Response) {
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

func (c *Client) handleNotification(method string, params json.RawMessage) {
	if c.onNotification != nil {
		c.onNotification(method, params)
	}
}

func (c *Client) handleRequest(id interface{}, method string, params json.RawMessage) {
	if c.onRequest != nil {
		c.onRequest(id, method, params)
	} else {
		c.logger.Warn("received request but no handler registered", zap.String("method", method))
		if err := c.SendResponse(id, nil, &Error{Code: MethodNotFound, Message: "Method not found"}); err != nil {
			c.logger.Warn("failed to send method not found response", zap.Error(err))
		}
	}
}
