package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// wsTestServer creates a test WebSocket server that echoes back response messages.
// The handler receives each request and should return a response message.
type wsTestServer struct {
	server  *httptest.Server
	handler func(msg ws.Message) *ws.Message
}

func newWSTestServer(t *testing.T, handler func(msg ws.Message) *ws.Message) *wsTestServer {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	ts := &wsTestServer{handler: handler}

	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var msg ws.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			if msg.Type == ws.MessageTypeRequest && ts.handler != nil {
				resp := ts.handler(msg)
				if resp != nil {
					data, _ := json.Marshal(resp)
					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						return
					}
				}
			}
		}
	}))

	return ts
}

func (ts *wsTestServer) Close() {
	ts.server.Close()
}

// newTestClientWithStream creates a Client connected to a test WebSocket server.
// The handler processes incoming requests and returns responses.
func newTestClientWithStream(t *testing.T, handler func(msg ws.Message) *ws.Message) (*Client, *wsTestServer) {
	t.Helper()

	ts := newWSTestServer(t, handler)

	// Parse the test server URL to get host and port
	// httptest URL is like http://127.0.0.1:PORT
	url := ts.server.URL
	log := newTestLogger()

	// Create client with the test server URL
	c := &Client{
		baseURL:         url,
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	// Connect the stream
	ctx := context.Background()
	err := c.StreamUpdates(ctx, func(event AgentEvent) {
		// no-op handler for agent events
	}, nil, nil)
	if err != nil {
		ts.Close()
		t.Fatalf("failed to connect stream: %v", err)
	}

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	return c, ts
}

// --- Tests for infrastructure methods ---

func TestSendStreamRequest_NotConnected(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		baseURL:         "http://localhost:0",
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	ctx := context.Background()
	_, err := c.sendStreamRequest(ctx, "test.action", nil)
	if err == nil {
		t.Fatal("expected error when stream not connected")
	}
	if !strings.Contains(err.Error(), "agent stream not connected") {
		t.Fatalf("expected 'agent stream not connected' error, got: %v", err)
	}
}

func TestSendStreamRequest_ContextCancelled(t *testing.T) {
	// Server that never responds
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		return nil // don't respond
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.sendStreamRequest(ctx, "test.action", nil)
	if err == nil {
		t.Fatal("expected error on timeout")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected deadline exceeded error, got: %v", err)
	}
}

func TestResolvePendingRequest_NoMatch(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	msg := &ws.Message{ID: "nonexistent", Type: ws.MessageTypeResponse}
	if c.resolvePendingRequest(msg) {
		t.Fatal("expected false for unmatched request")
	}
}

func TestResolvePendingRequest_EmptyID(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	msg := &ws.Message{ID: "", Type: ws.MessageTypeResponse}
	if c.resolvePendingRequest(msg) {
		t.Fatal("expected false for empty ID")
	}
}

func TestResolvePendingRequest_Match(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	ch := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pendingRequests["test-id"] = ch
	c.pendingMu.Unlock()

	msg := &ws.Message{ID: "test-id", Type: ws.MessageTypeResponse, Action: "test.action"}
	if !c.resolvePendingRequest(msg) {
		t.Fatal("expected true for matched request")
	}

	select {
	case resp := <-ch:
		if resp.ID != "test-id" {
			t.Fatalf("expected response ID 'test-id', got %q", resp.ID)
		}
	default:
		t.Fatal("expected response in channel")
	}
}

func TestCleanupPendingRequests(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	ch1 := make(chan *ws.Message, 1)
	ch2 := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pendingRequests["req-1"] = ch1
	c.pendingRequests["req-2"] = ch2
	c.pendingMu.Unlock()

	c.cleanupPendingRequests()

	c.pendingMu.Lock()
	count := len(c.pendingRequests)
	c.pendingMu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 pending requests after cleanup, got %d", count)
	}

	// Channels should be closed
	_, ok := <-ch1
	if ok {
		t.Fatal("expected ch1 to be closed")
	}
	_, ok = <-ch2
	if ok {
		t.Fatal("expected ch2 to be closed")
	}
}

