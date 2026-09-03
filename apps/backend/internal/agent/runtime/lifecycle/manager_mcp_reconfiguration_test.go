package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type sessionMCPStateFake struct {
	mu    sync.Mutex
	state map[string]mcpconfig.SessionMCPSelectionState
}

func newSessionMCPStateFake() *sessionMCPStateFake {
	return &sessionMCPStateFake{state: make(map[string]mcpconfig.SessionMCPSelectionState)}
}

func (r *sessionMCPStateFake) GetMCPSelectionState(_ context.Context, sessionID string) (mcpconfig.SessionMCPSelectionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.state[sessionID]
	if !ok {
		return mcpconfig.SessionMCPSelectionState{}, mcpconfig.ErrMCPSelectionStateNotFound
	}
	return state, nil
}

func (r *sessionMCPStateFake) SaveMCPSelectionState(_ context.Context, sessionID string, state mcpconfig.SessionMCPSelectionState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state[sessionID] = state
	return nil
}

type sessionMCPResolverFake struct{}

func (r *sessionMCPResolverFake) Resolve(context.Context, mcpconfig.ResolutionContext, mcpconfig.Policy) (*mcpconfig.EffectiveMCPResolution, error) {
	return &mcpconfig.EffectiveMCPResolution{Servers: []mcpconfig.ResolvedServer{}}, nil
}

func newSessionMCPExecution(client *agentctl.Client) *AgentExecution {
	return &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ACPSessionID: "acp-session-1", WorkspacePath: "/workspace",
		Status: v1.AgentStatusReady, agentctl: client,
	}
}

func TestApplyPendingSessionMCPDefersWhileTurnIsActive(t *testing.T) {
	mgr := newTestManager(t)
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	client := agentctl.NewClient("127.0.0.1", 1, newTestLogger())
	execution := newSessionMCPExecution(client)
	execution.Status = v1.AgentStatusRunning
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.applyPendingSessionMCP(context.Background(), execution.SessionID); err != nil {
		t.Fatalf("applyPendingSessionMCP: %v", err)
	}
	state, _ := stateRepo.GetMCPSelectionState(context.Background(), execution.SessionID)
	if state.ApplyState != mcpconfig.SessionMCPApplyStatePendingIdle || state.AppliedRevision != 1 {
		t.Fatalf("state = %#v, want pending idle with prior revision", state)
	}
}

