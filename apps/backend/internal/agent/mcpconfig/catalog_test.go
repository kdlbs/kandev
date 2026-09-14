package mcpconfig

import (
	"context"
	"errors"
	"testing"
	"time"
)

type catalogRepositoryFake struct {
	definitions map[string]*MCPServerDefinition
	created     int
	updated     int
	deleted     int
}

func newCatalogRepositoryFake() *catalogRepositoryFake {
	return &catalogRepositoryFake{definitions: make(map[string]*MCPServerDefinition)}
}

func (f *catalogRepositoryFake) ListMCPServerDefinitions(_ context.Context, workspaceID string) ([]*MCPServerDefinition, error) {
	var result []*MCPServerDefinition
	for _, definition := range f.definitions {
		if definition.WorkspaceID == workspaceID {
			result = append(result, cloneDefinition(definition))
		}
	}
	return result, nil
}

func (f *catalogRepositoryFake) GetMCPServerDefinition(_ context.Context, workspaceID, id string) (*MCPServerDefinition, error) {
	definition, ok := f.definitions[id]
	if !ok || definition.WorkspaceID != workspaceID {
		return nil, ErrMCPServerDefinitionNotFound
	}
	return cloneDefinition(definition), nil
}

func (f *catalogRepositoryFake) CreateMCPServerDefinition(_ context.Context, definition *MCPServerDefinition) error {
	f.created++
	f.definitions[definition.ID] = cloneDefinition(definition)
	return nil
}

func (f *catalogRepositoryFake) UpdateMCPServerDefinition(_ context.Context, definition *MCPServerDefinition, expectedRevision int64) error {
	current, ok := f.definitions[definition.ID]
	if !ok || current.WorkspaceID != definition.WorkspaceID {
		return ErrMCPServerDefinitionNotFound
	}
	if current.Revision != expectedRevision {
		return &MCPRevisionConflictError{Current: cloneDefinition(current)}
	}
	f.updated++
	f.definitions[definition.ID] = cloneDefinition(definition)
	return nil
}

func (f *catalogRepositoryFake) DeleteMCPServerDefinition(_ context.Context, workspaceID, id string, expectedRevision int64) error {
	definition, ok := f.definitions[id]
	if !ok || definition.WorkspaceID != workspaceID {
		return ErrMCPServerDefinitionNotFound
	}
	if definition.Revision != expectedRevision {
		return &MCPRevisionConflictError{Current: cloneDefinition(definition)}
	}
	f.deleted++
	delete(f.definitions, id)
	return nil
}

func TestCatalogServiceCreateValidatesWorkspaceDefinition(t *testing.T) {
	repo := newCatalogRepositoryFake()
	service := NewCatalogService(repo)
	service.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "workspace-1" {
			return errors.New("workspace not found")
		}
		return nil
	})

	definition, err := service.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   " Linear ",
		DisplayName:   "Linear",
		Description:   "Issue tools",
		ExecutionMode: ExecutionModeRemote,
		Transport:     ServerTypeHTTP,
		Configuration: MCPServerConfiguration{
			URL: "https://mcp.example.test",
		},
		SecretBindings: []MCPSecretBinding{{InputName: "Authorization", SecretID: "secret-1"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if definition.ID == "" || definition.Revision != 1 {
		t.Fatalf("definition identity/revision = %#v", definition)
	}
	if definition.RuntimeName != "Linear" || definition.NormalizedRuntimeName != "linear" {
		t.Fatalf("runtime names = %q/%q", definition.RuntimeName, definition.NormalizedRuntimeName)
	}
	if definition.Source != DefinitionSourceCustom || !definition.Enabled {
		t.Fatalf("source/enabled = %q/%v", definition.Source, definition.Enabled)
	}
	if len(definition.SecretBindings) != 1 || definition.SecretBindings[0].SecretID != "secret-1" {
		t.Fatalf("secret bindings = %#v", definition.SecretBindings)
	}
	if repo.created != 1 {
		t.Fatalf("created = %d, want one catalog write", repo.created)
	}
}

func TestCatalogServiceRejectsReservedAndDuplicateRuntimeNames(t *testing.T) {
	service := NewCatalogService(newCatalogRepositoryFake())
	input := CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   "kandev",
		DisplayName:   "Reserved",
		ExecutionMode: ExecutionModeRemote,
		Transport:     ServerTypeHTTP,
		Configuration: MCPServerConfiguration{URL: "https://mcp.example.test"},
	}
	if _, err := service.Create(context.Background(), input); !errors.Is(err, ErrMCPRuntimeNameReserved) {
		t.Fatalf("reserved name error = %v, want %v", err, ErrMCPRuntimeNameReserved)
	}

	input.RuntimeName = "Linear"
	if _, err := service.Create(context.Background(), input); err != nil {
		t.Fatalf("first definition: %v", err)
	}
	input.RuntimeName = " linear "
	if _, err := service.Create(context.Background(), input); !errors.Is(err, ErrMCPRuntimeNameConflict) {
		t.Fatalf("duplicate name error = %v, want %v", err, ErrMCPRuntimeNameConflict)
	}
}

