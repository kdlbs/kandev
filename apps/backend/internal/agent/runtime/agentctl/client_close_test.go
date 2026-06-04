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

// TestClientClose_DrainsWorkspaceStream is the regression test for the
// CI-only goleak flake around StreamManager and WorkspaceStream goroutines
// surviving Close. After Close returns, the workspace read/write loops must
// have fully unwound — otherwise tests with `defer client.Close()` see
// lingering goroutines and goleak.VerifyTestMain fails. The agent (updates)
// stream is closed but not drained synchronously: the cascade flow legitimately
// stops + restarts the updates stream on the same client.
func TestClientClose_DrainsWorkspaceStream(t *testing.T) {
	mock := newCloseBarrierMockServer(t)
	client := newCloseBarrierTestClient(t, mock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ws, err := client.StreamWorkspace(ctx, WorkspaceStreamCallbacks{})
	if err != nil {
		t.Fatalf("StreamWorkspace failed: %v", err)
	}
	if ws == nil {
		t.Fatal("nil WorkspaceStream")
	}

	select {
	case <-mock.connected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server never observed a WS connection")
	}

	// Close must return promptly and have drained the workspace stream. A hung
	// goroutine here would block Close forever (or, pre-fix, return early and
	// leave the goroutine running past goleak's check).
	done := make(chan struct{})
	go func() {
		client.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Client.Close did not return within 2s — workspace drain is stuck")
	}

	// Post-Close, StreamWorkspace must error so a racy second close path
	// doesn't strand a new dial past the barrier.
	if _, err := client.StreamWorkspace(context.Background(), WorkspaceStreamCallbacks{}); err == nil {
		t.Error("StreamWorkspace after Close should return error, got nil")
	}
}

// TestClientClose_StreamWorkspaceAddWaitRace exercises the window where one
// goroutine has just published c.workspaceStream from StreamWorkspace and is
// about to call stream.wg.Add(2), while another goroutine fires Close —
// captures the same pointer and calls ws.Wait(). Pre-fix, the race detector
// flagged Add(2) vs Wait() on the same WaitGroup because Add ran after the
// publication unlock. With Add moved under the publication lock, the
// unlock/lock pair carries the Add into Close's view.
//
// The test waits for the mock server to observe the WS upgrade before
// firing Close — that is the same window the failing CI traces showed
// (test code observes the connection, returns, deferred Close runs while
// the StreamWorkspace goroutine has not yet reached Add(2)).
func TestClientClose_StreamWorkspaceAddWaitRace(t *testing.T) {
	const iterations = 100
	runIter := func(i int) {
		mock := newCloseBarrierMockServer(t)
		client := newCloseBarrierTestClient(t, mock.server.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		streamErr := make(chan error, 1)
		go func() {
			_, err := client.StreamWorkspace(ctx, WorkspaceStreamCallbacks{})
			streamErr <- err
		}()

		// Wait until the server side has observed the upgrade — at this point
		// the client has either just unlocked after publishing the stream or
		// is about to. Firing Close immediately maximises the chance of
		// landing in the Add(2)-vs-Wait window.
		select {
		case <-mock.connected:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: mock never observed WS upgrade", i)
		}

		closeDone := make(chan struct{})
		go func() {
			client.Close()
			close(closeDone)
		}()

		select {
		case <-closeDone:
		case <-time.After(3 * time.Second):
			t.Fatalf("iteration %d: Close did not return within 3s", i)
		}
		<-streamErr
	}
	for i := 0; i < iterations; i++ {
		runIter(i)
	}
}
