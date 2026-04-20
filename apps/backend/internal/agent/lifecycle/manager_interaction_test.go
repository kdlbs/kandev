package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
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

// --- IsAgentRunningForSession tests ---

// mockExecutorWithRunner implements ExecutorBackend and returns a real InteractiveRunner.
type mockExecutorWithRunner struct {
	MockExecutor
	runner *process.InteractiveRunner
}

func (m *mockExecutorWithRunner) GetInteractiveRunner() *process.InteractiveRunner {
	return m.runner
}

func TestIsAgentRunningForSession(t *testing.T) {
	t.Run("no execution returns false", func(t *testing.T) {
		store := NewExecutionStore()
		mgr := &Manager{executionStore: store, logger: newTestLogger().WithFields()}
		require.False(t, mgr.IsAgentRunningForSession(context.Background(), "nonexistent"))
	})

	t.Run("passthrough with alive process returns true", func(t *testing.T) {
		log := newTestLogger()
		runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)

		// Start a deferred process (pending but alive)
		info, err := runner.Start(context.Background(), process.InteractiveStartRequest{
			SessionID: "session-pt",
			Command:   []string{"cat"},
		})
		require.NoError(t, err)

		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:                   "exec-pt",
			SessionID:            "session-pt",
			PassthroughProcessID: info.ID,
			Status:               v1.AgentStatusRunning,
		})

		execRegistry := NewExecutorRegistry(log)
		execRegistry.Register(&mockExecutorWithRunner{
			MockExecutor: MockExecutor{name: executor.NameStandalone},
			runner:       runner,
		})

		mgr := &Manager{
			executionStore:   store,
			executorRegistry: execRegistry,
			logger:           log.WithFields(),
		}
		require.True(t, mgr.IsAgentRunningForSession(context.Background(), "session-pt"))
	})

	t.Run("passthrough with dead process returns false", func(t *testing.T) {
		log := newTestLogger()
		runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)

		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:                   "exec-dead",
			SessionID:            "session-dead",
			PassthroughProcessID: "nonexistent-process-id",
			Status:               v1.AgentStatusRunning,
		})

		execRegistry := NewExecutorRegistry(log)
		execRegistry.Register(&mockExecutorWithRunner{
			MockExecutor: MockExecutor{name: executor.NameStandalone},
			runner:       runner,
		})

		mgr := &Manager{
			executionStore:   store,
			executorRegistry: execRegistry,
			logger:           log.WithFields(),
		}
		require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-dead"))
	})

	t.Run("passthrough with nil executor registry returns false", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:                   "exec-noreg",
			SessionID:            "session-noreg",
			PassthroughProcessID: "some-process-id",
			Status:               v1.AgentStatusRunning,
		})

		mgr := &Manager{
			executionStore: store,
			logger:         newTestLogger().WithFields(),
		}
		require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-noreg"))
	})

	t.Run("ACP execution with nil agentctl returns false", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:        "exec-acp-nil",
			SessionID: "session-acp-nil",
			Status:    v1.AgentStatusRunning,
			// No PassthroughProcessID → ACP path
			// No agentctl → returns false
		})

		mgr := &Manager{
			executionStore: store,
			logger:         newTestLogger().WithFields(),
		}
		require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-acp-nil"))
	})

	t.Run("ACP execution with running agent returns true", func(t *testing.T) {
		// Mock agentctl server that returns "running" status
		statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/status" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"agent_status":"running"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(statusServer.Close)

		client := createTestClient(t, statusServer.URL)
		t.Cleanup(client.Close)

		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:        "exec-acp-running",
			SessionID: "session-acp-running",
			Status:    v1.AgentStatusRunning,
			agentctl:  client,
		})

		mgr := &Manager{
			executionStore: store,
			logger:         newTestLogger().WithFields(),
		}
		require.True(t, mgr.IsAgentRunningForSession(context.Background(), "session-acp-running"))
	})

	t.Run("ACP execution with stopped agent returns false", func(t *testing.T) {
		statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/status" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"agent_status":"stopped"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(statusServer.Close)

		client := createTestClient(t, statusServer.URL)
		t.Cleanup(client.Close)

		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:        "exec-acp-stopped",
			SessionID: "session-acp-stopped",
			Status:    v1.AgentStatusRunning,
			agentctl:  client,
		})

		mgr := &Manager{
			executionStore: store,
			logger:         newTestLogger().WithFields(),
		}
		require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-acp-stopped"))
	})
}

