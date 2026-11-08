package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.3
func TestMCPSelectionRetryPreservesAcceptedState(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: "workspace", OwnerID: "s"}
	setup := &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}
	if err := repo.SaveAcceptedMCPSetup(ctx, "s", 4, "attempt", setup); err != nil {
		t.Fatal(err)
	}
	revision := int64(4)
	result, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision})
	if err != nil {
		t.Fatal(err)
	}
	if result.State.DesiredRevision != 5 || result.State.AppliedRevision != 4 || result.State.AttachmentAttemptID != "attempt" || result.State.AcceptedSetup == nil {
		t.Fatalf("lost accepted evidence: %#v", result.State)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision}); !errors.Is(err, mcpconfig.ErrMCPSelectionConflict) {
		t.Fatalf("stale retry: %v", err)
	}
}

func TestMCPSelectionDisabledExistingMembership(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeRepository, WorkspaceID: def.WorkspaceID, OwnerID: "r"}
	ids := []string{def.ID}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, mcpconfig.SelectionCondition{}); err != nil {
		t.Fatal(err)
	}
	def.Enabled = false
	if err := repo.UpdateMCPServerDefinition(ctx, def, def.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &ids}); err != nil {
		t.Fatalf("retaining disabled entry: %v", err)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &ids}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, mcpconfig.SelectionCondition{}); !errors.Is(err, mcpconfig.ErrMCPDefinitionDisabled) {
		t.Fatalf("adding disabled entry: %v", err)
	}
}

func TestMCPSelectionDisabledProfileCannotAdd(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	profileID := seedAgentProfile(t, repo, "disabled", "agent-disabled")
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profileID, false); err != nil {
		t.Fatal(err)
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeProfile, WorkspaceID: def.WorkspaceID, OwnerID: profileID}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, []string{def.ID}, mcpconfig.SelectionCondition{}); !errors.Is(err, mcpconfig.ErrMCPSelectionOwnerAccess) {
		t.Fatalf("disabled owner accepted addition: %v", err)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{}); err != nil {
		t.Fatalf("cleanup denied: %v", err)
	}
}

func TestMCPSelectionCommittedResponse(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: def.WorkspaceID, OwnerID: "s"}
	service := mcpconfig.NewSelectionService(repo, repo)
	service.SetSessionMCPStateRepository(repo)
	service.SetSessionMCPChangeNotifier(func(ctx context.Context, _ string) {
		if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{}); err != nil {
			t.Error(err)
		}
	})
	response, err := service.ReplaceConditional(ctx, origin, []string{def.ID}, mcpconfig.SelectionCondition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.DefinitionIDs) != 1 || response.DefinitionIDs[0] != def.ID || response.State.DesiredRevision != 1 {
		t.Fatalf("response belongs to later writer: %#v", response)
	}
	latest, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil || len(latest.DefinitionIDs) != 0 || latest.State.DesiredRevision != 2 {
		t.Fatalf("later writer did not commit: %#v %v", latest, err)
	}
}
