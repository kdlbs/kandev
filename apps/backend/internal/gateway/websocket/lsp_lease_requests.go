package websocket

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

func (l *lspLease) addClientRequestLocked(
	generation uint64,
	clientID []byte,
	serverID []byte,
) error {
	if len(clientID) == 0 || len(clientID) > lspLeaseMaxClientRequestIDBytes {
		return errors.New("LSP request ID exceeds its limit")
	}
	key := jsonRPCIDKey(clientID)
	if _, exists := l.clientRequests[key]; exists {
		return errors.New("duplicate pending LSP request ID")
	}
	if len(l.clientRequests) >= lspLeaseMaxPendingClientRequests {
		return errors.New("too many pending LSP requests")
	}
	size := len(key) + len(clientID) + len(serverID) + 64
	if l.pendingClientRequestBytes+size > lspLeaseMaxPendingClientRequestBytes {
		return errors.New("pending LSP request state exceeds its limit")
	}
	request := lspLeaseClientRequest{
		serverID:   append([]byte(nil), serverID...),
		generation: generation,
		clientID:   append([]byte(nil), clientID...),
		byteSize:   size,
	}
	request.timer = time.AfterFunc(l.clientRequestTimeout, func() {
		l.expireClientRequest(key, generation, serverID)
	})
	l.clientRequests[key] = request
	l.pendingClientRequestBytes += size
	return nil
}

func (l *lspLease) removeClientRequestLocked(key string) (lspLeaseClientRequest, bool) {
	request, ok := l.clientRequests[key]
	if !ok {
		return lspLeaseClientRequest{}, false
	}
	delete(l.clientRequests, key)
	if request.timer != nil {
		request.timer.Stop()
	}
	l.pendingClientRequestBytes -= request.byteSize
	if l.pendingClientRequestBytes < 0 {
		l.pendingClientRequestBytes = 0
	}
	return request, true
}

func (l *lspLease) expireClientRequest(key string, generation uint64, serverID []byte) {
	l.mu.Lock()
	request, ok := l.clientRequests[key]
	if !ok || request.generation != generation || !bytes.Equal(request.serverID, serverID) {
		l.mu.Unlock()
		return
	}
	request, _ = l.removeClientRequestLocked(key)
	attached := !l.closed && l.browser != nil && l.generation == generation
	l.mu.Unlock()
	if !attached {
		return
	}
	_ = l.writeUpstreamForGeneration(generation, jsonRPCNotification("$/cancelRequest", map[string]any{"id": request.serverID}))
	_ = l.writeBrowserOrDetach(generation, json.RawMessage(jsonRPCErrorResponseRaw(request.clientID, -32800, "language server request timed out")))
}
