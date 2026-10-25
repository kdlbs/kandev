package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"testing"
)

func TestUnsupportedMCPCapabilitiesDeferUntilRestart(t *testing.T) {
	client, _, cleanup := newSessionMCPClient(t, false, false, false)
	defer cleanup()
	mgr := newTestManager(t)
	mgr.mcpResolver = &sessionMCPResolverFake{}
	repo := newSessionMCPStateFake()
	repo.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
	mgr.SetMCPSelectionStateRepository(repo)
	execution := newSessionMCPExecution(client)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.applyPendingSessionMCP(context.Background(), execution.SessionID); err != nil {
		t.Fatal(err)
	}
	state, _ := repo.GetMCPSelectionState(context.Background(), execution.SessionID)
	if state.ApplyState != mcpconfig.SessionMCPApplyStateDeferredRestart {
		t.Fatalf("state = %#v; want deferred_restart", state)
	}
}
