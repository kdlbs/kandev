package mcpconfig

import (
	"context"
	"errors"
	"testing"
)

func TestResolverComposesSelectionsAndAccumulatesOrigins(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	first, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "github", DisplayName: "GitHub",
		ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP,
		Configuration: MCPServerConfiguration{URL: "https://github.example.test/mcp"},
	})
	if err != nil {
		t.Fatalf("create first definition: %v", err)
	}
	second, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "linear", DisplayName: "Linear",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "linear-mcp"},
	})
	if err != nil {
		t.Fatalf("create second definition: %v", err)
	}
	selectionRepo := newSelectionRepositoryFake()
	selections := NewSelectionService(selectionRepo, catalogRepo)
	if err := selections.Replace(context.Background(), SelectionScopeRepository, "workspace-1", "repo-1", []string{first.ID}); err != nil {
		t.Fatalf("repository selection: %v", err)
	}
	if err := selections.Replace(context.Background(), SelectionScopeProfile, "workspace-1", "profile-1", []string{first.ID, second.ID}); err != nil {
		t.Fatalf("profile selection: %v", err)
	}
	if err := selections.Replace(context.Background(), SelectionScopeTaskSession, "workspace-1", "session-1", []string{second.ID}); err != nil {
		t.Fatalf("session selection: %v", err)
	}

	resolver := NewResolver(catalogRepo, selectionRepo)
	result, err := resolver.Resolve(context.Background(), ResolutionContext{
		WorkspaceID: "workspace-1", RepositoryIDs: []string{"repo-1"},
		ProfileID: "profile-1", SessionID: "session-1",
	}, Policy{AllowHTTP: true, AllowStdio: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(result.Servers) != 2 || result.Servers[0].Name != "github" || result.Servers[1].Name != "linear" {
		t.Fatalf("resolved servers = %#v", result.Servers)
	}
	if len(result.Servers[0].Origins) != 2 || len(result.Servers[1].Origins) != 2 {
		t.Fatalf("origins = %#v", result.Servers)
	}
	if result.Servers[0].DefinitionID != first.ID || result.Servers[0].DefinitionRevision != first.Revision {
		t.Fatalf("first identity = %#v", result.Servers[0])
	}
}

func TestResolverFiltersDisabledAndReportsPolicyDecisions(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	disabled := false
	definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "tools", DisplayName: "Tools",
		Enabled: &disabled, ExecutionMode: ExecutionModeExistingExecutable,
		Transport: ServerTypeStdio, Configuration: MCPServerConfiguration{Command: "tools"},
	})
	if err != nil {
		t.Fatalf("create disabled definition: %v", err)
	}
	selectionRepo := newSelectionRepositoryFake()
	selectionRepo.values[selectionRepo.key(SelectionScopeProfile, "workspace-1", "profile-1")] = []string{definition.ID}
	resolver := NewResolver(catalogRepo, selectionRepo)
	result, err := resolver.Resolve(context.Background(), ResolutionContext{
		WorkspaceID: "workspace-1", ProfileID: "profile-1",
	}, Policy{AllowStdio: false})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(result.Servers) != 0 || len(result.Decisions) != 1 || result.Decisions[0].ReasonCode != "definition_disabled" {
		t.Fatalf("result = %#v", result)
	}
}

func TestResolverRejectsEffectiveRuntimeNameCollision(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	first, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "first", DisplayName: "First",
		ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP,
		Configuration: MCPServerConfiguration{URL: "https://one.example.test"},
	})
	if err != nil {
		t.Fatalf("create first definition: %v", err)
	}
	second := cloneDefinition(first)
	second.ID = "second"
	second.RuntimeName = "second"
	second.NormalizedRuntimeName = "first"
	catalogRepo.definitions[second.ID] = second
	selectionRepo := newSelectionRepositoryFake()
	selectionRepo.values[selectionRepo.key(SelectionScopeProfile, "workspace-1", "profile-1")] = []string{first.ID, second.ID}
	resolver := NewResolver(catalogRepo, selectionRepo)
	_, err = resolver.Resolve(context.Background(), ResolutionContext{
		WorkspaceID: "workspace-1", ProfileID: "profile-1",
	}, Policy{AllowHTTP: true})
	if !errors.Is(err, ErrMCPEffectiveNameCollision) {
		t.Fatalf("Resolve error = %v, want name collision", err)
	}
}

func TestResolverKeepsSecretValuesOutOfDecisionsAndUsesDeliveryOnly(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "private", DisplayName: "Private",
		ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP,
		Configuration:  MCPServerConfiguration{URL: "https://private.example.test"},
		SecretBindings: []MCPSecretBinding{{InputName: "Authorization", SecretID: "secret-1"}},
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	selectionRepo := newSelectionRepositoryFake()
	selectionRepo.values[selectionRepo.key(SelectionScopeProfile, "workspace-1", "profile-1")] = []string{definition.ID}
	resolver := NewResolver(catalogRepo, selectionRepo)
	resolver.SetSecretResolver(func(context.Context, string, string) (string, error) {
		return "Bearer top-secret", nil
	})
	result, err := resolver.Resolve(context.Background(), ResolutionContext{
		WorkspaceID: "workspace-1", ProfileID: "profile-1",
	}, Policy{AllowHTTP: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Servers[0].Headers["Authorization"] != "Bearer top-secret" {
		t.Fatalf("delivery headers = %#v", result.Servers[0].Headers)
	}
	if len(result.Servers[0].Origins) != 1 || result.Servers[0].Origins[0].OwnerID != "profile-1" {
		t.Fatalf("origins = %#v", result.Servers[0].Origins)
	}
}

func TestResolverDoesNotDuplicateTypedSelectionDuringLegacyFallback(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
		WorkspaceID: "workspace-1", RuntimeName: "github", DisplayName: "GitHub",
		ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
		Configuration: MCPServerConfiguration{Command: "github-mcp"},
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	selectionRepo := newSelectionRepositoryFake()
	selectionRepo.values[selectionRepo.key(SelectionScopeProfile, "workspace-1", "profile-1")] = []string{definition.ID}
	resolver := NewResolver(catalogRepo, selectionRepo)
	resolver.SetLegacyProvider(&legacyConfigReaderFake{config: &ProfileConfig{
		Enabled: true, Servers: map[string]ServerDef{
			"GitHub": {Type: ServerTypeStdio, Command: "legacy-github"},
		},
	}}, nil)

	result, err := resolver.Resolve(context.Background(), ResolutionContext{
		WorkspaceID: "workspace-1", ProfileID: "profile-1",
	}, Policy{AllowStdio: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(result.Servers) != 1 || result.Servers[0].DefinitionID != definition.ID {
		t.Fatalf("resolved servers = %#v", result.Servers)
	}
}
