package websocket

import (
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

func readLSPLeaseStatus(t *testing.T, conn *gorillaws.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal(message, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func readLSPJSONRPCMessage(t *testing.T, conn *gorillaws.Conn) map[string]json.RawMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var rpc map[string]json.RawMessage
	if err := json.Unmarshal(message, &rpc); err != nil {
		t.Fatal(err)
	}
	return rpc
}

func observeLSPFrames(t *testing.T, conn *gorillaws.Conn) <-chan []byte {
	t.Helper()
	frames := make(chan []byte, 8)
	go func() {
		defer close(frames)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			frames <- message
		}
	}()
	return frames
}

type lspBlockingCloseListener struct {
	net.Listener
	entered  chan struct{}
	release  <-chan struct{}
	accepted atomic.Int32
}

func (listener *lspBlockingCloseListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if listener.accepted.Add(1) == 1 {
		return &lspBlockingCloseConn{Conn: conn, entered: listener.entered, release: listener.release}, nil
	}
	return conn, nil
}

type lspBlockingCloseConn struct {
	net.Conn
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (conn *lspBlockingCloseConn) Close() error {
	conn.once.Do(func() {
		close(conn.entered)
		<-conn.release
	})
	return conn.Conn.Close()
}

func nextObservedLSPFrame(t *testing.T, frames <-chan []byte) []byte {
	t.Helper()
	select {
	case frame, ok := <-frames:
		if !ok {
			t.Fatal("upstream websocket closed before the expected frame")
		}
		return frame
	case <-time.After(wsTestTimeout):
		t.Fatal("timed out waiting for upstream LSP frame")
		return nil
	}
}

func decodeLSPJSONRPCMessage(t *testing.T, message []byte) map[string]json.RawMessage {
	t.Helper()
	var rpc map[string]json.RawMessage
	if err := json.Unmarshal(message, &rpc); err != nil {
		t.Fatal(err)
	}
	return rpc
}

func waitForLSPLease(t *testing.T, manager *lspLeaseManager, leaseID string, detached bool) {
	t.Helper()
	deadline := time.NewTimer(wsTestTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()
	for {
		manager.mu.Lock()
		lease := manager.leases[leaseID]
		isDetached := lease == nil || lease.isDetached()
		manager.mu.Unlock()
		if lease != nil && isDetached == detached {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("lease %q did not become detached=%t", leaseID, detached)
		}
	}
}

func documentVersionFromMessage(t *testing.T, message []byte) int64 {
	t.Helper()
	var payload struct {
		Params struct {
			TextDocument struct {
				Version int64 `json:"version"`
			} `json:"textDocument"`
		} `json:"params"`
	}
	if err := json.Unmarshal(message, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Params.TextDocument.Version
}

func readJSONRPCResponse(t *testing.T, conn *gorillaws.Conn) jsonRPCResponse {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var response jsonRPCResponse
	if err := json.Unmarshal(message, &response); err != nil {
		t.Fatal(err)
	}
	return response
}
