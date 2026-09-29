// Package codexappserver implements Codex app-server's JSON-RPC protocol.
package codexappserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	defaultMaxFrameBytes = 10 * 1024 * 1024
	defaultQueueSize     = 128
)

var (
	ErrClosed           = errors.New("codex app-server client closed")
	ErrForkPrecondition = errors.New("codex app-server fork was refused before provider RPC")
)

// Options bounds individual frames and queued server events. Zero values use
// the protocol client's defaults.
type Options struct {
	MaxFrameBytes         int
	QueueSize             int
	MaxConcurrentRequests int
}

// RPCError is an error returned by a JSON-RPC peer.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("Codex app-server RPC error %d: %s", e.Code, e.Message)
}

// ServerRequest is a JSON-RPC request initiated by the app-server. ID preserves
// its original JSON type so numeric and string request IDs remain distinct.
type ServerRequest struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

// RequestHandler answers JSON-RPC requests initiated by the app-server.
type RequestHandler func(context.Context, ServerRequest) (any, error)

// NotificationHandler receives notifications initiated by the app-server.
type NotificationHandler func(context.Context, string, json.RawMessage)

// FrameDirection identifies which side of the app-server transport emitted a
// captured JSON-RPC frame.
type FrameDirection string

const (
	FrameSent     FrameDirection = "sent"
	FrameReceived FrameDirection = "received"
)

// FrameObserver receives each complete frame in transport order. Returning an
// error stops the client so a capture failure cannot silently lose evidence.
type FrameObserver func(FrameDirection, json.RawMessage) error

type response struct {
	result json.RawMessage
	err    error
}

type pendingCall struct {
	response chan response
}

type outboundFrame struct {
	ctx     context.Context
	data    []byte
	done    chan error
	started chan struct{}
	written chan struct{}
}

type inboundMessage struct {
	id            json.RawMessage
	method        string
	params        json.RawMessage
	request       bool
	serverRequest *serverRequestState
	barrier       chan struct{}
}

type serverRequestStatus uint8

const (
	serverRequestPending serverRequestStatus = iota
	serverRequestResponding
	serverRequestResolved
	serverRequestReplied
)

type serverRequestState struct {
	key    string
	id     json.RawMessage
	ctx    context.Context
	cancel context.CancelFunc
	status serverRequestStatus
}

