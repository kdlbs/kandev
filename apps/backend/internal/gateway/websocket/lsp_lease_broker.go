package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const lspShutdownTimeout = 3 * time.Second

func (l *lspLease) gracefulRelease(generation uint64, reason, requestID string) error {
	l.mu.Lock()
	requests := make([]lspLeaseClientRequest, 0, len(l.clientRequests))
	for key, request := range l.clientRequests {
		if request.generation == generation {
			requests = append(requests, request)
			delete(l.clientRequests, key)
		}
	}
	l.mu.Unlock()
	for _, request := range requests {
		_ = l.writeBrowser(generation, jsonRPCErrorResponseRaw(request.clientID, -32800, "request cancelled because the language server is stopping"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), lspShutdownTimeout)
	defer cancel()
	if _, err := l.sendBrokerRequest(ctx, "shutdown", nil); err != nil {
		l.manager.logger.Debug("LSP shutdown request did not complete before release", zap.String("language", l.language), zap.Error(err))
	}
	_ = l.writeUpstream(jsonRPCNotification("exit", nil))
	if err := l.writeBrowser(generation, map[string]any{
		lspControlField: lspControlKind,
		"action":        lspControlAck,
		"requestId":     requestID,
		"reason":        reason,
	}); err != nil {
		return err
	}
	code := websocket.CloseNormalClosure
	text := "language server stopped"
	if reason == lspLeaseReleaseEditorIdle {
		text = "language server released after editor idle"
	}
	l.terminate(code, text, reason)
	return nil
}

func (l *lspLease) sendBrokerRequest(ctx context.Context, method string, params any) (jsonRPCResponse, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return jsonRPCResponse{}, errors.New("LSP lease has ended")
	}
	l.requestCounter++
	id := jsonRPCStringID(fmt.Sprintf("kandev:broker:%d", l.requestCounter))
	key := jsonRPCIDKey(id)
	response := make(chan jsonRPCResponse, 1)
	l.brokerRequests[key] = response
	l.mu.Unlock()
	message, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if err := l.writeUpstream(message); err != nil {
		l.mu.Lock()
		delete(l.brokerRequests, key)
		l.mu.Unlock()
		return jsonRPCResponse{}, err
	}
	select {
	case result := <-response:
		if len(result.Error) > 0 {
			return result, errors.New(string(result.Error))
		}
		return result, nil
	case <-ctx.Done():
		l.mu.Lock()
		delete(l.brokerRequests, key)
		l.mu.Unlock()
		return jsonRPCResponse{}, ctx.Err()
	}
}
