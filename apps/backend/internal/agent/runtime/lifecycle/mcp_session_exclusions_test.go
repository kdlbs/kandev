package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.1, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.3, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.4
func TestMCPSessionExclusionIdleAcceptanceAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "accept", true: "failure"}[fail], func(t *testing.T) { testSessionExclusionIdle(t, fail) })
	}
}

func testSessionExclusionIdle(t *testing.T, fail bool) {
	ctx := context.Background()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo, _, err := store.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	def := &mcpconfig.MCPServerDefinition{ID: "docs", WorkspaceID: "workspace-1", RuntimeName: "docs", NormalizedRuntimeName: "docs", DisplayName: "Docs", Enabled: true, ExecutionMode: "remote", Transport: "http", Configuration: mcpconfig.MCPServerConfiguration{URL: "https://example.test/mcp"}, Revision: 1}
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, def.WorkspaceID, "task-1", []string{def.ID}); err != nil {
		t.Fatal(err)
	}
	baseline := &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{{DefinitionID: def.ID, Revision: 1, Enabled: true, DisplayName: def.DisplayName, RuntimeName: def.RuntimeName}}}
	if err := repo.SaveAcceptedMCPSetup(ctx, "session-1", 1, "before", baseline); err != nil {
		t.Fatal(err)
	}
	rc := mcpconfig.ResolutionContext{WorkspaceID: def.WorkspaceID, SessionID: "session-1", TaskID: "task-1"}
	ids := []string{def.ID}
	revision := int64(1)
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: rc.WorkspaceID, OwnerID: rc.SessionID}, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &ids, ExclusionContext: &rc}); err != nil {
		t.Fatal(err)
	}
	captured := make(chan json.RawMessage, 2)
	client, actions, cleanup := newSessionMCPClientWithHooks(t, fail, true, false, nil, func(msg ws.Message) {
		if msg.Action == "agent.session.resume" {
			captured <- msg.Payload
		}
	})
	defer cleanup()
	<-actions // initialization is independent of applying a saved exclusion
	mgr := newTestManager(t)
	mgr.mcpResolver = mcpconfig.NewResolver(repo, repo)
	mgr.SetMCPSelectionStateRepository(repo)
	execution := newSessionMCPExecution(client)
	execution.Status = v1.AgentStatusRunning
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.applyPendingSessionMCP(ctx, execution.SessionID); err != nil {
		t.Fatal(err)
	}
	select {
	case action := <-actions:
		t.Fatalf("active turn reconfigured: %s", action)
	default:
	}
	execution.Status = v1.AgentStatusReady
	applyErr := mgr.applyPendingSessionMCP(ctx, execution.SessionID)
	if !fail && applyErr != nil {
		t.Fatal(applyErr)
	}
	var payload struct {
		Servers []types.McpServer `json:"mcp_servers"`
	}
	select {
	case raw := <-captured:
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("idle application did not reach provider")
	}
	for _, server := range payload.Servers {
		if server.Name == "docs" {
			t.Fatal("excluded server delivered")
		}
	}
	state, err := repo.GetMCPSelectionState(ctx, execution.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if fail {
		if state.ApplyState != mcpconfig.SessionMCPApplyStateFailed || state.AppliedRevision != 1 || state.AcceptedSetup.Servers[0].DefinitionID != "docs" {
			t.Fatalf("failure lost baseline: %#v", state)
		}
	} else if state.AppliedRevision != 2 || state.AcceptedSetup == nil || len(state.AcceptedSetup.Servers) != 0 {
		t.Fatalf("wrong accepted setup: %#v", state)
	}
}
