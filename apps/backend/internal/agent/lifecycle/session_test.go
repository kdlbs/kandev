package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newSessionTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// mockAgentServer creates a test WebSocket server simulating agentctl.
// It responds to agent stream requests and tracks which actions were called and in what order.
type mockAgentServer struct {
	server       *httptest.Server
	mu           sync.Mutex
	actionLog   []string // ordered log of actions received
	upgrader    websocket.Upgrader
	handler     func(msg ws.Message) *ws.Message
	wsConnected chan struct{} // closed when WS stream connects
}

func newMockAgentServer(t *testing.T) *mockAgentServer {
	t.Helper()
	m := &mockAgentServer{
		wsConnected: make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()

	// Agent stream WebSocket endpoint
	mux.HandleFunc("/api/v1/agent/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := m.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// Signal that WS is connected
		select {
		case <-m.wsConnected:
		default:
			close(m.wsConnected)
		}

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
				m.mu.Lock()
				m.actionLog = append(m.actionLog, msg.Action)
				m.mu.Unlock()

				var resp *ws.Message
				if m.handler != nil {
					resp = m.handler(msg)
				} else {
					resp = m.defaultHandler(msg)
				}

				if resp != nil {
					data, _ := json.Marshal(resp)
					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						return
					}
				}
			}
		}
	})

	// Workspace stream WebSocket endpoint (required by ConnectAll)
	mux.HandleFunc("/api/v1/workspace/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := m.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// Send connected message
		connMsg := map[string]string{"type": "connected"}
		data, _ := json.Marshal(connMsg)
		_ = conn.WriteMessage(websocket.TextMessage, data)

		// Keep alive
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockAgentServer) defaultHandler(msg ws.Message) *ws.Message {
	switch msg.Action {
	case "agent.initialize":
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
			"agent_info": map[string]string{
				"name":    "test-agent",
				"version": "1.0.0",
			},
		})
		return resp
	case "agent.session.new":
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success":    true,
			"session_id": "test-session-123",
		})
		return resp
	case "agent.session.load":
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success":    true,
			"session_id": "loaded-session",
		})
		return resp
	case "agent.prompt":
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"success": true,
		})
		return resp
	default:
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "unknown action", nil)
		return resp
	}
}

func (m *mockAgentServer) getActionLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.actionLog))
	copy(result, m.actionLog)
	return result
}

func (m *mockAgentServer) Close() {
	m.server.Close()
}

// createTestClient creates an agentctl.Client pointed at the mock server.
func createTestClient(t *testing.T, serverURL string) *agentctl.Client {
	t.Helper()
	// Parse the test server URL (http://127.0.0.1:PORT)
	url := strings.TrimPrefix(serverURL, "http://")
	parts := strings.Split(url, ":")
	host := parts[0]
	var port int
	_, _ = fmt.Sscanf(parts[1], "%d", &port)

	log := newSessionTestLogger()
	return agentctl.NewClient(host, port, log)
}

// --- Tests ---

func TestInitializeAndPrompt_StreamBeforeInitialize(t *testing.T) {
	// This test verifies the critical ordering: stream connects BEFORE initialize is called.
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	// Set up real stream manager with callbacks
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil)
	sm.SetDependencies(nil, streamMgr, nil, nil)

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
		SessionConfig: registry.SessionConfig{
			NativeSessionResume: false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, func(executionID string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("InitializeAndPrompt failed: %v", err)
	}

	// Wait for stream to connect (it should have connected before initialize)
	select {
	case <-mock.wsConnected:
	case <-time.After(5 * time.Second):
		t.Fatal("stream never connected")
	}

	// Verify the ordering: initialize and session.new should have been called
	// (the stream was connected first, then these went through it)
	actions := mock.getActionLog()
	if len(actions) < 2 {
		t.Fatalf("expected at least 2 actions, got %d: %v", len(actions), actions)
	}
	if actions[0] != "agent.initialize" {
		t.Errorf("expected first action to be 'agent.initialize', got %q", actions[0])
	}
	if actions[1] != "agent.session.new" {
		t.Errorf("expected second action to be 'agent.session.new', got %q", actions[1])
	}

	// The session ID should be set
	if execution.ACPSessionID != "test-session-123" {
		t.Errorf("expected ACPSessionID 'test-session-123', got %q", execution.ACPSessionID)
	}
}