func TestCatalogServiceRequiresExactManagedPackageVersion(t *testing.T) {
	service := NewCatalogService(newCatalogRepositoryFake())
	base := CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   "tools",
		DisplayName:   "Tools",
		ExecutionMode: ExecutionModeManagedPackage,
		Transport:     ServerTypeStdio,
		Configuration: MCPServerConfiguration{
			PackageType:    "npm",
			PackageName:    "@example/tools",
			PackageVersion: "^1.2.3",
		},
	}
	if _, err := service.Create(context.Background(), base); !errors.Is(err, ErrMCPInvalidDefinition) {
		t.Fatalf("range version error = %v, want %v", err, ErrMCPInvalidDefinition)
	}
	base.Configuration.PackageVersion = "1.2.3"
	if _, err := service.Create(context.Background(), base); err != nil {
		t.Fatalf("exact version: %v", err)
	}
}

func TestCatalogServiceRevisionConflictReturnsSanitizedCurrentDefinition(t *testing.T) {
	repo := newCatalogRepositoryFake()
	service := NewCatalogService(repo)
	created, err := service.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID:    "workspace-1",
		RuntimeName:    "secrets",
		DisplayName:    "Secrets",
		ExecutionMode:  ExecutionModeRemote,
		Transport:      ServerTypeHTTP,
		Configuration:  MCPServerConfiguration{URL: "https://mcp.example.test"},
		SecretBindings: []MCPSecretBinding{{InputName: "Authorization", SecretID: "secret-id"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	current := cloneDefinition(created)
	current.Revision = 2
	current.DisplayName = "Updated"
	repo.definitions[created.ID] = current

	_, err = service.Update(context.Background(), UpdateDefinitionInput{
		WorkspaceID:      "workspace-1",
		ID:               created.ID,
		ExpectedRevision: 1,
		DisplayName:      stringPointer("Stale write"),
	})
	var conflict *MCPRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Update error = %v, want revision conflict", err)
	}
	if conflict.Current == nil || conflict.Current.DisplayName != "Updated" {
		t.Fatalf("conflict current = %#v", conflict.Current)
	}
	if conflict.Current.SecretBindings[0].SecretID != "secret-id" {
		t.Fatalf("conflict secret reference = %#v", conflict.Current.SecretBindings)
	}
	if conflict.Current.Configuration.URL != "https://mcp.example.test" {
		t.Fatalf("conflict configuration = %#v", conflict.Current.Configuration)
	}
}

func TestCatalogServiceExistingExecutableDoesNotProbeOrExecute(t *testing.T) {
	repo := newCatalogRepositoryFake()
	service := NewCatalogService(repo)
	_, err := service.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   "local-tools",
		DisplayName:   "Local tools",
		ExecutionMode: ExecutionModeExistingExecutable,
		Transport:     ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "/not-installed/mcp"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if repo.created != 1 || repo.updated != 0 || repo.deleted != 0 {
		t.Fatalf("repository activity = created %d, updated %d, deleted %d", repo.created, repo.updated, repo.deleted)
	}
}

func TestCatalogServiceUpdateAndDelete(t *testing.T) {
	repo := newCatalogRepositoryFake()
	service := NewCatalogService(repo)
	created, err := service.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   "tools",
		DisplayName:   "Tools",
		ExecutionMode: ExecutionModeExistingExecutable,
		Transport:     ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "tools"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := service.Update(context.Background(), UpdateDefinitionInput{
		WorkspaceID:      "workspace-1",
		ID:               created.ID,
		ExpectedRevision: 1,
		Description:      stringPointer("Updated description"),
		Enabled:          boolPointer(false),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Revision != 2 || updated.Enabled || updated.Description != "Updated description" {
		t.Fatalf("updated definition = %#v", updated)
	}
	if err := service.Delete(context.Background(), "workspace-1", created.ID, 2, true); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deleted != 1 {
		t.Fatalf("deleted = %d, want one catalog delete", repo.deleted)
	}
}

func TestCatalogServiceCopiesMutableDefinitionFields(t *testing.T) {
	repo := newCatalogRepositoryFake()
	service := NewCatalogService(repo)
	args := []string{"--mode", "safe"}
	definition, err := service.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID:   "workspace-1",
		RuntimeName:   "tools",
		DisplayName:   "Tools",
		ExecutionMode: ExecutionModeExistingExecutable,
		Transport:     ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "tools", Args: args},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	args[0] = "mutated"
	definition.Configuration.Args[1] = "changed"
	stored, err := service.Get(context.Background(), "workspace-1", definition.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Configuration.Args[0] != "--mode" || stored.Configuration.Args[1] != "safe" {
		t.Fatalf("stored mutable fields = %#v", stored.Configuration.Args)
	}
}

func stringPointer(value string) *string { return &value }

func boolPointer(value bool) *bool { return &value }

var _ = time.Time{}
