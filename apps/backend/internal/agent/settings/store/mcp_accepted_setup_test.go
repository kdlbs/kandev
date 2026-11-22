package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.17
func TestMCPAcceptedSetupSurvivesStateWrites(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	var state mcpconfig.SessionMCPSelectionState
	err := json.Unmarshal([]byte(`{"desired_revision":2,"applied_revision":1,"apply_state":"pending_idle","accepted_setup":{"version":1,"servers":[{"definition_id":"docs","revision":4,"enabled":true,"display_name":"Team docs","runtime_name":"docs"}]}}`), &state)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMCPSelectionState(ctx, "session", state); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	assertAcceptedSetupJSON(t, got)
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, "workspace", "session", nil, got); err != nil {
		t.Fatal(err)
	}
	got.DesiredRevision++
	if ok, err := repo.CompareAndSwapMCPSelectionState(ctx, "session", 2, got); err != nil || !ok {
		t.Fatalf("CAS = %v, %v", ok, err)
	}
	got, err = repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	assertAcceptedSetupJSON(t, got)
}

func assertAcceptedSetupJSON(t *testing.T, state mcpconfig.SessionMCPSelectionState) {
	t.Helper()
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["accepted_setup"]) != `{"version":1,"servers":[{"definition_id":"docs","revision":4,"enabled":true,"display_name":"Team docs","runtime_name":"docs"}]}` {
		t.Fatalf("accepted setup lost: %s", encoded)
	}
}

func TestMCPAcceptedSetupDoesNotClearNewerDesiredOrRegress(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	writer, ok := any(repo).(mcpconfig.AcceptedMCPSetupRepository)
	if !ok {
		t.Fatal("repository cannot atomically accept setup")
	}
	ctx := context.Background()
	setup := &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}
	if err := repo.SaveMCPSelectionState(ctx, "session", mcpconfig.SessionMCPSelectionState{DesiredRevision: 3, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}); err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveAcceptedMCPSetup(ctx, "session", 2, "accepted-2", setup); err != nil {
		t.Fatal(err)
	}
	state, err := repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedRevision != 2 || state.DesiredRevision != 3 || state.ApplyState != mcpconfig.SessionMCPApplyStatePendingIdle || state.AcceptedSetup == nil {
		t.Fatalf("older acceptance cleared newer change: %#v", state)
	}
	if err := writer.SaveAcceptedMCPSetup(ctx, "session", 1, "stale", nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, "workspace", "session", nil, mcpconfig.SessionMCPSelectionState{DesiredRevision: 4, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}); err != nil {
		t.Fatal(err)
	}
	state, err = repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedRevision != 2 || state.AttachmentAttemptID != "accepted-2" || state.AcceptedSetup == nil {
		t.Fatalf("stale read overwrote accepted baseline: %#v", state)
	}
	if err := writer.SaveAcceptedMCPSetup(ctx, "session", 4, "accepted-4", setup); err != nil {
		t.Fatal(err)
	}
	state, err = repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.ApplyState != mcpconfig.SessionMCPApplyStateApplied || state.AppliedRevision != 4 {
		t.Fatalf("positive acceptance failed: %#v", state)
	}
}