// --- IsRemoteSession tests ---

type mockWorkspaceInfoProvider struct {
	infos map[string]*WorkspaceInfo
	err   error
}

func (m *mockWorkspaceInfoProvider) GetWorkspaceInfoForSession(_ context.Context, _, sessionID string) (*WorkspaceInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.infos[sessionID], nil
}

func TestIsRemoteSession(t *testing.T) {
	t.Run("in-memory execution with sprites runtime", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-1",
			SessionID:   "session-1",
			RuntimeName: string(executor.NameSprites),
			Status:      v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store}
		require.True(t, mgr.IsRemoteSession(context.Background(), "session-1"))
	})

	t.Run("in-memory execution with is_remote metadata", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-2",
			SessionID:   "session-2",
			RuntimeName: string(executor.NameStandalone),
			Status:      v1.AgentStatusRunning,
			Metadata:    map[string]interface{}{MetadataKeyIsRemote: true},
		})
		mgr := &Manager{executionStore: store}
		require.True(t, mgr.IsRemoteSession(context.Background(), "session-2"))
	})

	t.Run("in-memory execution with local runtime", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-3",
			SessionID:   "session-3",
			RuntimeName: string(executor.NameStandalone),
			Status:      v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store}
		require.False(t, mgr.IsRemoteSession(context.Background(), "session-3"))
	})

	t.Run("not in memory, DB returns sprites executor type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-4": {ExecutorType: string(models.ExecutorTypeSprites)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.IsRemoteSession(context.Background(), "session-4"))
	})

	t.Run("not in memory, DB returns remote_docker executor type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-5": {ExecutorType: string(models.ExecutorTypeRemoteDocker)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.IsRemoteSession(context.Background(), "session-5"))
	})

	t.Run("not in memory, DB returns sprites runtime name", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-6": {RuntimeName: string(executor.NameSprites)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.IsRemoteSession(context.Background(), "session-6"))
	})

	t.Run("not in memory, DB returns local type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-7": {ExecutorType: string(models.ExecutorTypeLocal)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.False(t, mgr.IsRemoteSession(context.Background(), "session-7"))
	})

	t.Run("not in memory, DB error returns false", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{err: fmt.Errorf("db error")}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.False(t, mgr.IsRemoteSession(context.Background(), "session-8"))
	})

	t.Run("nil workspaceInfoProvider returns false", func(t *testing.T) {
		store := NewExecutionStore()
		mgr := &Manager{executionStore: store}
		require.False(t, mgr.IsRemoteSession(context.Background(), "nonexistent"))
	})
}

