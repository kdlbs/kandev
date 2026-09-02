package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
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
