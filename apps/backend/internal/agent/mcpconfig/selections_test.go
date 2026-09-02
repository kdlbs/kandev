package mcpconfig

import (
	"context"
	"errors"
	"testing"
)

type selectionRepositoryFake struct {
	values       map[string][]string
	replaceCalls int
}

func newSelectionRepositoryFake() *selectionRepositoryFake {
	return &selectionRepositoryFake{values: make(map[string][]string)}
}

func (r *selectionRepositoryFake) key(scope SelectionScope, workspaceID, ownerID string) string {
	return string(scope) + ":" + workspaceID + ":" + ownerID
}

func (r *selectionRepositoryFake) ListMCPSelections(_ context.Context, scope SelectionScope, workspaceID, ownerID string) ([]string, error) {
	return append([]string(nil), r.values[r.key(scope, workspaceID, ownerID)]...), nil
}

func (r *selectionRepositoryFake) ReplaceMCPSelections(_ context.Context, scope SelectionScope, workspaceID, ownerID string, definitionIDs []string) error {
	r.replaceCalls++
	r.values[r.key(scope, workspaceID, ownerID)] = append([]string(nil), definitionIDs...)
	return nil
}

func (r *selectionRepositoryFake) SelectionImpact(context.Context, string, string) (SelectionImpact, error) {
	return SelectionImpact{}, nil
}

func (r *selectionRepositoryFake) DeleteMCPSelectionsForDefinition(context.Context, string, string) error {
	return nil
}

func TestSelectionServiceScopesGlobalProfileByWorkspace(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	first, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "first", DisplayName: "First",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "first"},
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-2", RuntimeName: "second", DisplayName: "Second",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "second"},
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	repo := newSelectionRepositoryFake()
	service := NewSelectionService(repo, catalogRepo)
	service.SetOwnerValidator(func(_ context.Context, scope SelectionScope, workspaceID, ownerID string) error {
		if scope != SelectionScopeProfile || ownerID != "global-profile" || (workspaceID != "workspace-1" && workspaceID != "workspace-2") {
			return errors.New("invalid profile context")
		}
		return nil
	})
	if err := service.Replace(context.Background(), SelectionScopeProfile, "workspace-1", "global-profile", []string{first.ID}); err != nil {
		t.Fatalf("workspace 1 selection: %v", err)
	}
	if err := service.Replace(context.Background(), SelectionScopeProfile, "workspace-2", "global-profile", []string{second.ID}); err != nil {
		t.Fatalf("workspace 2 selection: %v", err)
	}
	firstIDs, _ := service.List(context.Background(), SelectionScopeProfile, "workspace-1", "global-profile")
	secondIDs, _ := service.List(context.Background(), SelectionScopeProfile, "workspace-2", "global-profile")
	if len(firstIDs) != 1 || firstIDs[0] != first.ID || len(secondIDs) != 1 || secondIDs[0] != second.ID {
		t.Fatalf("workspace selections = %#v/%#v", firstIDs, secondIDs)
	}
}

func TestSelectionServiceRejectsCrossWorkspaceAndDisabledDefinitionsAtomically(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "tools", DisplayName: "Tools",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "tools"},
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	definition.Enabled = false
	catalogRepo.definitions[definition.ID] = definition
	repo := newSelectionRepositoryFake()
	service := NewSelectionService(repo, catalogRepo)
	if err := service.Replace(context.Background(), SelectionScopeTask, "workspace-2", "task-1", []string{definition.ID}); !errors.Is(err, ErrMCPSelectionWorkspaceMismatch) {
		t.Fatalf("cross-workspace error = %v", err)
	}
	if err := service.Replace(context.Background(), SelectionScopeTask, "workspace-1", "task-1", []string{definition.ID}); !errors.Is(err, ErrMCPDefinitionDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	if repo.replaceCalls != 0 {
		t.Fatalf("replace calls = %d, want no partial writes", repo.replaceCalls)
	}
}

func TestSelectionServiceDeduplicatesIDsBeforeAtomicReplace(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "tools", DisplayName: "Tools",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "tools"},
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	repo := newSelectionRepositoryFake()
	service := NewSelectionService(repo, catalogRepo)
	if err := service.Replace(context.Background(), SelectionScopeTask, "workspace-1", "task-1", []string{definition.ID, definition.ID}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	selected, _ := repo.ListMCPSelections(context.Background(), SelectionScopeTask, "workspace-1", "task-1")
	if len(selected) != 1 || selected[0] != definition.ID || repo.replaceCalls != 1 {
		t.Fatalf("selected = %#v, replace calls = %d", selected, repo.replaceCalls)
	}
}
