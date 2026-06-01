package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
)

// closeBarrierMockServer is a minimal agentctl mock that exposes the two
// WebSocket endpoints needed for Close/drain regression coverage. Handlers
// stay open until the client tears down, mirroring the behaviour real
// agentctl exhibits when the manager hasn't asked it to exit.
type closeBarrierMockServer struct {
	server *httptest.Server

	mu        sync.Mutex
	wsConns   []*websocket.Conn
	connected chan struct{}
	once      sync.Once
}

func newCloseBarrierMockServer(t *testing.T) *closeBarrierMockServer {
	t.Helper()
	m := &closeBarrierMockServer{connected: make(chan struct{})}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		m.mu.Lock()
		m.wsConns = append(m.wsConns, conn)
		m.mu.Unlock()
		m.once.Do(func() { close(m.connected) })
		// Block until client closes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				_ = conn.Close()
				return
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agent/stream", handler)
	mux.HandleFunc("/api/v1/workspace/stream", handler)
	m.server = httptest.NewServer(mux)
	t.Cleanup(func() {
		m.mu.Lock()
		for _, c := range m.wsConns {
			_ = c.Close()
		}
		m.mu.Unlock()
		m.server.Close()
	})
	return m
}

func newCloseBarrierTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	url := strings.TrimPrefix(serverURL, "http://")
	parts := strings.SplitN(url, ":", 2)
	host := parts[0]
	var port int
	_, _ = fmt.Sscanf(parts[1], "%d", &port)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return NewClient(host, port, log)
}

// TestClientClose_DrainsStreamGoroutines is the regression test for the
// CI-only goleak flake described in issue/PR around StreamManager and
// WorkspaceStream goroutines surviving Close. After Close returns, every
// read/write loop the client spawned (updates stream + workspace stream)
// must have fully unwound — otherwise tests with `defer client.Close()`
// see lingering goroutines and goleak.VerifyTestMain fails.
func TestClientClose_DrainsStreamGoroutines(t *testing.T) {
	mock := newCloseBarrierMockServer(t)
	client := newCloseBarrierTestClient(t, mock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.StreamUpdates(ctx, func(AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("StreamUpdates failed: %v", err)
	}
	ws, err := client.StreamWorkspace(ctx, WorkspaceStreamCallbacks{})
	if err != nil {
		t.Fatalf("StreamWorkspace failed: %v", err)
	}
	if ws == nil {
		t.Fatal("nil WorkspaceStream")
	}

	// Wait for both server-side WS handlers to register so we know the
	// goroutines we're testing the drain of are actually live.
	select {
	case <-mock.connected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server never observed a WS connection")
	}

	// Close must return promptly and have drained both streams. A hung
	// goroutine here would block Close forever (or, pre-fix, return early
	// and leave the goroutine running past goleak's check).
	done := make(chan struct{})
	go func() {
		client.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Client.Close did not return within 2s — goroutine drain is stuck")
	}

	// Post-Close, Stream* calls must error so a racy second close path
	// doesn't strand a new dial past the barrier.
	if err := client.StreamUpdates(context.Background(), func(AgentEvent) {}, nil, nil); err == nil {
		t.Error("StreamUpdates after Close should return error, got nil")
	}
	if _, err := client.StreamWorkspace(context.Background(), WorkspaceStreamCallbacks{}); err == nil {
		t.Error("StreamWorkspace after Close should return error, got nil")
	}
}
