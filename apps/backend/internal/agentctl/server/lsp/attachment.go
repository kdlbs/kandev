package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const attachmentQueueSize = 256

type attachmentRequest struct {
	cancel context.CancelFunc
}

type Attachment struct {
	id  uint64
	hub *hub

	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	stateMu sync.Mutex
	closing bool

	pendingMu sync.Mutex
	pending   map[string]*attachmentRequest
	requests  sync.WaitGroup

	outMu    sync.Mutex
	outgoing chan []byte
	failed   bool

	failOnce  sync.Once
	closeOnce sync.Once
}

func newAttachment(id uint64, owner *hub) *Attachment {
	ctx, cancel := context.WithCancel(context.Background())
	return &Attachment{
		id:       id,
		hub:      owner,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		pending:  make(map[string]*attachmentRequest),
		outgoing: make(chan []byte, attachmentQueueSize),
	}
}

func (a *Attachment) Messages() <-chan []byte {
	return a.outgoing
}

func (a *Attachment) Done() <-chan struct{} {
	return a.done
}

func (a *Attachment) Handle(raw []byte) error {
	if len(raw) == 0 {
		return errors.New("empty attachment message")
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.closing {
		return ErrAttachmentClosed
	}
	var message rpcMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return fmt.Errorf("decode attachment message: %w", err)
	}
	if message.Method == "" {
		return errors.New("browser responses are not accepted by task-host LSP attachments")
	}
	if isLifecycleMethod(message.Method) {
		if hasRequestID(message.ID) {
			a.enqueueRPCError(message.ID, -32600, "lifecycle method is task-host owned")
		}
		return nil
	}
	if message.Method == "$/cancelRequest" {
		return a.handleCancellation(message.Params)
	}
	if isDocumentNotification(message.Method) && !hasRequestID(message.ID) {
		return a.hub.documents.Handle(a.id, message.Method, message.Params)
	}
	if hasRequestID(message.ID) {
		if !isAllowedFeatureRequest(message.Method) {
			a.enqueueRPCError(message.ID, -32601, "unsupported attachment method")
			return nil
		}
		return a.startRequest(message)
	}
	return nil
}

func (a *Attachment) Close() {
	a.close(true)
}

func (a *Attachment) close(detach bool) {
	a.closeOnce.Do(func() {
		a.stateMu.Lock()
		a.closing = true
		a.stateMu.Unlock()
		a.fail()
		a.cancelPending()
		a.requests.Wait()
		a.hub.documents.ReleaseAttachment(a.id)
		if detach {
			a.hub.detach(a.id)
		}
		a.outMu.Lock()
		close(a.outgoing)
		a.outMu.Unlock()
	})
}

func (a *Attachment) fail() {
	a.failOnce.Do(func() {
		a.outMu.Lock()
		a.failed = true
		a.outMu.Unlock()
		a.cancel()
		close(a.done)
	})
}

func (a *Attachment) startRequest(message rpcMessage) error {
	key, ok := requestIDKey(message.ID)
	if !ok {
		a.enqueueRPCError(message.ID, -32600, "request id must be a string or number")
		return nil
	}
	ctx, cancel := context.WithCancel(a.ctx)
	request := &attachmentRequest{cancel: cancel}
	a.pendingMu.Lock()
	if _, duplicate := a.pending[key]; duplicate {
		a.pendingMu.Unlock()
		cancel()
		a.enqueueRPCError(message.ID, -32600, "duplicate pending request id")
		return nil
	}
	a.pending[key] = request
	a.pendingMu.Unlock()
	a.requests.Add(1)
	go func() {
		defer a.requests.Done()
		defer cancel()
		result, err := a.hub.upstream.Request(ctx, message.Method, message.Params)
		a.pendingMu.Lock()
		if a.pending[key] == request {
			delete(a.pending, key)
		}
		a.pendingMu.Unlock()
		select {
		case <-a.ctx.Done():
			return
		default:
		}
		if err != nil {
			code := -32603
			if errors.Is(err, context.Canceled) {
				code = -32800
			}
			a.enqueueRPCError(message.ID, code, err.Error())
			return
		}
		a.enqueueRPCResult(message.ID, result)
	}()
	return nil
}

func (a *Attachment) handleCancellation(raw json.RawMessage) error {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode cancellation: %w", err)
	}
	key, ok := requestIDKey(params.ID)
	if !ok {
		return nil
	}
	a.pendingMu.Lock()
	request := a.pending[key]
	a.pendingMu.Unlock()
	if request != nil {
		request.cancel()
	}
	return nil
}

func (a *Attachment) cancelPending() {
	a.pendingMu.Lock()
	requests := make([]*attachmentRequest, 0, len(a.pending))
	for _, request := range a.pending {
		requests = append(requests, request)
	}
	a.pendingMu.Unlock()
	for _, request := range requests {
		request.cancel()
	}
}

func (a *Attachment) enqueueRPCResult(id, result json.RawMessage) {
	if len(result) == 0 {
		result = json.RawMessage(jsonNull)
	}
	message, _ := json.Marshal(rpcMessage{JSONRPC: rpcVersion, ID: id, Result: result})
	if !a.enqueue(message) {
		a.fail()
	}
}

func (a *Attachment) enqueueRPCError(id json.RawMessage, code int, message string) {
	encoded, _ := json.Marshal(rpcMessage{
		JSONRPC: rpcVersion,
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
	if !a.enqueue(encoded) {
		a.fail()
	}
}

func (a *Attachment) enqueue(message []byte) bool {
	a.outMu.Lock()
	defer a.outMu.Unlock()
	if a.failed {
		return false
	}
	copy := append([]byte(nil), message...)
	select {
	case a.outgoing <- copy:
		return true
	default:
		return false
	}
}

func hasRequestID(id json.RawMessage) bool {
	return len(id) != 0 && string(id) != jsonNull
}

func requestIDKey(id json.RawMessage) (string, bool) {
	if !hasRequestID(id) {
		return "", false
	}
	var value any
	if json.Unmarshal(id, &value) != nil {
		return "", false
	}
	switch value.(type) {
	case string, float64:
		return string(id), true
	default:
		return "", false
	}
}

func isLifecycleMethod(method string) bool {
	switch method {
	case "initialize", methodInitialized, methodShutdown, methodExit, "workspace/didChangeConfiguration":
		return true
	default:
		return false
	}
}

func isDocumentNotification(method string) bool {
	switch method {
	case methodDidOpen, methodDidChange, methodDidSave, methodDidClose:
		return true
	default:
		return false
	}
}

func isAllowedFeatureRequest(method string) bool {
	return strings.HasPrefix(method, "textDocument/") || method == "workspace/symbol"
}
