package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type acceptedSetupResolverFake struct{ state *sessionMCPStateFake }

func (r *acceptedSetupResolverFake) Resolve(ctx context.Context, _ mcpconfig.ResolutionContext, _ mcpconfig.Policy) (*mcpconfig.EffectiveMCPResolution, error) {
	// A newer save lands after this operation's immutable resolution was read.
	if err := r.state.SaveMCPSelectionState(ctx, "session-1", mcpconfig.SessionMCPSelectionState{DesiredRevision: 3, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}); err != nil {
		return nil, err
	}
	return &mcpconfig.EffectiveMCPResolution{DesiredRevision: 2, Setup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{{DefinitionID: "old", Revision: 1, Enabled: true, DisplayName: "Accepted docs", RuntimeName: "old"}}}, Servers: []mcpconfig.ResolvedServer{{Name: "old", DefinitionID: "old", DefinitionRevision: 1}}}, nil
}

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.17
func TestMCPAcceptanceCapturesResolvedOperationWhileNewerSelectionStaysPending(t *testing.T) {
	client, _, cleanup := newSessionMCPClient(t, false, true, false)
	defer cleanup()
	mgr := newTestManager(t)
	stateRepo := newSessionMCPStateFake()
	stateRepo.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
	mgr.mcpResolver = &acceptedSetupResolverFake{state: stateRepo}
	mgr.SetMCPSelectionStateRepository(stateRepo)
	execution := newSessionMCPExecution(client)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.applyPendingSessionMCP(context.Background(), execution.SessionID); err != nil {
		t.Fatal(err)
	}
	got, err := stateRepo.GetMCPSelectionState(context.Background(), execution.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DesiredRevision != 3 || got.AppliedRevision != 2 || got.ApplyState != mcpconfig.SessionMCPApplyStatePendingIdle || got.AcceptedSetup == nil || got.AcceptedSetup.Servers[0].DefinitionID != "old" {
		t.Fatalf("accepted operation not retained: %#v", got)
	}
}

func TestMCPInitialPromptRecordsAcceptedSetup(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()
	client := createTestClient(t, mock.server.URL)
	_ = connectAgentStream(t, mock, client)
	mgr := newTestManager(t)
	mgr.sessionManager = NewSessionManager(newSessionTestLogger(), mgr.stopCh)
	repo := newSessionMCPStateFake()
	mgr.SetMCPSelectionStateRepository(repo)
	defer client.Close()
	execution := &AgentExecution{ID: "initial", TaskID: "task", SessionID: "initial-session", WorkspacePath: "/workspace", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	dispatched := make(chan struct{}, 1)
	execution.setInitialPromptDispatchCallbacks(nil, func() {
		dispatched <- struct{}{}
		execution.promptDoneCh <- PromptCompletionSignal{StopReason: "test-complete"}
	}, nil)
	operation := prepareMCPResolution(&mcpconfig.EffectiveMCPResolution{DesiredRevision: 0, Setup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}})
	agentConfig := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{}}}
	if err := mgr.initializeACPSession(operation.context(context.Background()), execution, agentConfig, "initial work", nil, nil); err != nil {
		t.Fatal(err)
	}
	state, err := repo.GetMCPSelectionState(context.Background(), execution.SessionID)
	if err != nil || state.AcceptedSetup == nil {
		t.Fatalf("initial prompt setup not recorded: %#v, %v", state, err)
	}
	select {
	case <-dispatched:
	case <-time.After(3 * time.Second):
		t.Fatal("initial prompt was not dispatched")
	}
	mgr.closeStopCh()
}

func TestMCPStaleStateUpdatesDoNotUndoAcceptedSetup(t *testing.T) {
	mgr := newTestManager(t)
	repo := newSessionMCPStateFake()
	mgr.SetMCPSelectionStateRepository(repo)
	for _, update := range []func(context.Context, string) error{
		mgr.savePendingSessionMCP, mgr.saveDeferredSessionMCP,
		func(ctx context.Context, id string) error {
			return mgr.saveFailedSessionMCP(ctx, id, 1, errors.New("old failure"))
		},
	} {
		repo.state["session"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 2, ApplyState: mcpconfig.SessionMCPApplyStateApplied, AcceptedSetup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}}
		if err := update(context.Background(), "session"); err != nil {
			t.Fatal(err)
		}
		got, _ := repo.GetMCPSelectionState(context.Background(), "session")
		if got.ApplyState != mcpconfig.SessionMCPApplyStateApplied || got.AcceptedSetup == nil {
			t.Fatalf("stale update undid acceptance: %#v", got)
		}
	}
}

func TestMCPPreparedOperationRetainsItsOwnResolution(t *testing.T) {
	result := &mcpconfig.EffectiveMCPResolution{DesiredRevision: 2,
		Setup:     &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{{DefinitionID: "before", Revision: 2, RuntimeName: "docs", DisplayName: "Docs", Enabled: true}}},
		Servers:   []mcpconfig.ResolvedServer{{Name: "docs", Type: mcpconfig.ServerTypeStdio, Command: "node", Args: []string{"before"}, Env: map[string]string{"TOKEN": "private-value"}, DefinitionID: "before", DefinitionRevision: 2}},
		Decisions: []mcpconfig.MCPResolutionDecision{{DefinitionID: "excluded", RuntimeName: "protected", ReasonCode: "secret_resolution_failed", Origins: []mcpconfig.SelectionOrigin{{Scope: mcpconfig.SelectionScopeRepository, OwnerID: "repo"}}}},
	}
	operation := prepareMCPResolution(result)
	result.DesiredRevision = 3
	result.Setup.Servers[0].DefinitionID = "after"
	result.Servers[0].Args[0] = "after"
	result.Servers[0].Env["TOKEN"] = "after"
	result.Decisions[0].Origins[0].OwnerID = "after"
	setup, revision, captured := capturedMCPSetup(operation.context(context.Background()))
	if !captured || revision != 2 || setup.Servers[0].DefinitionID != "before" || operation.servers[0].Args[0] != "before" || operation.servers[0].Env["TOKEN"] != "private-value" {
		t.Fatal("prepared operation changed with resolver state")
	}
	metadata := streams.MCPResolutionFromContext(operation.context(context.Background()))
	if metadata[1].Origins[0].OwnerID != "repo" {
		t.Fatal("origin was not frozen")
	}
	encoded, _ := json.Marshal(metadata)
	if strings.Contains(string(encoded), "private-value") {
		t.Fatal("secret entered safe context")
	}
}