func TestCompletedTurnReappliesPendingSessionMCP(t *testing.T) {
	client, actions, cleanup := newSessionMCPClient(t, false, false, true)
	defer cleanup()
	mgr := newTestManager(t)
	mgr.mcpResolver = &sessionMCPResolverFake{}
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 1, AppliedRevision: 0, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	execution := newSessionMCPExecution(client)
	execution.Status = v1.AgentStatusRunning
	execution.promptDoneCh = make(chan PromptCompletionSignal, 1)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt: %v", err)
	}

	if !mgr.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type:             "complete",
		SessionID:        execution.SessionID,
		PromptGeneration: generation,
		Data:             map[string]any{"stop_reason": "end_turn"},
	}) {
		t.Fatal("completed turn was not accepted")
	}
	if got := <-actions; got != "agent.initialize" {
		t.Fatalf("first action = %q, want initialize", got)
	}
	select {
	case got := <-actions:
		if got != "agent.session.load" {
			t.Fatalf("second action = %q, want session.load", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("completed turn did not retry pending session MCP configuration")
	}

	deadline := time.After(2 * time.Second)
	for {
		state, _ := stateRepo.GetMCPSelectionState(context.Background(), execution.SessionID)
		if state.ApplyState == mcpconfig.SessionMCPApplyStateApplied && state.AppliedRevision == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("state = %#v, want applied revision 1", state)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestApplyPendingSessionMCPUsesResumeAndRecordsAppliedAttempt(t *testing.T) {
	client, actions, cleanup := newSessionMCPClient(t, false, true, false)
	defer cleanup()
	mgr := newTestManager(t)
	mgr.mcpResolver = &sessionMCPResolverFake{}
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	execution := newSessionMCPExecution(client)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.applyPendingSessionMCP(context.Background(), execution.SessionID); err != nil {
		t.Fatalf("applyPendingSessionMCP: %v", err)
	}
	if got := <-actions; got != "agent.initialize" {
		t.Fatalf("first action = %q", got)
	}
	if got := <-actions; got != "agent.session.resume" {
		t.Fatalf("second action = %q, want resume", got)
	}
	state, _ := stateRepo.GetMCPSelectionState(context.Background(), execution.SessionID)
	if state.ApplyState != mcpconfig.SessionMCPApplyStateApplied || state.AppliedRevision != 2 || state.AttachmentAttemptID != "attempt-resume" {
		t.Fatalf("state = %#v", state)
	}
}

func TestApplyPendingSessionMCPFallsBackToLoad(t *testing.T) {
	client, actions, cleanup := newSessionMCPClient(t, true, true, true)
	defer cleanup()
	mgr := newTestManager(t)
	mgr.mcpResolver = &sessionMCPResolverFake{}
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 1, AppliedRevision: 0, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	if err := mgr.executionStore.Add(newSessionMCPExecution(client)); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.applyPendingSessionMCP(context.Background(), "session-1"); err != nil {
		t.Fatalf("applyPendingSessionMCP: %v", err)
	}
	if got := <-actions; got != "agent.initialize" {
		t.Fatalf("first action = %q", got)
	}
	if got := <-actions; got != "agent.session.resume" {
		t.Fatalf("second action = %q", got)
	}
	if got := <-actions; got != "agent.session.load" {
		t.Fatalf("third action = %q, want load", got)
	}
	state, _ := stateRepo.GetMCPSelectionState(context.Background(), "session-1")
	if state.ApplyState != mcpconfig.SessionMCPApplyStateApplied || state.AttachmentAttemptID != "attempt-load" {
		t.Fatalf("state = %#v", state)
	}
}

func TestApplyPendingSessionMCPFailurePreservesAppliedRevision(t *testing.T) {
	client, actions, cleanup := newSessionMCPClient(t, true, true, false)
	defer cleanup()
	mgr := newTestManager(t)
	mgr.mcpResolver = &sessionMCPResolverFake{}
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	if err := mgr.executionStore.Add(newSessionMCPExecution(client)); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.applyPendingSessionMCP(context.Background(), "session-1"); err != nil {
		t.Fatalf("applyPendingSessionMCP: %v", err)
	}
	<-actions
	<-actions
	state, _ := stateRepo.GetMCPSelectionState(context.Background(), "session-1")
	if state.ApplyState != mcpconfig.SessionMCPApplyStateFailed || state.AppliedRevision != 1 || state.FailureCode != "session_resume_failed" {
		t.Fatalf("state = %#v", state)
	}
	if state.FailureSummary == "provider rejected update" {
		t.Fatal("raw provider error leaked into durable failure summary")
	}
}

func newSessionMCPClient(t *testing.T, failResume, supportsResume, supportsLoad bool) (*agentctl.Client, <-chan string, func()) {
	t.Helper()
	actions := make(chan string, 8)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg ws.Message
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			actions <- msg.Action
			var response *ws.Message
			switch msg.Action {
			case "agent.initialize":
				response, _ = ws.NewResponse(msg.ID, msg.Action, map[string]any{
					"success": true, "agent_info": map[string]any{
						"name": "test", "version": "1",
						"supports_session_resume": supportsResume,
						"supports_session_load":   supportsLoad,
					},
				})
			case "agent.session.resume":
				if failResume {
					response, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "resume rejected", nil)
				} else {
					response, _ = ws.NewResponse(msg.ID, msg.Action, map[string]any{
						"success": true, "attachment_attempt_id": "attempt-resume",
					})
				}
			case "agent.session.load":
				response, _ = ws.NewResponse(msg.ID, msg.Action, map[string]any{
					"success": true, "attachment_attempt_id": "attempt-load",
				})
			}
			if response != nil {
				encoded, _ := json.Marshal(response)
				if conn.WriteMessage(websocket.TextMessage, encoded) != nil {
					return
				}
			}
		}
	}))
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())
	if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		server.Close()
		t.Fatalf("StreamUpdates: %v", err)
	}
	if _, err := client.Initialize(context.Background(), "kandev", "1"); err != nil {
		client.Close()
		server.Close()
		t.Fatalf("Initialize: %v", err)
	}
	cleanup := func() {
		client.Close()
		server.Close()
	}
	return client, actions, cleanup
}