func TestInitializeAndPrompt_StreamTimeout(t *testing.T) {
	// This test verifies that InitializeAndPrompt returns an error if
	// the stream fails to connect within the timeout.
	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	// Create a stream manager that will try to connect to a server that doesn't exist
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil)
	sm.SetDependencies(nil, streamMgr, nil, nil)

	// Point client at a port that doesn't exist
	badClient := agentctl.NewClient("127.0.0.1", 1, log)
	defer badClient.Close()

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      badClient,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
	}

	// Use short timeout so test doesn't take 10s
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, func(executionID string) error {
		return nil
	})

	// Should fail because stream couldn't connect and Initialize fails
	if err == nil {
		t.Fatal("expected error when stream cannot connect")
	}
	// The error could be either timeout waiting for stream or initialize failure
	// (since stream failed, initialize over WS will also fail)
}

func TestInitializeAndPrompt_WithTaskDescription(t *testing.T) {
	// Test that when a task description is provided, prompt is sent after initialization.
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil)
	sm.SetDependencies(nil, streamMgr, nil, nil)

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
		SessionConfig: registry.SessionConfig{
			NativeSessionResume: false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "Build a feature", nil, func(executionID string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("InitializeAndPrompt failed: %v", err)
	}

	// Wait for the prompt to be sent asynchronously
	time.Sleep(500 * time.Millisecond)

	actions := mock.getActionLog()

	// Should have: initialize, session.new, prompt
	if len(actions) < 3 {
		t.Fatalf("expected at least 3 actions (initialize, session.new, prompt), got %d: %v", len(actions), actions)
	}
	if actions[0] != "agent.initialize" {
		t.Errorf("expected first action 'agent.initialize', got %q", actions[0])
	}
	if actions[1] != "agent.session.new" {
		t.Errorf("expected second action 'agent.session.new', got %q", actions[1])
	}
	if actions[2] != "agent.prompt" {
		t.Errorf("expected third action 'agent.prompt', got %q", actions[2])
	}
}

func TestInitializeAndPrompt_NoStreamManager(t *testing.T) {
	// When streamManager is nil, InitializeAndPrompt should still work
	// (though in practice this means sendStreamRequest will fail).
	// This tests that the nil guard for streamManager works.
	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))
	// No dependencies set — streamManager is nil

	// We can't actually call InitializeAndPrompt without a stream because
	// the client's sendStreamRequest will fail. But we can verify the code path
	// doesn't panic when streamManager is nil.
	mock := newMockAgentServer(t)
	defer mock.Close()

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This will fail because the stream isn't connected (sendStreamRequest returns error).
	// But it should NOT panic due to nil streamManager.
	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, func(executionID string) error {
		return nil
	})

	// Expect error because Initialize call over WS will fail (stream not connected)
	if err == nil {
		t.Fatal("expected error when stream is not connected")
	}
}

func TestInitializeSession_CreatesNewSession(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	// Create client and manually connect stream
	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	ctx := context.Background()
	err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil)
	if err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let goroutine start

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
		SessionConfig: registry.SessionConfig{
			NativeSessionResume: false,
		},
	}

	result, err := sm.InitializeSession(ctx, client, agentConfig, "", "/workspace", nil)
	if err != nil {
		t.Fatalf("InitializeSession failed: %v", err)
	}

	if result.AgentName != "test-agent" {
		t.Errorf("expected agent name 'test-agent', got %q", result.AgentName)
	}
	if result.SessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", result.SessionID)
	}
}

func TestInitializeSession_LoadsExistingSession(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	ctx := context.Background()
	err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil)
	if err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	agentConfig := &registry.AgentTypeConfig{
		ID:       "test-agent",
		Protocol: agent.ProtocolACP,
		SessionConfig: registry.SessionConfig{
			NativeSessionResume: true,
		},
	}

	result, err := sm.InitializeSession(ctx, client, agentConfig, "existing-session", "/workspace", nil)
	if err != nil {
		t.Fatalf("InitializeSession failed: %v", err)
	}

	if result.SessionID != "existing-session" {
		t.Errorf("expected session ID 'existing-session', got %q", result.SessionID)
	}

	// Verify that session.load was called (not session.new)
	actions := mock.getActionLog()
	hasLoad := false
	hasNew := false
	for _, a := range actions {
		if a == "agent.session.load" {
			hasLoad = true
		}
		if a == "agent.session.new" {
			hasNew = true
		}
	}
	if !hasLoad {
		t.Error("expected agent.session.load to be called")
	}
	if hasNew {
		t.Error("did not expect agent.session.new to be called when loading existing session")
	}
}
