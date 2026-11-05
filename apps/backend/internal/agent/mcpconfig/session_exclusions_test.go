package mcpconfig

import (
	"context"
	"testing"
)

type sessionExclusionInputs struct {
	*selectionRepositoryFake
	inputs *ResolutionInputs
}

func (r *sessionExclusionInputs) ReadMCPResolutionInputs(context.Context, ResolutionContext) (*ResolutionInputs, error) {
	return r.inputs, nil
}

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.7, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6
func TestSessionExclusionKeepsLegacyAliasSuppressed(t *testing.T) {
	ctx := context.Background()
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	def, err := catalog.Create(ctx, CreateDefinitionInput{WorkspaceID: "workspace", RuntimeName: "github", DisplayName: "GitHub", ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio, Configuration: MCPServerConfiguration{Command: "catalog-command"}})
	if err != nil {
		t.Fatal(err)
	}
	origin := SelectionOrigin{Scope: SelectionScopeProfile, WorkspaceID: "workspace", OwnerID: "profile"}
	repo := &sessionExclusionInputs{selectionRepositoryFake: newSelectionRepositoryFake(), inputs: &ResolutionInputs{Origins: map[string][]SelectionOrigin{def.ID: {origin}}, Definitions: map[string]*MCPServerDefinition{def.ID: def}, ExcludedDefinitionIDs: []string{def.ID}}}
	resolver := NewResolver(catalogRepo, repo)
	resolver.SetLegacyProvider(&legacyConfigReaderFake{config: &ProfileConfig{Enabled: true, Servers: map[string]ServerDef{"GitHub": {Type: ServerTypeStdio, Command: "legacy-command"}, "independent": {Type: ServerTypeStdio, Command: "independent"}}}}, nil)
	result, err := resolver.Resolve(ctx, ResolutionContext{WorkspaceID: "workspace", ProfileID: "profile", SessionID: "session"}, Policy{AllowStdio: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Servers) != 1 || result.Servers[0].Name != "independent" || len(result.Decisions) != 1 || result.Decisions[0].ReasonCode != "session_excluded" {
		t.Fatalf("legacy fallback restored excluded alias or lost independent config: %#v", result)
	}
}
