package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestMCPServerDefinitionRoundTrip(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	definition := testMCPDefinition()
	ctx := context.Background()

	if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
		t.Fatalf("CreateMCPServerDefinition: %v", err)
	}
	got, err := repo.GetMCPServerDefinition(ctx, definition.WorkspaceID, definition.ID)
	if err != nil {
		t.Fatalf("GetMCPServerDefinition: %v", err)
	}
	if got.Configuration.URL != definition.Configuration.URL || got.Configuration.Options["timeout"] != float64(30) {
		t.Fatalf("configuration = %#v, want %#v", got.Configuration, definition.Configuration)
	}
	if got.SecretBindings[0].SecretID != "secret-1" || got.Revision != 1 {
		t.Fatalf("stored definition = %#v", got)
	}
}

func TestMCPServerDefinitionUpdateUsesRevisionAndWorkspace(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	definition := testMCPDefinition()
	ctx := context.Background()
	if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
		t.Fatalf("CreateMCPServerDefinition: %v", err)
	}

	definition.Description = "updated"
	definition.Revision = 2
	if err := repo.UpdateMCPServerDefinition(ctx, definition, 1); err != nil {
		t.Fatalf("UpdateMCPServerDefinition: %v", err)
	}
	if err := repo.UpdateMCPServerDefinition(ctx, definition, 1); err == nil {
		t.Fatal("stale update succeeded")
	} else {
		var conflict *mcpconfig.MCPRevisionConflictError
		if !errors.As(err, &conflict) || conflict.Current.Revision != 2 {
			t.Fatalf("stale update error = %v", err)
		}
	}
	if err := repo.DeleteMCPServerDefinition(ctx, "other-workspace", definition.ID, 2); !errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound) {
		t.Fatalf("cross-workspace delete error = %v", err)
	}
	if err := repo.DeleteMCPServerDefinition(ctx, definition.WorkspaceID, definition.ID, 2); err != nil {
		t.Fatalf("DeleteMCPServerDefinition: %v", err)
	}
}

func TestMCPServerDefinitionListIsWorkspaceScopedAndOrdered(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	for _, name := range []string{"zeta", "alpha"} {
		definition := testMCPDefinition()
		definition.ID = "id-" + name
		definition.RuntimeName = name
		definition.NormalizedRuntimeName = name
		if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
			t.Fatalf("CreateMCPServerDefinition(%s): %v", name, err)
		}
	}
	other := testMCPDefinition()
	other.ID = "other"
	other.WorkspaceID = "other-workspace"
	other.RuntimeName = "other"
	other.NormalizedRuntimeName = "other"
	if err := repo.CreateMCPServerDefinition(ctx, other); err != nil {
		t.Fatalf("Create other definition: %v", err)
	}
	definitions, err := repo.ListMCPServerDefinitions(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("ListMCPServerDefinitions: %v", err)
	}
	if len(definitions) != 2 || definitions[0].RuntimeName != "alpha" || definitions[1].RuntimeName != "zeta" {
		t.Fatalf("workspace definitions = %#v", definitions)
	}
}

func TestMCPSelectionsAreAtomicAndCatalogDeleteCleansThem(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	definition := testMCPDefinition()
	ctx := context.Background()
	if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
		t.Fatalf("CreateMCPServerDefinition: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, definition.WorkspaceID, "task-1", []string{definition.ID, definition.ID}); err != nil {
		t.Fatalf("ReplaceMCPSelections: %v", err)
	}
	impact, err := repo.SelectionImpact(ctx, definition.WorkspaceID, definition.ID)
	if err != nil {
		t.Fatalf("SelectionImpact: %v", err)
	}
	if impact.Task != 1 || impact.Total() != 1 {
		t.Fatalf("selection impact = %#v", impact)
	}
	catalog := mcpconfig.NewCatalogService(repo)
	catalog.SetSelectionRepository(repo)
	if err := catalog.Delete(ctx, definition.WorkspaceID, definition.ID, definition.Revision, false); err == nil {
		t.Fatal("delete without confirmation succeeded")
	} else {
		var impactErr *mcpconfig.MCPSelectionImpactError
		if !errors.As(err, &impactErr) || impactErr.Impact.Task != 1 {
			t.Fatalf("guarded delete error = %v", err)
		}
	}
	if err := catalog.Delete(ctx, definition.WorkspaceID, definition.ID, definition.Revision, true); err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	selected, err := repo.ListMCPSelections(ctx, mcpconfig.SelectionScopeTask, definition.WorkspaceID, "task-1")
	if err != nil {
		t.Fatalf("ListMCPSelections after delete: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("selections after delete = %#v", selected)
	}
}

func TestDeleteMCPTaskDataRemovesTaskAndSessionRows(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	definition := testMCPDefinition()
	ctx := context.Background()
	if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
		t.Fatalf("CreateMCPServerDefinition: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, definition.WorkspaceID, "task-1", []string{definition.ID}); err != nil {
		t.Fatalf("replace task-1 selection: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, definition.WorkspaceID, "task-2", []string{definition.ID}); err != nil {
		t.Fatalf("replace task-2 selection: %v", err)
	}
	state := mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, ApplyState: mcpconfig.SessionMCPApplyStateApplied}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, definition.WorkspaceID, "session-1", []string{definition.ID}, state); err != nil {
		t.Fatalf("replace session-1 selection: %v", err)
	}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, definition.WorkspaceID, "session-2", []string{definition.ID}, state); err != nil {
		t.Fatalf("replace session-2 selection: %v", err)
	}

	if err := repo.DeleteMCPTaskData(ctx, "task-1", []string{"session-1", "session-1", ""}); err != nil {
		t.Fatalf("DeleteMCPTaskData: %v", err)
	}
	assertMCPSelectionCount(t, repo, mcpconfig.SelectionScopeTask, "task-1", 0)
	assertMCPSelectionCount(t, repo, mcpconfig.SelectionScopeTask, "task-2", 1)
	assertMCPSelectionCount(t, repo, mcpconfig.SelectionScopeTaskSession, "session-1", 0)
	assertMCPSelectionCount(t, repo, mcpconfig.SelectionScopeTaskSession, "session-2", 1)
	if _, err := repo.GetMCPSelectionState(ctx, "session-1"); !errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		t.Fatalf("session-1 state = %v, want not found", err)
	}
	if got, err := repo.GetMCPSelectionState(ctx, "session-2"); err != nil || got != state {
		t.Fatalf("session-2 state = %#v, %v; want %#v", got, err, state)
	}
}