type wireMessage struct {
	// Codex app-server uses JSON-RPC-shaped envelopes but omits the optional
	// jsonrpc version field on inbound messages.
	JSONRPC json.RawMessage `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

// Client multiplexes requests over caller-owned stdin/stdout. Close stops the
// protocol loops and closes those streams when their concrete types implement
// io.Closer; process ownership remains with the caller.
type Client struct {
	stdout io.Reader
	stdin  io.Writer
	reader *bufio.Reader

	maxFrameBytes int
	outbound      chan outboundFrame
	inbound       chan inboundMessage
	requestSlots  chan struct{}
	done          chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc

	mu              sync.Mutex
	closeOnce       sync.Once
	streamCloseOnce sync.Once
	closeErr        error
	streamCloseErr  error
	requestHandler  RequestHandler
	notifyHandler   NotificationHandler
	frameObserver   FrameObserver
	pending         map[string]pendingCall
	serverRequests  map[string]*serverRequestState
	nextID          uint64
	unknownResponse atomic.Uint64
}

// NewClient starts a protocol reader and a serialized writer for the supplied
// streams. The client does not start or supervise an app-server process.
func NewClient(stdin io.Writer, stdout io.Reader, options Options) *Client {
	if options.MaxFrameBytes <= 0 {
		options.MaxFrameBytes = defaultMaxFrameBytes
	}
	if options.QueueSize <= 0 {
		options.QueueSize = defaultQueueSize
	}
	if options.MaxConcurrentRequests <= 0 {
		options.MaxConcurrentRequests = options.QueueSize
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		stdout:         stdout,
		stdin:          stdin,
		reader:         bufio.NewReaderSize(stdout, 64*1024),
		maxFrameBytes:  options.MaxFrameBytes,
		outbound:       make(chan outboundFrame, options.QueueSize),
		inbound:        make(chan inboundMessage, options.QueueSize),
		requestSlots:   make(chan struct{}, options.MaxConcurrentRequests),
		done:           make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
		pending:        make(map[string]pendingCall),
		serverRequests: make(map[string]*serverRequestState),
	}
	go c.readLoop()
	go c.writeLoop()
	go c.dispatchLoop()
	return c
}

// SetRequestHandler installs the handler for requests sent by the server.
func (c *Client) SetRequestHandler(handler RequestHandler) {
	c.mu.Lock()
	c.requestHandler = handler
	c.mu.Unlock()
}

// SetNotificationHandler installs the handler for server notifications.
func (c *Client) SetNotificationHandler(handler NotificationHandler) {
	c.mu.Lock()
	c.notifyHandler = handler
	c.mu.Unlock()
}

// SetFrameObserver installs an observer for raw JSON-RPC frames.
func (c *Client) SetFrameObserver(observer FrameObserver) {
	c.mu.Lock()
	c.frameObserver = observer
	c.mu.Unlock()
}

// Call sends a request and decodes its result into result. Pass nil to discard
// the response body.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	raw, err := c.CallRaw(ctx, method, params)
	if err != nil || result == nil || len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return err
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	return nil
}

// CallRaw sends a request and returns the response JSON without interpreting
// provider-specific fields.
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if strings.TrimSpace(method) == "" {
		return nil, errors.New("JSON-RPC method is required")
	}
	if ctx == nil {
		return nil, errors.New("JSON-RPC call context is required")
	}
	if err := c.terminalError(); err != nil {
		return nil, err
	}
	if err := c.Err(); err != nil {
		return nil, err
	}
	id := strconv.FormatUint(atomic.AddUint64(&c.nextID, 1), 10)
	idJSON := json.RawMessage(id)
	key := "n:" + id
	pending := pendingCall{response: make(chan response, 1)}
	c.mu.Lock()
	if c.closeErr != nil {
		err := c.closeErr
		c.mu.Unlock()
		return nil, err
	}
	c.pending[key] = pending
	c.mu.Unlock()

	frame := map[string]any{"jsonrpc": "2.0", "id": idJSON, "method": method}
	if params != nil {
		frame["params"] = params
	}
	if err := c.send(ctx, frame); err != nil {
		c.removePending(key)
		return nil, err
	}
	select {
	case got := <-pending.response:
		if got.err != nil {
			return nil, got.err
		}
		return got.result, nil
	case <-ctx.Done():
		c.removePending(key)
		return nil, ctx.Err()
	case <-c.done:
		c.removePending(key)
		return nil, c.terminalError()
	}
}

// FlushInbound waits until every server message read before this call has been
// dispatched. RPC responses bypass the serialized notification dispatcher, so
// callers that synthesize terminal events from a response can use this barrier
// to preserve wire order.
func (c *Client) FlushInbound(ctx context.Context) error {
	if ctx == nil {
		return errors.New("JSON-RPC flush context is required")
	}
	if err := c.terminalError(); err != nil {
		return err
	}
	barrier := make(chan struct{})
	select {
	case c.inbound <- inboundMessage{barrier: barrier}:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	}
	select {
	case <-barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	}
}

// Notify sends a JSON-RPC notification without waiting for a response.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if strings.TrimSpace(method) == "" {
		return errors.New("JSON-RPC method is required")
	}
	if params == nil {
		params = struct{}{}
	}
	return c.send(ctx, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// Done closes after a transport or protocol failure, or after Close is called.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns the terminal transport error, if one occurred.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

// UnknownResponseCount reports responses whose request was canceled or is not
// known to this client. The count is bounded to one integer and contains no
// provider or user identifiers.
func (c *Client) UnknownResponseCount() uint64 { return c.unknownResponse.Load() }

// Close ends the protocol loops and closes any closable streams. It does not
// kill a process that owns those streams.
func (c *Client) Close() error {
	c.terminate(ErrClosed)
	return c.closeStreams()
}

func (c *Client) closeStreams() error {
	c.streamCloseOnce.Do(func() {
		if closer, ok := c.stdout.(io.Closer); ok {
			c.streamCloseErr = errors.Join(c.streamCloseErr, closer.Close())
		}
		if closer, ok := c.stdin.(io.Closer); ok {
			c.streamCloseErr = errors.Join(c.streamCloseErr, closer.Close())
		}
	})
	return c.streamCloseErr
}

func (c *Client) send(ctx context.Context, frame any) error {
	if ctx == nil {
		return errors.New("JSON-RPC write context is required")
	}
	if err := c.terminalError(); err != nil {
		return err
	}
	if err := c.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC frame: %w", err)
	}
	if len(data) > c.maxFrameBytes {
		return fmt.Errorf("JSON-RPC frame exceeds %d bytes", c.maxFrameBytes)
	}
	write := outboundFrame{
		ctx: ctx, data: append(data, '\n'), done: make(chan error, 1),
		started: make(chan struct{}), written: make(chan struct{}),
	}
	select {
	case c.outbound <- write:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	}
	select {
	case err := <-write.done:
		return err
	case <-ctx.Done():
		if err := c.interruptWrite(write, ctx.Err()); err != nil {
			return err
		}
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	}
}

func (c *Client) sendAsync(ctx context.Context, frame any) error {
	if ctx == nil {
		return errors.New("JSON-RPC write context is required")
	}
	if err := c.terminalError(); err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC frame: %w", err)
	}
	if len(data) > c.maxFrameBytes {
		return fmt.Errorf("JSON-RPC frame exceeds %d bytes", c.maxFrameBytes)
	}
	write := outboundFrame{
		ctx: ctx, data: append(data, '\n'), done: make(chan error, 1),
		started: make(chan struct{}), written: make(chan struct{}),
	}
	select {
	case c.outbound <- write:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	default:
		return fmt.Errorf("app-server write queue exceeds %d messages", cap(c.outbound))
	}
}

func (c *Client) interruptWrite(write outboundFrame, cause error) error {
	select {
	case err := <-write.done:
		return err
	default:
	}
	select {
	case <-write.written:
		return nil
	default:
	}
	select {
	case <-write.started:
		select {
		case err := <-write.done:
			return err
		case <-write.written:
			return nil
		default:
			c.terminate(fmt.Errorf("write app-server frame: %w", cause))
			return nil
		}
	default:
		return nil
	}
}

func (c *Client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case write := <-c.outbound:
			if err := write.ctx.Err(); err != nil {
				write.done <- err
				continue
			}
			close(write.started)
			err := writeFrame(c.stdin, write.data)
			if err != nil {
				err = fmt.Errorf("write app-server frame: %w", err)
				write.done <- err
				c.terminate(err)
				return
			}
			close(write.written)
			if err := c.observeFrame(FrameSent, bytes.TrimSuffix(write.data, []byte{'\n'})); err != nil {
				write.done <- err
				c.terminate(err)
				return
			}
			write.done <- nil
		}
	}
}

func writeFrame(writer io.Writer, data []byte) error {
	remaining := data
	for len(remaining) > 0 {
		count, err := writer.Write(remaining)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		remaining = remaining[count:]
	}
	return nil
}

func (c *Client) readLoop() {
	for {
		line, err := readLine(c.reader, c.maxFrameBytes)
		if err != nil {
			c.terminate(fmt.Errorf("read app-server frame: %w", err))
			return
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := c.observeFrame(FrameReceived, line); err != nil {
			c.terminate(err)
			return
		}
		message, err := decodeWireMessage(line)
		if err == nil {
			err = c.handleWireMessage(message)
		}
		if err != nil {
			c.terminate(err)
			return
		}
	}
}

func decodeWireMessage(line []byte) (wireMessage, error) {
	var message wireMessage
	if err := json.Unmarshal(line, &message); err != nil {
		return wireMessage{}, fmt.Errorf("decode app-server frame: %w", err)
	}
	if len(message.JSONRPC) != 0 {
		var version string
		if err := json.Unmarshal(message.JSONRPC, &version); err != nil || version != "2.0" {
			return wireMessage{}, fmt.Errorf("unsupported JSON-RPC version %s", message.JSONRPC)
		}
	}
	if message.Method != "" {
		if len(message.Result) != 0 || message.Error != nil {
			return wireMessage{}, errors.New("app-server request contains response fields")
		}
		if len(message.ID) != 0 && (string(message.ID) == "null" || !validID(message.ID)) {
			return wireMessage{}, errors.New("app-server request has invalid id")
		}
		return message, nil
	}
	if len(message.ID) == 0 || !validID(message.ID) || (len(message.Result) == 0) == (message.Error == nil) {
		return wireMessage{}, errors.New("invalid app-server JSON-RPC response")
	}
	return message, nil
}

func (c *Client) handleWireMessage(message wireMessage) error {
	if message.Method == "" {
		return c.handleRPCResponse(message)
	}
	return c.handleServerNotification(message)
}

func (c *Client) handleServerNotification(message wireMessage) error {
	inbound := inboundMessage{method: message.Method, params: message.Params, id: message.ID, request: len(message.ID) != 0}
	if inbound.request {
		state, err := c.registerServerRequest(message.ID)
		if err != nil {
			return err
		}
		inbound.serverRequest = state
	}
	if message.Method == NotificationServerRequestResolved {
		c.resolveServerRequest(message.Params)
	}
	if err := c.enqueueInbound(inbound); err != nil {
		if inbound.serverRequest != nil {
			c.finishServerRequest(inbound.serverRequest)
		}
		return err
	}
	return nil
}

func (c *Client) handleRPCResponse(message wireMessage) error {
	key, err := responseIDKey(message.ID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	pending, ok := c.pending[key]
	if ok {
		delete(c.pending, key)
	}
	c.mu.Unlock()
	if !ok {
		c.unknownResponse.Add(1)
		return nil
	}
	var rpcErr error
	if message.Error != nil {
		rpcErr = message.Error
	}
	pending.response <- response{result: message.Result, err: rpcErr}
	return nil
}

func (c *Client) registerServerRequest(id json.RawMessage) (*serverRequestState, error) {
	key, err := responseIDKey(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	state := &serverRequestState{key: key, id: append(json.RawMessage(nil), id...), ctx: ctx, cancel: cancel}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.serverRequests[key]; exists {
		cancel()
		return nil, fmt.Errorf("duplicate outstanding app-server request id %s", id)
	}
	c.serverRequests[key] = state
	return state, nil
}

func (c *Client) resolveServerRequest(params json.RawMessage) {
	var notification struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &notification) != nil || len(notification.RequestID) == 0 {
		return
	}
	key, err := responseIDKey(notification.RequestID)
	if err != nil {
		return
	}
	c.mu.Lock()
	state := c.serverRequests[key]
	if state != nil && state.status == serverRequestPending {
		state.status = serverRequestResolved
		delete(c.serverRequests, key)
		state.cancel()
		state = nil
	}
	c.mu.Unlock()
}

func (c *Client) beginServerRequestResponse(state *serverRequestState) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state == nil || state.status != serverRequestPending {
		return false
	}
	state.status = serverRequestResponding
	return true
}

func (c *Client) finishServerRequest(state *serverRequestState) {
	if state == nil {
		return
	}
	c.mu.Lock()
	if c.serverRequests[state.key] == state {
		delete(c.serverRequests, state.key)
	}
	if state.status != serverRequestResolved {
		state.status = serverRequestReplied
	}
	c.mu.Unlock()
	state.cancel()
}

func (c *Client) enqueueInbound(message inboundMessage) error {
	select {
	case c.inbound <- message:
		return nil
	case <-c.done:
		return c.terminalError()
	default:
		return fmt.Errorf("app-server event queue exceeds %d messages", cap(c.inbound))
	}
}

func (c *Client) dispatchLoop() {
	for {
		select {
		case <-c.done:
			return
		case message := <-c.inbound:
			if message.barrier != nil {
				close(message.barrier)
				continue
			}
			if message.request {
				c.dispatchRequest(message)
				continue
			}
			c.mu.Lock()
			handler := c.notifyHandler
			c.mu.Unlock()
			if handler != nil {
				handler(c.ctx, message.method, message.params)
			}
		}
	}
}

func (c *Client) dispatchRequest(message inboundMessage) {
	if !c.serverRequestPending(message.serverRequest) {
		return
	}
	select {
	case c.requestSlots <- struct{}{}:
		if !c.serverRequestPending(message.serverRequest) {
			<-c.requestSlots
			return
		}
		go c.handleRequest(message)
	default:
		if !c.beginServerRequestResponse(message.serverRequest) {
			return
		}
		frame := serverResponseFrame(message.id, nil, &RPCError{
			Code: -32000, Message: "too many concurrent app-server requests",
		})
		if err := c.sendAsync(c.ctx, frame); err != nil {
			c.terminate(err)
		}
		c.finishServerRequest(message.serverRequest)
	}
}

func (c *Client) handleRequest(message inboundMessage) {
	defer func() { <-c.requestSlots }()
	c.mu.Lock()
	requestHandler := c.requestHandler
	c.mu.Unlock()
	result, rpcErr := executeServerRequest(message.serverRequest.ctx, requestHandler, ServerRequest{
		ID: message.id, Method: message.method, Params: message.params,
	})
	if !c.beginServerRequestResponse(message.serverRequest) {
		return
	}
	frame := serverResponseFrame(message.id, result, rpcErr)
	if err := c.send(c.ctx, frame); err != nil && !errors.Is(err, ErrClosed) {
		c.terminate(err)
	}
	c.finishServerRequest(message.serverRequest)
}

func (c *Client) serverRequestPending(state *serverRequestState) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return state != nil && state.status == serverRequestPending
}

func serverResponseFrame(id json.RawMessage, result any, rpcErr *RPCError) map[string]any {
	frame := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		frame["error"] = rpcErr
	} else {
		frame["result"] = result
	}
	return frame
}

func executeServerRequest(ctx context.Context, handler RequestHandler, request ServerRequest) (any, *RPCError) {
	if handler == nil {
		return nil, &RPCError{Code: -32601, Message: "method not found: " + request.Method}
	}
	result, err := handler(ctx, request)
	if err == nil {
		return result, nil
	}
	var typed *RPCError
	if errors.As(err, &typed) {
		return nil, typed
	}
	return nil, &RPCError{Code: -32603, Message: "internal error"}
}

func (c *Client) removePending(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

func (c *Client) observeFrame(direction FrameDirection, frame []byte) error {
	c.mu.Lock()
	observer := c.frameObserver
	c.mu.Unlock()
	if observer == nil {
		return nil
	}
	if err := observer(direction, append(json.RawMessage(nil), frame...)); err != nil {
		return fmt.Errorf("record %s app-server frame: %w", direction, err)
	}
	return nil
}

func (c *Client) terminalError() error {
	select {
	case <-c.done:
	default:
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeErr != nil {
		return c.closeErr
	}
	return ErrClosed
}

func (c *Client) terminate(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closeErr = err
		pending := c.pending
		c.pending = make(map[string]pendingCall)
		serverRequests := c.serverRequests
		c.serverRequests = make(map[string]*serverRequestState)
		for _, request := range serverRequests {
			request.status = serverRequestResolved
		}
		c.mu.Unlock()
		c.cancel()
		close(c.done)
		for _, call := range pending {
			call.response <- response{err: err}
		}
		for _, request := range serverRequests {
			request.cancel()
		}
		_ = c.closeStreams()
	})
}

func readLine(reader *bufio.Reader, limit int) ([]byte, error) {
	line := make([]byte, 0, 4096)
	for {
		part, err := reader.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > limit+1 {
			return nil, fmt.Errorf("frame exceeds %d bytes", limit)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if len(line) == 0 {
			return nil, io.EOF
		}
		if line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) > limit {
			return nil, fmt.Errorf("frame exceeds %d bytes", limit)
		}
		return line, nil
	}
}

func validID(id json.RawMessage) bool {
	if len(id) == 0 || bytes.Equal(id, []byte("null")) {
		return false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	switch value.(type) {
	case string, json.Number:
		return true
	default:
		return false
	}
}

func responseIDKey(id json.RawMessage) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode JSON-RPC id: %w", err)
	}
	switch typed := value.(type) {
	case string:
		return "s:" + typed, nil
	case json.Number:
		return "n:" + typed.String(), nil
	default:
		return "", fmt.Errorf("unsupported JSON-RPC id %s", id)
	}
}
