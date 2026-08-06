package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/kandev/kandev/internal/lsp/protocol"
)

var errPeerClosed = errors.New("language-server JSON-RPC peer closed")

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("JSON-RPC %d: %s", e.Code, e.Message)
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

type peer struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  uint64
	pending map[string]chan rpcResponse
	done    chan struct{}
	doneErr error

	onRequest      func(string, json.RawMessage) (any, error)
	onNotification func(string, json.RawMessage)
}

func newPeer(
	stdin io.WriteCloser,
	stdout io.ReadCloser,
	onRequest func(string, json.RawMessage) (any, error),
	onNotification func(string, json.RawMessage),
) *peer {
	p := &peer{
		stdin:          stdin,
		stdout:         stdout,
		pending:        make(map[string]chan rpcResponse),
		done:           make(chan struct{}),
		onRequest:      onRequest,
		onNotification: onNotification,
	}
	go p.readLoop()
	return p
}

func (p *peer) Call(ctx context.Context, method string, params any, result any) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal %s params: %w", method, err)
	}
	reply, err := p.callRaw(ctx, method, paramsJSON)
	if err != nil {
		return err
	}
	if result == nil || len(reply) == 0 || string(reply) == jsonNull {
		return nil
	}
	if err := json.Unmarshal(reply, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

func (p *peer) CallRaw(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if len(params) == 0 {
		params = json.RawMessage(jsonNull)
	}
	return p.callRaw(ctx, method, params)
}

func (p *peer) callRaw(
	ctx context.Context,
	method string,
	paramsJSON json.RawMessage,
) (json.RawMessage, error) {
	p.mu.Lock()
	select {
	case <-p.done:
		err := p.doneErr
		p.mu.Unlock()
		return nil, errors.Join(errPeerClosed, err)
	default:
	}
	p.nextID++
	idJSON, _ := json.Marshal(p.nextID)
	id := string(idJSON)
	response := make(chan rpcResponse, 1)
	p.pending[id] = response
	p.mu.Unlock()

	if err := p.write(rpcMessage{
		JSONRPC: rpcVersion,
		ID:      idJSON,
		Method:  method,
		Params:  paramsJSON,
	}); err != nil {
		p.removePending(id)
		return nil, err
	}

	select {
	case reply := <-response:
		if reply.err != nil {
			return nil, reply.err
		}
		return append(json.RawMessage(nil), reply.result...), nil
	case <-ctx.Done():
		_ = p.Notify("$/cancelRequest", map[string]any{"id": json.RawMessage(idJSON)})
		p.removePending(id)
		return nil, ctx.Err()
	case <-p.done:
		p.mu.Lock()
		err := p.doneErr
		p.mu.Unlock()
		return nil, errors.Join(errPeerClosed, err)
	}
}

func (p *peer) Notify(method string, params any) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal %s params: %w", method, err)
	}
	return p.write(rpcMessage{JSONRPC: rpcVersion, Method: method, Params: paramsJSON})
}

func (p *peer) Done() <-chan struct{} {
	return p.done
}

func (p *peer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.doneErr
}

func (p *peer) CloseInput() error {
	return p.stdin.Close()
}

func (p *peer) CloseStreams() {
	_ = p.stdin.Close()
	_ = p.stdout.Close()
}

func (p *peer) readLoop() {
	reader := bufio.NewReader(p.stdout)
	for {
		body, err := readPeerMessage(reader)
		if err != nil {
			p.finish(err)
			return
		}
		var message rpcMessage
		if err := json.Unmarshal(body, &message); err != nil {
			p.finish(fmt.Errorf("decode language-server message: %w", err))
			return
		}
		if message.Method != "" {
			if len(message.ID) != 0 {
				p.handleRequest(message)
			} else if p.onNotification != nil {
				p.onNotification(message.Method, message.Params)
			}
			continue
		}
		p.handleResponse(message)
	}
}

func readPeerMessage(reader *bufio.Reader) ([]byte, error) {
	body, err := protocol.ReadMessage(reader)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (p *peer) handleRequest(message rpcMessage) {
	var (
		result any
		err    error
	)
	if p.onRequest == nil {
		err = &rpcError{Code: -32601, Message: "method not found"}
	} else {
		result, err = p.onRequest(message.Method, message.Params)
	}
	response := rpcMessage{JSONRPC: rpcVersion, ID: message.ID}
	if err != nil {
		if protocolError, ok := err.(*rpcError); ok {
			response.Error = protocolError
		} else {
			response.Error = &rpcError{Code: -32603, Message: err.Error()}
		}
	} else {
		response.Result, _ = json.Marshal(result)
	}
	_ = p.write(response)
}

func (p *peer) handleResponse(message rpcMessage) {
	id := string(message.ID)
	p.mu.Lock()
	response := p.pending[id]
	delete(p.pending, id)
	p.mu.Unlock()
	if response == nil {
		return
	}
	if message.Error != nil {
		response <- rpcResponse{err: message.Error}
		return
	}
	response <- rpcResponse{result: message.Result}
}

func (p *peer) write(message rpcMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	select {
	case <-p.done:
		return errPeerClosed
	default:
	}
	if _, err := fmt.Fprintf(p.stdin, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	if _, err := p.stdin.Write(payload); err != nil {
		return err
	}
	return nil
}

func (p *peer) removePending(id string) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *peer) finish(err error) {
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		return
	default:
	}
	p.doneErr = err
	for id, response := range p.pending {
		delete(p.pending, id)
		response <- rpcResponse{err: errors.Join(errPeerClosed, err)}
	}
	close(p.done)
	p.mu.Unlock()
}
