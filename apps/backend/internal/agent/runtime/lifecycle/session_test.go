package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newSessionTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// newTestStopCh returns a stopCh shared with t.Cleanup so any background
// StreamManager retry loop that races past the test body's return drains
// cleanly. Closing on cleanup is what lets goleak.VerifyTestMain stay green
// for tests that exercise connectWorkspaceStream's backoff.
func newTestStopCh(t *testing.T) chan struct{} {
	t.Helper()
	stopCh := make(chan struct{})
	t.Cleanup(func() { closeStopChOnce(stopCh) })
	return stopCh
}

// mockAgentServer creates a test WebSocket server simulating agentctl.
// It responds to agent stream requests and tracks which actions were called and in what order.
type mockAgentServer struct {
	server               *httptest.Server
	mu                   sync.Mutex
	actionLog            []string // ordered log of actions received
	rejectStreamAttempts int
	agentStatus          string
	upgrader             websocket.Upgrader
	handler              func(msg ws.Message) *ws.Message
	wsConnected          chan struct{} // closed when WS stream connects
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
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		m.mu.Lock()
		status := m.agentStatus
		m.mu.Unlock()
		if status == "" {
			status = "running"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"agent_status":%q}`, status)
	})

	// Agent stream WebSocket endpoint
	mux.HandleFunc("/api/v1/agent/stream", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		if m.rejectStreamAttempts > 0 {
			m.rejectStreamAttempts--
			m.mu.Unlock()
			http.Error(w, "stream temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		m.mu.Unlock()

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

			if msg.Type != ws.MessageTypeRequest {
				continue
			}

			m.mu.Lock()
			m.actionLog = append(m.actionLog, msg.Action)
			m.mu.Unlock()

			resp := m.buildResponse(msg)
			if resp == nil {
				continue
			}
			data, _ := json.Marshal(resp)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
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

// buildResponse returns the handler or default response for a request message.
func (m *mockAgentServer) buildResponse(msg ws.Message) *ws.Message {
	if m.handler != nil {
		return m.handler(msg)
	}
	return m.defaultHandler(msg)
}

func (m *mockAgentServer) rejectNextStreamAttempts(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rejectStreamAttempts = count
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
	case "agent.session.set_model", "agent.session.set_mode", "agent.session.set_config_option":
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
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)

	// Set up real stream manager with callbacks
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
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

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("test-agent"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: false,
			},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, nil, func(executionID string) error {
		return nil
	}, "", "", nil)
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

func TestInitializeAndPrompt_AppliesProfileConfigOptions(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
	eventBus := &MockEventBusWithTracking{}
	sm.SetDependencies(NewEventPublisher(eventBus, log), streamMgr, nil, nil)

	client := createTestClient(t, mock.server.URL)
	defer client.Close()
	execution := &AgentExecution{
		ID:            "exec-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	execution.SetModelState(&CachedModelState{
		CurrentModelID: "default-model",
		ConfigOptions: []streams.ConfigOption{
			{
				ID: "model", Category: "model", CurrentValue: "default-model",
				Options: []streams.ConfigOptionValue{{Value: "stale-cached-model"}},
			},
			{ID: "effort", CurrentValue: "medium"},
			{ID: "fast_mode", CurrentValue: "off"},
		},
	})
	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:            agents.NewCommand("test-agent"),
			Protocol:       agent.ProtocolACP,
			SessionConfig:  agents.SessionConfig{},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, nil, func(executionID string) error {
		return nil
	}, "sonnet", "plan", map[string]string{
		"effort": "high",
		"model":  "ignored",
		"mode":   "ignored",
	})
	if err != nil {
		t.Fatalf("InitializeAndPrompt failed: %v", err)
	}

	actions := mock.getActionLog()
	for _, want := range []string{
		"agent.session.set_model",
		"agent.session.set_mode",
		"agent.session.set_config_option",
	} {
		if !containsAction(actions, want) {
			t.Fatalf("actions = %v, missing %s", actions, want)
		}
	}

	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && settledConfigEventData(event.Data.Data) {
			t.Fatal("startup requests without an authoritative config snapshot must not be marked settled")
		}
	}
}

func settledConfigEventData(data any) bool {
	metadata, _ := data.(map[string]any)
	settled, _ := metadata["config_options_settled"].(bool)
	return settled
}

func configValueByID(options []streams.ConfigOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.CurrentValue
		}
	}
	return ""
}

func TestPublishSettledConfigOptionsAppliesToDelayedModelState(t *testing.T) {
	log := newSessionTestLogger()
	eventBus := &MockEventBusWithTracking{}
	publisher := NewEventPublisher(eventBus, log)
	sm := NewSessionManager(log, nil)
	sm.SetDependencies(publisher, nil, nil, nil)
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}

	sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", nil)
	mgr := &Manager{logger: log, eventPublisher: publisher}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      "acp-session-1",
		CurrentModelID: "sonnet",
		ConfigOptions: []streams.ConfigOption{
			{ID: "model", Category: "model", CurrentValue: "sonnet"},
			{ID: "effort", CurrentValue: "high"},
		},
		Data: map[string]any{
			"config_options_source":    "provider_response",
			"config_options_config_id": "effort",
		},
	})

	var settled *AgentStreamEventData
	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && settledConfigEventData(event.Data.Data) {
			settled = event.Data
		}
	}
	if settled == nil {
		t.Fatal("expected delayed model state to carry settlement marker")
	}
	if got := configValueByID(settled.ConfigOptions, "model"); got != "sonnet" {
		t.Errorf("settled model = %q, want sonnet", got)
	}
	if got := configValueByID(settled.ConfigOptions, "effort"); got != "high" {
		t.Errorf("settled effort = %q, want high", got)
	}
}

func TestConfigBaselineRetainsProviderDefaultsWhenProfileOverrideSettles(t *testing.T) {
	log := newSessionTestLogger()
	eventBus := &MockEventBusWithTracking{}
	publisher := NewEventPublisher(eventBus, log)
	sm := NewSessionManager(log, nil)
	sm.SetDependencies(publisher, nil, nil, nil)
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}
	execution.SetModelState(&CachedModelState{
		CurrentModelID: "gpt-5",
		ConfigOptions: []streams.ConfigOption{
			{ID: "model", Category: "model", CurrentValue: "gpt-5"},
			{ID: "effort", CurrentValue: "medium"},
			{ID: "fast_mode", CurrentValue: "off"},
		},
	})
	providerDefaults := execution.GetModelState()

	// Startup requested effort=high, but the stable ACP response's complete
	// configOptions reports the provider's dependent final state. Stream
	// dispatch is deliberately delayed until after the startup RPC boundary.
	sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", providerDefaults)
	mgr := &Manager{logger: log, eventPublisher: publisher}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      "acp-session-1",
		CurrentModelID: "gpt-5",
		ConfigOptions: []streams.ConfigOption{
			{ID: "model", Category: "model", CurrentValue: "gpt-5"},
			{ID: "effort", CurrentValue: "low"},
			{ID: "fast_mode", CurrentValue: "on"},
		},
		Data: map[string]any{
			"config_options_source":    "provider_response",
			"config_options_config_id": "effort",
		},
	})

	var settledEvents []*AgentStreamEventData
	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && settledConfigEventData(event.Data.Data) {
			settledEvents = append(settledEvents, event.Data)
		}
	}
	if len(settledEvents) != 1 {
		t.Fatalf("settled event count = %d, want 1 authoritative provider state", len(settledEvents))
	}
	settled := settledEvents[0]
	if got := configValueByID(settled.ConfigOptions, "effort"); got != "low" {
		t.Errorf("settled effort = %q, want provider-reported low", got)
	}
	if got := configValueByID(settled.ConfigOptions, "fast_mode"); got != "on" {
		t.Errorf("settled fast_mode = %q, want provider-reported dependent on", got)
	}
	if got := configValueByID(settled.ConfigBaselineCandidate, "effort"); got != "medium" {
		t.Errorf("baseline effort = %q, want provider default medium", got)
	}
	if got := configValueByID(settled.ConfigBaselineCandidate, "fast_mode"); got != "off" {
		t.Errorf("baseline fast_mode = %q, want provider default off", got)
	}
}

func TestConfigSettlementAuthoritativeResponseOrdering(t *testing.T) {
	newHarness := func() (*SessionManager, *Manager, *AgentExecution, *MockEventBusWithTracking) {
		log := newSessionTestLogger()
		eventBus := &MockEventBusWithTracking{}
		publisher := NewEventPublisher(eventBus, log)
		sm := NewSessionManager(log, nil)
		sm.SetDependencies(publisher, nil, nil, nil)
		return sm, &Manager{logger: log, eventPublisher: publisher}, &AgentExecution{
			ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		}, eventBus
	}
	event := func(source, configID, effort, fastMode string) agentctl.AgentEvent {
		return agentctl.AgentEvent{
			Type:           streams.EventTypeSessionModels,
			SessionID:      "acp-session-1",
			CurrentModelID: "gpt-5",
			ConfigOptions: []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "gpt-5"},
				{ID: "effort", CurrentValue: effort},
				{ID: "fast_mode", CurrentValue: fastMode},
			},
			Data: map[string]any{
				"config_options_source":    source,
				"config_options_config_id": configID,
			},
		}
	}
	settledEvents := func(eventBus *MockEventBusWithTracking) []*AgentStreamEventData {
		var settled []*AgentStreamEventData
		for _, published := range eventBus.getStreamEvents() {
			if published.Data != nil && settledConfigEventData(published.Data.Data) {
				settled = append(settled, published.Data)
			}
		}
		return settled
	}

	t.Run("old provider update then settle then matching response", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		mgr.handleAgentEvent(execution, event("provider_update", "", "medium", "off"))
		providerDefaults := execution.GetModelState()
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", providerDefaults)
		mgr.handleAgentEvent(execution, event("provider_response", "effort", "low", "on"))

		settled := settledEvents(eventBus)
		if len(settled) != 1 || configValueByID(settled[0].ConfigOptions, "effort") != "low" {
			t.Fatalf("settled events = %#v, want matching response effort=low", settled)
		}
	})

	t.Run("matching response then newer provider update then settle", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		mgr.handleAgentEvent(execution, event("provider_response", "effort", "high", "off"))
		mgr.handleAgentEvent(execution, event("provider_update", "", "low", "on"))
		providerDefaults := &CachedModelState{ConfigOptions: []streams.ConfigOption{
			{ID: "model", Category: "model", CurrentValue: "gpt-5"},
			{ID: "effort", CurrentValue: "medium"},
			{ID: "fast_mode", CurrentValue: "off"},
		}}
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", providerDefaults)

		settled := settledEvents(eventBus)
		if len(settled) != 1 {
			t.Fatalf("settled events = %#v, want one event", settled)
		}
		if got := configValueByID(settled[0].ConfigOptions, "effort"); got != "low" {
			t.Fatalf("published live effort = %q, want newest provider update low", got)
		}
		if got := configValueByID(settled[0].ConfigOptions, "fast_mode"); got != "on" {
			t.Fatalf("published live fast mode = %q, want newest provider update on", got)
		}
		if got := configValueByID(settled[0].ConfigBaselineCandidate, "effort"); got != "medium" {
			t.Fatalf("baseline candidate effort = %q, want provider default medium", got)
		}
		if got := configValueByID(settled[0].ConfigBaselineCandidate, "fast_mode"); got != "off" {
			t.Fatalf("baseline candidate fast mode = %q, want retained response off", got)
		}
		if got := configValueByID(execution.GetModelState().ConfigOptions, "effort"); got != "low" {
			t.Fatalf("live effort = %q, want newest provider update low", got)
		}
	})

	t.Run("pending settlement ignores unrelated provider update", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", nil)
		mgr.handleAgentEvent(execution, event("provider_update", "", "medium", "off"))

		if settled := settledEvents(eventBus); len(settled) != 0 {
			t.Fatalf("provider update settled pending response: %#v", settled)
		}
		if got := configValueByID(execution.GetModelState().ConfigOptions, "effort"); got != "medium" {
			t.Fatalf("live effort = %q, want provider update medium", got)
		}
	})

	t.Run("queued initial state remains baseline before matching profile response", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", nil)
		mgr.handleAgentEvent(execution, event("provider_update", "", "medium", "off"))
		mgr.handleAgentEvent(execution, event("provider_response", "effort", "high", "off"))

		settled := settledEvents(eventBus)
		if len(settled) != 1 {
			t.Fatalf("settled events = %#v, want one event", settled)
		}
		if got := configValueByID(settled[0].ConfigBaselineCandidate, "effort"); got != "medium" {
			t.Fatalf("baseline effort = %q, want queued provider default medium", got)
		}
		if got := configValueByID(settled[0].ConfigOptions, "effort"); got != "high" {
			t.Fatalf("live effort = %q, want profile-selected high", got)
		}
	})

	t.Run("first provider update settles empty startup config", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		sm.publishSettledConfigOptions(execution, "acp-session-1", "", nil)
		mgr.handleAgentEvent(execution, event("provider_update", "", "medium", "off"))

		settled := settledEvents(eventBus)
		if len(settled) != 1 {
			t.Fatalf("settled events = %#v, want first provider update to settle", settled)
		}
		if got := configValueByID(settled[0].ConfigBaselineCandidate, "effort"); got != "medium" {
			t.Fatalf("baseline candidate effort = %q, want medium", got)
		}
	})

	t.Run("matching response is consumed once", func(t *testing.T) {
		sm, mgr, execution, eventBus := newHarness()
		mgr.handleAgentEvent(execution, event("provider_response", "effort", "low", "on"))
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", nil)
		sm.publishSettledConfigOptions(execution, "acp-session-1", "effort", nil)

		if settled := settledEvents(eventBus); len(settled) != 1 {
			t.Fatalf("settled event count = %d, want response consumed once", len(settled))
		}
	})
}

func containsAction(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestInitializeAndPrompt_StreamTimeout(t *testing.T) {
	// This test verifies that InitializeAndPrompt returns an error if
	// the stream fails to connect within the timeout.
	log := newSessionTestLogger()
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)

	// Create a stream manager that will try to connect to a server that doesn't exist
	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
	sm.SetDependencies(nil, streamMgr, nil, nil)

	// Bind to a random port and immediately close it so the port is guaranteed
	// to be closed and returns connection refused quickly on every system.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if cerr := ln.Close(); cerr != nil {
		t.Fatalf("failed to close listener: %v", cerr)
	}
	badClient := agentctl.NewClient("127.0.0.1", port, log)
	defer badClient.Close()

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      badClient,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:            agents.NewCommand("test-agent"),
			Protocol:       agent.ProtocolACP,
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	// Use short timeout so test doesn't take 10s
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, nil, func(executionID string) error {
		return nil
	}, "", "", nil)

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
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)

	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
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

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("test-agent"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: false,
			},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "Build a feature", nil, nil, func(executionID string) error {
		return nil
	}, "", "", nil)
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

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:            agents.NewCommand("test-agent"),
			Protocol:       agent.ProtocolACP,
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This will fail because the stream isn't connected (sendStreamRequest returns error).
	// But it should NOT panic due to nil streamManager.
	err := sm.InitializeAndPrompt(ctx, execution, agentConfig, "", nil, nil, func(executionID string) error {
		return nil
	}, "", "", nil)

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

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("test-agent"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: false,
			},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
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

func TestIsMethodNotFoundErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "matching JSON-RPC method not found code",
			err:      fmt.Errorf("wrapped: %w", &acp.RequestError{Code: -32601, Message: "Method not found"}),
			expected: true,
		},
		{
			name:     "non-matching JSON-RPC code",
			err:      fmt.Errorf("wrapped: %w", &acp.RequestError{Code: -32600, Message: "Invalid Request"}),
			expected: false,
		},
		{
			name:     "non-RequestError error",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "direct RequestError with method not found code",
			err:      &acp.RequestError{Code: -32601, Message: "Method not found"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMethodNotFoundErr(tt.err)
			if got != tt.expected {
				t.Errorf("isMethodNotFoundErr(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestIsTransportDeadErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "peer disconnected before response",
			err:      fmt.Errorf("session/load failed: %w", fmt.Errorf("peer disconnected before response")),
			expected: true,
		},
		{
			name:     "peer disconnected while waiting for pre-response notifications",
			err:      fmt.Errorf("peer disconnected while waiting for pre-response notifications"),
			expected: true,
		},
		{
			name:     "connection closed cause",
			err:      fmt.Errorf("load session failed: connection closed"),
			expected: true,
		},
		{
			name:     "notification queue overflow",
			err:      fmt.Errorf("load session failed: notification queue overflow"),
			expected: true,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "context deadline exceeded wrapped",
			err:      fmt.Errorf("load session failed: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "method not found is not transport-dead",
			err:      &acp.RequestError{Code: -32601, Message: "Method not found"},
			expected: false,
		},
		{
			name:     "session unknown is not transport-dead",
			err:      &acp.RequestError{Code: -32002, Message: "Resource not found"},
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "unrelated error",
			err:      fmt.Errorf("some other failure"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransportDeadErr(tt.err)
			if got != tt.expected {
				t.Errorf("isTransportDeadErr(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
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

	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("test-agent"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: true,
			},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
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

// waitForWSConnected blocks until the mock's agent stream WebSocket has accepted
// a connection. Avoids racing the goroutine that StreamUpdates spawns to maintain
// the WS, per CLAUDE.md preference for channel-based sync over time.Sleep.
func waitForWSConnected(t *testing.T, mock *mockAgentServer) {
	t.Helper()
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("agent stream did not connect within 2s")
	}
}

func TestSendPrompt_RetriesUntilUpdateStreamReconnects(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()
	mock.rejectNextStreamAttempts(1)

	log := newSessionTestLogger()
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)

	streamMgr := NewStreamManager(log, StreamCallbacks{
		OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, streamMgr)
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := sm.SendPrompt(ctx, execution, "hello after resume", false, nil, true)
	if err != nil {
		t.Fatalf("SendPrompt should retry after a transient stream reconnect failure: %v", err)
	}
	if result == nil || result.StopReason != PromptStopReasonDispatched {
		t.Fatalf("expected StopReason=%q, got %+v", PromptStopReasonDispatched, result)
	}

	actions := mock.getActionLog()
	if len(actions) != 1 || actions[0] != "agent.prompt" {
		t.Fatalf("expected one prompt after reconnect, got actions: %v", actions)
	}
}

// TestSendPrompt_DispatchOnlyReturnsWithoutWaiting verifies that dispatch-only
// mode returns immediately after agentctl.Prompt succeeds, without blocking on
// the agent's complete event. This is what message_task_kandev relies on so the
// MCP tool call doesn't hang for the duration of the target's turn.
func TestSendPrompt_DispatchOnlyReturnsWithoutWaiting(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	// Stream must be connected before SendPrompt — agentctl.Prompt sends through it.
	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		// Buffered chan but nothing will ever publish to it during this test —
		// dispatch-only must not wait on it.
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}

	done := make(chan struct {
		result *PromptResult
		err    error
	}, 1)
	go func() {
		result, err := sm.SendPrompt(ctx, execution, "hello", false, nil, true)
		done <- struct {
			result *PromptResult
			err    error
		}{result, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("SendPrompt(dispatchOnly) returned error: %v", got.err)
		}
		if got.result == nil || got.result.StopReason != PromptStopReasonDispatched {
			t.Fatalf("expected StopReason=%q, got %+v", PromptStopReasonDispatched, got.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendPrompt(dispatchOnly=true) blocked waiting for completion signal")
	}
}

func TestSendPrompt_PersistsPartialAssistantHistoryBeforeReset(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)

	log := newSessionTestLogger()
	history, err := NewSessionHistoryManager(t.TempDir(), "", log)
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	sm := NewSessionManager(log, make(chan struct{}))
	sm.SetDependencies(nil, nil, nil, history)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID:             "test-exec",
		TaskID:         "test-task",
		SessionID:      "test-session",
		WorkspacePath:  "/workspace",
		Status:         v1.AgentStatusReady,
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
		historyEnabled: true,
	}
	execution.assistantHistoryBuffer.WriteString("partial prior response")

	if _, err := sm.SendPrompt(ctx, execution, "next prompt", true, nil, true); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("history entry count = %d, want assistant then user: %+v", len(entries), entries)
	}
	if entries[0].Type != "agent_message" || entries[0].Content != "partial prior response" {
		t.Fatalf("first history entry = %+v, want partial assistant response", entries[0])
	}
	if entries[1].Type != "user_message" || entries[1].Content != "next prompt" {
		t.Fatalf("second history entry = %+v, want next user prompt", entries[1])
	}
	if execution.assistantHistoryBuffer.Len() != 0 {
		t.Fatal("assistant history buffer was not reset after prompt dispatch")
	}
}

func TestSendPrompt_AdvancesGenerationForEveryDispatch(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))
	store := NewExecutionStore()
	sm.SetDependencies(nil, nil, store, nil)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		Status:        v1.AgentStatusRunning,
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if _, err := sm.SendPrompt(ctx, execution, "initial", false, nil, true); err != nil {
		t.Fatalf("dispatch initial prompt: %v", err)
	}
	if !store.OwnsPromptGeneration(execution.SessionID, execution.ID, 1) {
		t.Fatal("initial prompt must own generation 1 even when execution starts running")
	}

	if _, err := sm.SendPrompt(ctx, execution, "replacement", true, nil, true); err != nil {
		t.Fatalf("dispatch replacement prompt: %v", err)
	}
	if !store.OwnsPromptGeneration(execution.SessionID, execution.ID, 2) {
		t.Fatal("replacement prompt must advance generation while execution remains running")
	}
}

// TestSendPrompt_DrainsStaleSignalFromPriorDispatchOnly verifies that a leftover
// completion signal from a previous dispatch-only prompt does not cause the next
// (waiting) SendPrompt to return immediately with a stale result.
func TestSendPrompt_DrainsStaleSignalFromPriorDispatchOnly(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	// Pre-load a stale signal as if a prior dispatch-only prompt's completion
	// landed in the buffer after SendPrompt returned.
	execution.promptDoneCh <- PromptCompletionSignal{StopReason: "stale"}

	// Run a normal SendPrompt (waits for completion). It must drain the stale
	// signal and block until a real signal arrives or ctx times out.
	timeoutCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	_, err := sm.SendPrompt(timeoutCtx, execution, "hello", false, nil, false)
	if err == nil {
		t.Fatal("expected SendPrompt to block until ctx timeout, but it returned immediately (stale signal not drained)")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded error, got: %v", err)
	}
}

func TestWaitForPromptDone_TreatsPromptAbandonedAfterCancelAsCancelEscalated(t *testing.T) {
	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))
	execution := &AgentExecution{
		ID:           "test-exec",
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.promptDoneCh <- PromptCompletionSignal{
		IsError: true,
		Error:   "prompt abandoned after cancel",
	}

	_, err := sm.waitForPromptDone(context.Background(), execution)
	if !errors.Is(err, ErrCancelEscalated) {
		t.Fatalf("expected ErrCancelEscalated, got: %v", err)
	}
	if !errors.Is(err, ErrAgentReported) {
		t.Fatalf("expected ErrAgentReported wrapper, got: %v", err)
	}
}

func TestSendPrompt_TriggerTimeCancelReleaseReturnsErrCancelEscalated(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "prompt abandoned after cancel", nil)
			return resp
		}
		return mock.defaultHandler(msg)
	}

	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID:            "test-exec",
		TaskID:        "test-task",
		SessionID:     "test-session",
		WorkspacePath: "/workspace",
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}

	_, err := sm.SendPrompt(ctx, execution, "hello", false, nil, true)
	if !errors.Is(err, ErrCancelEscalated) {
		t.Fatalf("expected ErrCancelEscalated, got: %v", err)
	}
}

// TestBuildEffectivePrompt_DoesNotInjectKandevContextOnResume verifies the
// lifecycle layer no longer wraps follow-up prompts with the Kandev system
// block. The orchestrator wraps the first prompt only; on resume the agent
// CLI's restored conversation already contains it.
func TestBuildEffectivePrompt_DoesNotInjectKandevContextOnResume(t *testing.T) {
	log := newSessionTestLogger()
	sm := NewSessionManager(log, make(chan struct{}))

	execution := &AgentExecution{
		ID:                 "test-exec",
		TaskID:             "test-task",
		SessionID:          "test-session",
		needsResumeContext: true,
	}

	got := sm.buildEffectivePrompt(execution, "follow-up message")
	if strings.Contains(got, "<kandev-system>") {
		t.Fatalf("expected no <kandev-system> wrap on resumed prompt, got %q", got)
	}
	if !execution.resumeContextInjected {
		t.Fatal("expected resumeContextInjected to be set after first call")
	}

	// Second call must be a no-op pass-through.
	got2 := sm.buildEffectivePrompt(execution, "another message")
	if got2 != "another message" {
		t.Fatalf("expected pass-through on second call, got %q", got2)
	}
}
