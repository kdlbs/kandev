package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type streamContextMCPHandler struct {
	observed chan error
}

func (h streamContextMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	h.observed <- ctx.Err()
	return &ws.Message{ID: msg.ID, Type: ws.MessageTypeResponse, Action: msg.Action}, nil
}

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

func TestCloseCancelsPendingUpdatesDialAndAllowsReconnect(t *testing.T) {
	for _, tc := range []struct {
		name  string
		close func(*Client)
	}{
		{name: "close updates stream", close: func(client *Client) { client.CloseUpdatesStream() }},
		{name: "close client", close: func(client *Client) { client.Close() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialStarted := make(chan struct{})
			requestCanceled := make(chan struct{})
			releaseFirst := make(chan struct{})
			upgraded := make(chan struct{}, 1)
			mcpContextErr := make(chan error, 1)
			var releaseOnce sync.Once
			var requests atomic.Int32
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					close(dialStarted)
					select {
					case <-r.Context().Done():
						close(requestCanceled)
					case <-releaseFirst:
					}
					return
				}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				upgraded <- struct{}{}
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"live-context","type":"request","action":"mcp.context","payload":{}}`))
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						_ = conn.Close()
						return
					}
				}
			}))
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseFirst) })
				server.Close()
			})
			client := newCloseBarrierTestClient(t, server.URL)
			t.Cleanup(client.Close)
			mcpHandler := streamContextMCPHandler{observed: mcpContextErr}

			streamResult := make(chan error, 1)
			go func() {
				streamResult <- client.StreamUpdates(context.Background(), func(AgentEvent) {}, mcpHandler, nil)
			}()
			select {
			case <-dialStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("server did not receive the pending updates-stream dial")
			}

			tc.close(client)
			select {
			case <-requestCanceled:
			case <-time.After(2 * time.Second):
				t.Fatal("closing the updates stream did not cancel the pending HTTP upgrade")
			}
			select {
			case err := <-streamResult:
				if err == nil {
					t.Fatal("StreamUpdates succeeded after its pending dial was closed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("pending StreamUpdates dial did not stop after close")
			}

			if err := client.StreamUpdates(context.Background(), func(AgentEvent) {}, mcpHandler, nil); err != nil {
				t.Fatalf("StreamUpdates should reconnect after close: %v", err)
			}
			select {
			case <-upgraded:
			case <-time.After(2 * time.Second):
				t.Fatal("fresh updates-stream dial did not upgrade")
			}
			if !client.HasAgentStream() {
				t.Fatal("fresh updates stream was not registered")
			}
			select {
			case err := <-mcpContextErr:
				if err != nil {
					t.Fatalf("MCP handler received canceled stream context after reconnect: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("fresh updates stream did not dispatch MCP request")
			}
			client.CloseUpdatesStream()
		})
	}
}

func TestStaleUpdatesDialCompletionCannotReplaceSuccessor(t *testing.T) {
	server := newWSTestServer(t, func(msg ws.Message) *ws.Message {
		response, err := ws.NewResponse(msg.ID, msg.Action, map[string]bool{"ok": true})
		if err != nil {
			t.Errorf("build response: %v", err)
			return nil
		}
		return response
	})
	defer server.Close()
	client := newCloseBarrierTestClient(t, server.server.URL)
	t.Cleanup(client.Close)

	staleAttempt, _, err := client.beginUpdatesStreamDial(context.Background())
	if err != nil {
		t.Fatalf("begin stale dial: %v", err)
	}
	if err := client.StreamUpdates(context.Background(), func(AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("install successor updates stream: %v", err)
	}
	client.mu.RLock()
	successor := client.agentStreamConn
	client.mu.RUnlock()
	if successor == nil {
		t.Fatal("successor updates stream was not installed")
	}

	lateConn, _, err := client.dialWebSocket(context.Background(), "/api/v1/agent/stream", nil)
	if err != nil {
		t.Fatalf("open simulated late handshake result: %v", err)
	}
	if _, err := client.completeUpdatesStreamDial(staleAttempt, lateConn); err == nil {
		t.Fatal("stale dial completion unexpectedly succeeded")
	}
	if _, _, err := lateConn.ReadMessage(); err == nil {
		t.Fatal("stale dial completion left its late socket open")
	}
	client.mu.RLock()
	stillCurrent := client.agentStreamConn == successor
	client.mu.RUnlock()
	if !stillCurrent {
		t.Fatal("stale dial completion replaced the successor stream")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := client.sendStreamRequest(ctx, "test.successor", nil)
	if err != nil {
		t.Fatalf("successor stream stopped serving requests after stale completion: %v", err)
	}
	if response.Type != ws.MessageTypeResponse {
		t.Fatalf("successor response type = %q, want response", response.Type)
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