func TestCompareAndSwapMCPSelectionStateRejectsStaleRevision(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	state := mcpconfig.SessionMCPSelectionState{
		DesiredRevision: 2,
		ApplyState:      mcpconfig.SessionMCPApplyStatePendingIdle,
	}
	if err := repo.ReplaceMCPSelectionsAndState(
		ctx, mcpconfig.SelectionScopeTaskSession, "workspace-1", "session-1", nil, state,
	); err != nil {
		t.Fatalf("ReplaceMCPSelectionsAndState: %v", err)
	}
	state.ApplyState = mcpconfig.SessionMCPApplyStateApplied
	updated, err := repo.CompareAndSwapMCPSelectionState(ctx, "session-1", 2, state)
	if err != nil || !updated {
		t.Fatalf("first compare-and-swap = %v, %v; want true", updated, err)
	}
	state.DesiredRevision = 3
	state.ApplyState = mcpconfig.SessionMCPApplyStatePendingIdle
	if err := repo.SaveMCPSelectionState(ctx, "session-1", state); err != nil {
		t.Fatalf("SaveMCPSelectionState: %v", err)
	}
	state.ApplyState = mcpconfig.SessionMCPApplyStateFailed
	updated, err = repo.CompareAndSwapMCPSelectionState(ctx, "session-1", 2, state)
	if err != nil {
		t.Fatalf("stale compare-and-swap: %v", err)
	}
	if updated {
		t.Fatal("stale compare-and-swap succeeded")
	}
	current, err := repo.GetMCPSelectionState(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetMCPSelectionState: %v", err)
	}
	if current.DesiredRevision != 3 || current.ApplyState != mcpconfig.SessionMCPApplyStatePendingIdle {
		t.Fatalf("state after stale compare-and-swap = %#v", current)
	}
}