func TestCleanupPendingRequests_ConcurrentSafe(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	// Add many pending requests
	for i := 0; i < 50; i++ {
		ch := make(chan *ws.Message, 1)
		c.pendingMu.Lock()
		c.pendingRequests[strings.Repeat("x", i+1)] = ch
		c.pendingMu.Unlock()
	}

	// Cleanup concurrently
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.cleanupPendingRequests()
	}()
	wg.Wait()

	c.pendingMu.Lock()
	count := len(c.pendingRequests)
	c.pendingMu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 pending requests, got %d", count)
	}
}

// --- Tests for the 7 migrated methods ---

func TestInitialize_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.initialize" {
			t.Errorf("expected action 'agent.initialize', got %q", msg.Action)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
			"agent_info": map[string]string{
				"name":    "test-agent",
				"version": "1.0.0",
			},
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := c.Initialize(ctx, "kandev", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected agent info, got nil")
	} else {
		if info.Name != "test-agent" {
			t.Errorf("expected name 'test-agent', got %q", info.Name)
		}
		if info.Version != "1.0.0" {
			t.Errorf("expected version '1.0.0', got %q", info.Version)
		}
	}
}

func TestInitialize_ServerError(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Initialize(ctx, "kandev", "1.0.0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent not running") {
		t.Fatalf("expected 'agent not running' error, got: %v", err)
	}
}

func TestInitialize_SuccessFalse(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": false,
			"error":   "init failed internally",
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Initialize(ctx, "kandev", "1.0.0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "init failed internally") {
		t.Fatalf("expected 'init failed internally' error, got: %v", err)
	}
}

func TestNewSession_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.session.new" {
			t.Errorf("expected action 'agent.session.new', got %q", msg.Action)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success":    true,
			"session_id": "sess-123",
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionID, err := c.NewSession(ctx, "/workspace", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID != "sess-123" {
		t.Errorf("expected session ID 'sess-123', got %q", sessionID)
	}
}

func TestNewSession_Error(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.NewSession(ctx, "/workspace", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent not running") {
		t.Fatalf("expected 'agent not running' error, got: %v", err)
	}
}

func TestLoadSession_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.session.load" {
			t.Errorf("expected action 'agent.session.load', got %q", msg.Action)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success":    true,
			"session_id": "sess-456",
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.LoadSession(ctx, "sess-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSession_Error(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Method not found", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.LoadSession(ctx, "sess-456")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Method not found") {
		t.Fatalf("expected 'Method not found' error, got: %v", err)
	}
}

func TestPrompt_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.prompt" {
			t.Errorf("expected action 'agent.prompt', got %q", msg.Action)
		}
		// Verify payload
		var payload struct {
			Text string `json:"text"`
		}
		if err := msg.ParsePayload(&payload); err != nil {
			t.Errorf("failed to parse payload: %v", err)
		}
		if payload.Text != "hello agent" {
			t.Errorf("expected text 'hello agent', got %q", payload.Text)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Prompt(ctx, "hello agent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrompt_NoActiveSession(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "no active session - call new_session first", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Prompt(ctx, "hello agent", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("expected 'no active session' error, got: %v", err)
	}
}

