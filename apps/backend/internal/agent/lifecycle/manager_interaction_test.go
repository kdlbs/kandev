package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type restartMockAgentctlServer struct {
	server *httptest.Server

	mu          sync.Mutex
	httpActions []string
	wsActions   []string

	failStop       bool
	failSessionNew bool
}

func newRestartMockAgentctlServer(t *testing.T, failStop, failSessionNew bool) *restartMockAgentctlServer {
	t.Helper()

	m := &restartMockAgentctlServer{
		failStop:       failStop,
		failSessionNew: failSessionNew,
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/v1/stop", func(w http.ResponseWriter, _ *http.Request) {
		m.recordHTTP("stop")
		if m.failStop {
			_, _ = w.Write([]byte(`{"success":false,"error":"stop failed"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/api/v1/agent/configure", func(w http.ResponseWriter, _ *http.Request) {
		m.recordHTTP("configure")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/api/v1/start", func(w http.ResponseWriter, _ *http.Request) {
		m.recordHTTP("start")
		_, _ = w.Write([]byte(`{"success":true,"command":"auggie --model test"}`))
	})
	mux.HandleFunc("/api/v1/agent/stream", func(w http.ResponseWriter, r *http.Request) {
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
			if msg.Type != ws.MessageTypeRequest {
				continue
			}

			m.recordWS(msg.Action)

			var resp *ws.Message
			switch msg.Action {
			case "agent.initialize":
				resp, _ = ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
					"success": true,
					"agent_info": map[string]string{
						"name":    "test-agent",
						"version": "1.0.0",
					},
				})
			case "agent.session.new":
				if m.failSessionNew {
					resp, _ = ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
						"success": false,
						"error":   "session new failed",
					})
				} else {
					resp, _ = ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
						"success":    true,
						"session_id": "new-session-123",
					})
				}
			default:
				resp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "unknown action", nil)
			}

			data, _ := json.Marshal(resp)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/api/v1/workspace/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		connected := map[string]string{"type": "connected"}
		data, _ := json.Marshal(connected)
		_ = conn.WriteMessage(websocket.TextMessage, data)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func (m *restartMockAgentctlServer) recordHTTP(action string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpActions = append(m.httpActions, action)
}

func (m *restartMockAgentctlServer) recordWS(action string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsActions = append(m.wsActions, action)
}

func (m *restartMockAgentctlServer) getHTTPActions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.httpActions))
	copy(out, m.httpActions)
	return out
}

func (m *restartMockAgentctlServer) getWSActions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.wsActions))
	copy(out, m.wsActions)
	return out
}

func TestManager_RestartAgentProcess_Success(t *testing.T) {
	mgr := newTestManager()
	mock := newRestartMockAgentctlServer(t, false, false)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	exec := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		ACPSessionID:   "old-session",
		AgentCommand:   "auggie --model test",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		Metadata: map[string]interface{}{
			"task_description": "review the changes",
		},
		agentctl:     client,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	exec.messageBuffer.WriteString("old-response")
	exec.thinkingBuffer.WriteString("old-thinking")
	exec.currentMessageID = "msg-1"
	exec.currentThinkingID = "th-1"
	exec.needsResumeContext = true
	exec.resumeContextInjected = true
	exec.promptDoneCh <- PromptCompletionSignal{StopReason: "stale"}

	mgr.executionStore.Add(exec)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := mgr.RestartAgentProcess(ctx, exec.ID); err != nil {
		t.Fatalf("RestartAgentProcess failed: %v", err)
	}

	if exec.ACPSessionID != "new-session-123" {
		t.Fatalf("expected new ACP session ID, got %q", exec.ACPSessionID)
	}
	if exec.Status != v1.AgentStatusReady {
		t.Fatalf("expected status %q, got %q", v1.AgentStatusReady, exec.Status)
	}
	if exec.messageBuffer.Len() != 0 || exec.thinkingBuffer.Len() != 0 {
		t.Fatalf("expected message buffers to be reset")
	}
	if exec.currentMessageID != "" || exec.currentThinkingID != "" {
		t.Fatalf("expected streaming message IDs to be reset")
	}
	if exec.needsResumeContext || exec.resumeContextInjected {
		t.Fatalf("expected resume context flags to be reset")
	}
	select {
	case <-exec.promptDoneCh:
		t.Fatalf("expected stale prompt signal to be drained")
	default:
	}

	httpActions := mock.getHTTPActions()
	if !slices.Equal(httpActions, []string{"stop", "configure", "start"}) {
		t.Fatalf("unexpected HTTP action order: %v", httpActions)
	}

	wsActions := mock.getWSActions()
	if !slices.Equal(wsActions, []string{"agent.initialize", "agent.session.new"}) {
		t.Fatalf("unexpected WS action order: %v", wsActions)
	}

	mockBus, ok := mgr.eventBus.(*MockEventBus)
	if !ok {
		t.Fatal("expected mock event bus")
	}
	eventTypes := make([]string, 0, len(mockBus.PublishedEvents))
	for _, ev := range mockBus.PublishedEvents {
		eventTypes = append(eventTypes, ev.Type)
	}
	if !slices.Contains(eventTypes, events.AgentReady) {
		t.Fatalf("expected %q event, got %v", events.AgentReady, eventTypes)
	}
	if !slices.Contains(eventTypes, events.AgentACPSessionCreated) {
		t.Fatalf("expected %q event, got %v", events.AgentACPSessionCreated, eventTypes)
	}
	if !slices.Contains(eventTypes, events.AgentContextReset) {
		t.Fatalf("expected %q event, got %v", events.AgentContextReset, eventTypes)
	}
}

func TestManager_RestartAgentProcess_StopErrorIsNonFatal(t *testing.T) {
	mgr := newTestManager()
	mock := newRestartMockAgentctlServer(t, true, false)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	exec := &AgentExecution{
		ID:             "exec-stop-error",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		AgentCommand:   "auggie --model test",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
	}
	mgr.executionStore.Add(exec)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := mgr.RestartAgentProcess(ctx, exec.ID); err != nil {
		t.Fatalf("expected restart to continue after stop failure, got: %v", err)
	}
}

func TestManager_RestartAgentProcess_SessionInitFailure(t *testing.T) {
	mgr := newTestManager()
	mock := newRestartMockAgentctlServer(t, false, true)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	exec := &AgentExecution{
		ID:             "exec-session-fail",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		AgentCommand:   "auggie --model test",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
	}
	mgr.executionStore.Add(exec)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := mgr.RestartAgentProcess(ctx, exec.ID)
	if err == nil {
		t.Fatal("expected restart to fail when ACP session initialization fails")
	}

	updated, found := mgr.executionStore.Get(exec.ID)
	if !found {
		t.Fatal("expected execution to still exist")
	}
	if updated.Status != v1.AgentStatusFailed {
		t.Fatalf("expected status %q, got %q", v1.AgentStatusFailed, updated.Status)
	}
	if updated.ErrorMessage == "" {
		t.Fatal("expected execution error message to be set")
	}

	mockBus, ok := mgr.eventBus.(*MockEventBus)
	if !ok {
		t.Fatal("expected mock event bus")
	}
	for _, ev := range mockBus.PublishedEvents {
		if ev.Type == events.AgentContextReset {
			t.Fatalf("did not expect %q event on failed restart", events.AgentContextReset)
		}
	}
}