func TestDeleteMCPWorkspaceDataRemovesOnlyWorkspaceRows(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	workspaceDefinition := testMCPDefinition()
	otherDefinition := testMCPDefinition()
	otherDefinition.ID = "server-2"
	otherDefinition.WorkspaceID = "workspace-2"
	otherDefinition.RuntimeName = "other-tools"
	otherDefinition.NormalizedRuntimeName = "other-tools"
	for _, definition := range []*mcpconfig.MCPServerDefinition{workspaceDefinition, otherDefinition} {
		if err := repo.CreateMCPServerDefinition(ctx, definition); err != nil {
			t.Fatalf("CreateMCPServerDefinition(%s): %v", definition.ID, err)
		}
	}
	if err := repo.CreateAgent(ctx, &settingsmodels.Agent{ID: "agent-1", Name: "agent"}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := repo.CreateAgentProfile(ctx, &settingsmodels.AgentProfile{
		ID: "profile-1", AgentID: "agent-1", Name: "profile", AgentDisplayName: "Profile", WorkspaceID: "workspace-1",
	}); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeProfile, workspaceDefinition.WorkspaceID, "profile-1", []string{workspaceDefinition.ID}); err != nil {
		t.Fatalf("replace profile selection: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeRepository, workspaceDefinition.WorkspaceID, "repository-1", []string{workspaceDefinition.ID}); err != nil {
		t.Fatalf("replace repository selection: %v", err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, workspaceDefinition.WorkspaceID, "task-1", []string{workspaceDefinition.ID}); err != nil {
		t.Fatalf("replace task selection: %v", err)
	}
	state := mcpconfig.SessionMCPSelectionState{DesiredRevision: 3, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, workspaceDefinition.WorkspaceID, "session-1", []string{workspaceDefinition.ID}, state); err != nil {
		t.Fatalf("replace workspace session selection: %v", err)
	}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, otherDefinition.WorkspaceID, "session-2", []string{otherDefinition.ID}, state); err != nil {
		t.Fatalf("replace other session selection: %v", err)
	}
	if err := repo.SaveMCPImportState(ctx, mcpconfig.LegacyImportState{WorkspaceID: "workspace-1", ProfileID: "profile-1", Status: mcpconfig.LegacyImportStatusComplete}); err != nil {
		t.Fatalf("save workspace import state: %v", err)
	}
	if err := repo.SaveMCPImportState(ctx, mcpconfig.LegacyImportState{WorkspaceID: "workspace-2", ProfileID: "profile-2", Status: mcpconfig.LegacyImportStatusComplete}); err != nil {
		t.Fatalf("save other import state: %v", err)
	}

	if err := repo.DeleteMCPWorkspaceData(ctx, "workspace-1"); err != nil {
		t.Fatalf("DeleteMCPWorkspaceData: %v", err)
	}
	if definitions, err := repo.ListMCPServerDefinitions(ctx, "workspace-1"); err != nil || len(definitions) != 0 {
		t.Fatalf("workspace-1 definitions = %#v, %v; want empty", definitions, err)
	}
	for _, scopeOwner := range []struct {
		scope mcpconfig.SelectionScope
		owner string
	}{
		{mcpconfig.SelectionScopeProfile, "profile-1"},
		{mcpconfig.SelectionScopeRepository, "repository-1"},
		{mcpconfig.SelectionScopeTask, "task-1"},
		{mcpconfig.SelectionScopeTaskSession, "session-1"},
	} {
		assertMCPSelectionCount(t, repo, scopeOwner.scope, scopeOwner.owner, 0)
	}
	if _, err := repo.GetMCPSelectionState(ctx, "session-1"); !errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		t.Fatalf("workspace-1 apply state = %v, want not found", err)
	}
	if _, err := repo.GetMCPImportState(ctx, "workspace-1", "profile-1"); !errors.Is(err, mcpconfig.ErrMCPLegacyImportStateNotFound) {
		t.Fatalf("workspace-1 import state = %v, want not found", err)
	}
	if definitions, err := repo.ListMCPServerDefinitions(ctx, "workspace-2"); err != nil || len(definitions) != 1 || definitions[0].ID != otherDefinition.ID {
		t.Fatalf("workspace-2 definitions = %#v, %v; want other definition", definitions, err)
	}
	assertMCPSelectionCount(t, repo, mcpconfig.SelectionScopeTaskSession, "session-2", 1)
	if _, err := repo.GetMCPSelectionState(ctx, "session-2"); err != nil {
		t.Fatalf("workspace-2 apply state: %v", err)
	}
	if _, err := repo.GetMCPImportState(ctx, "workspace-2", "profile-2"); err != nil {
		t.Fatalf("workspace-2 import state: %v", err)
	}
}

func assertMCPSelectionCount(t *testing.T, repo *sqliteRepository, scope mcpconfig.SelectionScope, ownerID string, want int) {
	t.Helper()
	selected, err := repo.ListMCPSelections(context.Background(), scope, "workspace-1", ownerID)
	if err != nil {
		t.Fatalf("ListMCPSelections(%s, %s): %v", scope, ownerID, err)
	}
	if len(selected) != want {
		t.Fatalf("%s selections for %s = %#v, want %d", scope, ownerID, selected, want)
	}
}

func testMCPDefinition() *mcpconfig.MCPServerDefinition {
	return &mcpconfig.MCPServerDefinition{
		ID:                    "server-1",
		WorkspaceID:           "workspace-1",
		RuntimeName:           "tools",
		NormalizedRuntimeName: "tools",
		DisplayName:           "Tools",
		Enabled:               true,
		ExecutionMode:         mcpconfig.ExecutionModeRemote,
		Transport:             mcpconfig.ServerTypeHTTP,
		Configuration: mcpconfig.MCPServerConfiguration{
			URL:     "https://mcp.example.test",
			Options: map[string]any{"timeout": 30},
		},
		SecretBindings: []mcpconfig.MCPSecretBinding{{InputName: "Authorization", SecretID: "secret-1"}},
		Source:         mcpconfig.DefinitionSourceCustom,
		Revision:       1,
	}
}