func TestCancel_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.cancel" {
			t.Errorf("expected action 'agent.cancel', got %q", msg.Action)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Cancel(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancel_Error(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Cancel(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent not running") {
		t.Fatalf("expected 'agent not running' error, got: %v", err)
	}
}

func TestGetAgentStderr_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.stderr" {
			t.Errorf("expected action 'agent.stderr', got %q", msg.Action)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"lines": []string{"error line 1", "error line 2"},
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lines, err := c.GetAgentStderr(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "error line 1" {
		t.Errorf("expected 'error line 1', got %q", lines[0])
	}
}

func TestGetAgentStderr_NotConnected(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		baseURL:         "http://localhost:0",
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	ctx := context.Background()
	_, err := c.GetAgentStderr(ctx)
	if err == nil {
		t.Fatal("expected error when stream not connected")
	}
	if !strings.Contains(err.Error(), "agent stream not connected") {
		t.Fatalf("expected 'agent stream not connected' error, got: %v", err)
	}
}

func TestRespondToPermission_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.permissions.respond" {
			t.Errorf("expected action 'agent.permissions.respond', got %q", msg.Action)
		}
		// Verify payload
		var payload struct {
			PendingID string `json:"pending_id"`
			OptionID  string `json:"option_id"`
			Cancelled bool   `json:"cancelled"`
		}
		if err := msg.ParsePayload(&payload); err != nil {
			t.Errorf("failed to parse payload: %v", err)
		}
		if payload.PendingID != "perm-1" {
			t.Errorf("expected pending_id 'perm-1', got %q", payload.PendingID)
		}
		if payload.OptionID != "allow" {
			t.Errorf("expected option_id 'allow', got %q", payload.OptionID)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.RespondToPermission(ctx, "perm-1", "allow", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRespondToPermission_NotFound(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "permission request not found", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.RespondToPermission(ctx, "nonexistent", "allow", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission request not found") {
		t.Fatalf("expected 'permission request not found' error, got: %v", err)
	}
}

// --- Tests for StreamUpdates response routing ---

func TestStreamUpdates_RoutesResponsesToPending(t *testing.T) {
	// This test verifies that StreamUpdates correctly routes response messages
	// to pending requests instead of treating them as agent events.
	var receivedEvents []AgentEvent
	var mu sync.Mutex

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg ws.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}
			if msg.Type == ws.MessageTypeRequest {
				// Send response back
				resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
					"success": true,
				})
				data, _ := json.Marshal(resp)
				_ = conn.WriteMessage(websocket.TextMessage, data)

				// Also send an agent event
				event := AgentEvent{Type: "message_chunk", Text: "hello"}
				eventData, _ := json.Marshal(event)
				_ = conn.WriteMessage(websocket.TextMessage, eventData)
			}
		}
	}))
	defer server.Close()

	log := newTestLogger()
	c := &Client{
		baseURL:         server.URL,
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	ctx := context.Background()
	err := c.StreamUpdates(ctx, func(event AgentEvent) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Send a request and get a response
	reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := c.sendStreamRequest(reqCtx, "test.action", nil)
	if err != nil {
		t.Fatalf("sendStreamRequest failed: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Errorf("expected response type, got %q", resp.Type)
	}

	// Wait for the agent event to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventCount := len(receivedEvents)
	mu.Unlock()

	if eventCount != 1 {
		t.Errorf("expected 1 agent event, got %d", eventCount)
	}

	c.Close()
}

func TestStreamUpdates_DisconnectCleansPending(t *testing.T) {
	// This test verifies that pending requests are cleaned up on stream disconnect.
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Close immediately to trigger disconnect
		time.Sleep(100 * time.Millisecond)
		_ = conn.Close()
	}))
	defer server.Close()

	log := newTestLogger()
	c := &Client{
		baseURL:         server.URL,
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	disconnected := make(chan struct{})
	ctx := context.Background()
	err := c.StreamUpdates(ctx, func(event AgentEvent) {}, nil, func(err error) {
		close(disconnected)
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Add a pending request
	ch := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	c.pendingRequests["pending-test"] = ch
	c.pendingMu.Unlock()

	// Wait for disconnect
	select {
	case <-disconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for disconnect")
	}

	// The channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after disconnect")
	}

	c.pendingMu.Lock()
	count := len(c.pendingRequests)
	c.pendingMu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 pending requests after disconnect, got %d", count)
	}
}

// --- Concurrent request tests ---

func TestConcurrentRequests(t *testing.T) {
	// Multiple concurrent requests should all resolve correctly.
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
			"action":  msg.Action,
		})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := c.sendStreamRequest(ctx, "test.concurrent", nil)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}