func TestShouldUseContainerShell(t *testing.T) {
	t.Run("in-memory execution with docker runtime", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-1",
			SessionID:   "session-1",
			RuntimeName: string(executor.NameDocker),
			Status:      v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-1"))
	})

	t.Run("in-memory execution with sprites runtime", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-2",
			SessionID:   "session-2",
			RuntimeName: string(executor.NameSprites),
			Status:      v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-2"))
	})

	t.Run("in-memory execution with is_remote metadata", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-3",
			SessionID:   "session-3",
			RuntimeName: string(executor.NameStandalone),
			Status:      v1.AgentStatusRunning,
			Metadata:    map[string]interface{}{MetadataKeyIsRemote: true},
		})
		mgr := &Manager{executionStore: store}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-3"))
	})

	t.Run("in-memory execution with standalone runtime", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:          "exec-4",
			SessionID:   "session-4",
			RuntimeName: string(executor.NameStandalone),
			Status:      v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store}
		require.False(t, mgr.ShouldUseContainerShell(context.Background(), "session-4"))
	})

	t.Run("not in memory, DB returns local_docker executor type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-5": {ExecutorType: string(models.ExecutorTypeLocalDocker)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-5"))
	})

	t.Run("not in memory, DB returns sprites executor type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-6": {ExecutorType: string(models.ExecutorTypeSprites)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-6"))
	})

	t.Run("not in memory, DB returns docker runtime name", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-7": {RuntimeName: string(executor.NameDocker)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.True(t, mgr.ShouldUseContainerShell(context.Background(), "session-7"))
	})

	t.Run("not in memory, DB returns local type", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-8": {ExecutorType: string(models.ExecutorTypeLocal)},
			},
		}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.False(t, mgr.ShouldUseContainerShell(context.Background(), "session-8"))
	})

	t.Run("not in memory, DB error returns false", func(t *testing.T) {
		store := NewExecutionStore()
		provider := &mockWorkspaceInfoProvider{err: fmt.Errorf("db error")}
		mgr := &Manager{executionStore: store, workspaceInfoProvider: provider}
		require.False(t, mgr.ShouldUseContainerShell(context.Background(), "session-9"))
	})

	t.Run("nil workspaceInfoProvider returns false", func(t *testing.T) {
		store := NewExecutionStore()
		mgr := &Manager{executionStore: store}
		require.False(t, mgr.ShouldUseContainerShell(context.Background(), "nonexistent"))
	})
}

func TestFallbackAuthMethods(t *testing.T) {
	t.Run("claude-acp returns auth login method", func(t *testing.T) {
		methods := fallbackAuthMethods("claude-acp")
		require.Len(t, methods, 1)
		require.Equal(t, "claude-auth-login", methods[0].ID)
		require.Equal(t, "Anthropic Authentication", methods[0].Name)
		require.NotNil(t, methods[0].TerminalAuth)
		require.Equal(t, "claude", methods[0].TerminalAuth.Command)
		require.Equal(t, []string{"auth", "login"}, methods[0].TerminalAuth.Args)
	})

	t.Run("auggie returns login method", func(t *testing.T) {
		methods := fallbackAuthMethods("auggie")
		require.Len(t, methods, 1)
		require.Equal(t, "auggie-login", methods[0].ID)
		require.Equal(t, "Auggie Authentication", methods[0].Name)
		require.NotNil(t, methods[0].TerminalAuth)
		require.Equal(t, "auggie", methods[0].TerminalAuth.Command)
		require.Equal(t, []string{"login"}, methods[0].TerminalAuth.Args)
	})

	t.Run("unknown agent returns nil", func(t *testing.T) {
		require.Nil(t, fallbackAuthMethods("unknown-agent"))
	})

	t.Run("empty agent ID returns nil", func(t *testing.T) {
		require.Nil(t, fallbackAuthMethods(""))
	})
}

func TestGetSessionAuthMethodsFallback(t *testing.T) {
	t.Run("returns cached methods when available", func(t *testing.T) {
		store := NewExecutionStore()
		exec := &AgentExecution{
			ID:        "exec-1",
			SessionID: "session-1",
			AgentID:   "claude-acp",
		}
		exec.SetAuthMethods([]streams.AuthMethodInfo{
			{ID: "custom-method", Name: "Custom"},
		})
		store.Add(exec)
		mgr := &Manager{executionStore: store}

		methods := mgr.GetSessionAuthMethods("session-1")
		require.Len(t, methods, 1)
		require.Equal(t, "custom-method", methods[0].ID)
	})

	t.Run("falls back to static methods when cache is empty", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:        "exec-2",
			SessionID: "session-2",
			AgentID:   "claude-acp",
		})
		mgr := &Manager{executionStore: store}

		methods := mgr.GetSessionAuthMethods("session-2")
		require.Len(t, methods, 1)
		require.Equal(t, "claude-auth-login", methods[0].ID)
	})

	t.Run("returns nil for unknown session", func(t *testing.T) {
		store := NewExecutionStore()
		mgr := &Manager{executionStore: store}
		require.Nil(t, mgr.GetSessionAuthMethods("nonexistent"))
	})
}
