package mcpconfig

import (
	"context"
	"errors"
	"testing"
)

type legacyConfigReaderFake struct {
	config *ProfileConfig
	err    error
}

func (f *legacyConfigReaderFake) GetConfigByProfileID(context.Context, string) (*ProfileConfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.config, nil
}

type legacyWorkspaceListerFake struct{ workspaceIDs []string }

func (f *legacyWorkspaceListerFake) ListMCPProfileWorkspaces(context.Context, string) ([]string, error) {
	return append([]string(nil), f.workspaceIDs...), nil
}

type legacyImportStateFake struct {
	values map[string]LegacyImportState
}

func newLegacyImportStateFake() *legacyImportStateFake {
	return &legacyImportStateFake{values: make(map[string]LegacyImportState)}
}

func (f *legacyImportStateFake) key(workspaceID, profileID string) string {
	return workspaceID + ":" + profileID
}

func (f *legacyImportStateFake) GetMCPImportState(_ context.Context, workspaceID, profileID string) (LegacyImportState, error) {
	state, ok := f.values[f.key(workspaceID, profileID)]
	if !ok {
		return LegacyImportState{}, ErrMCPLegacyImportStateNotFound
	}
	return state, nil
}

func (f *legacyImportStateFake) SaveMCPImportState(_ context.Context, state LegacyImportState) error {
	f.values[f.key(state.WorkspaceID, state.ProfileID)] = state
	return nil
}

func TestLegacyImporterIsWorkspaceScopedAndIdempotent(t *testing.T) {
	reader := &legacyConfigReaderFake{config: &ProfileConfig{
		Enabled: true,
		Servers: map[string]ServerDef{
			"GitHub": {Type: ServerTypeStdio, Command: "github-mcp", Args: []string{"--safe"}},
		},
	}}
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	selectionRepo := newSelectionRepositoryFake()
	selections := NewSelectionService(selectionRepo, catalogRepo)
	states := newLegacyImportStateFake()
	importer := NewLegacyImporter(reader, &legacyWorkspaceListerFake{workspaceIDs: []string{"workspace-2", "workspace-1"}}, catalog, selections, states)

	results, err := importer.ImportProfile(context.Background(), "profile-global")
	if err != nil {
		t.Fatalf("ImportProfile: %v", err)
	}
	if len(results) != 2 || catalogRepo.created != 2 {
		t.Fatalf("results/created = %#v/%d", results, catalogRepo.created)
	}
	first, err := selections.List(context.Background(), SelectionScopeProfile, "workspace-1", "profile-global")
	if err != nil {
		t.Fatalf("workspace 1 selections: %v", err)
	}
	second, err := selections.List(context.Background(), SelectionScopeProfile, "workspace-2", "profile-global")
	if err != nil {
		t.Fatalf("workspace 2 selections: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0] == second[0] {
		t.Fatalf("workspace selections = %#v/%#v", first, second)
	}
	if _, err := importer.ImportProfile(context.Background(), "profile-global"); err != nil {
		t.Fatalf("retry ImportProfile: %v", err)
	}
	if catalogRepo.created != 2 {
		t.Fatalf("retry created %d definitions, want 2", catalogRepo.created)
	}
}

func TestLegacyImporterRedactsSecretsAndKeepsFallbackState(t *testing.T) {
	reader := &legacyConfigReaderFake{config: &ProfileConfig{Enabled: true, Servers: map[string]ServerDef{
		"private": {
			Type: ServerTypeStdio, Command: "private-mcp",
			Env:     map[string]string{"TOKEN": "do-not-store-this"},
			Headers: map[string]string{"Authorization": "also-secret"},
		},
	}}}
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	selectionRepo := newSelectionRepositoryFake()
	selections := NewSelectionService(selectionRepo, catalogRepo)
	states := newLegacyImportStateFake()
	importer := NewLegacyImporter(reader, nil, catalog, selections, states)

	result, err := importer.ImportProfileWorkspace(context.Background(), "workspace-1", "profile-1")
	if !errors.Is(err, ErrMCPLegacyImportRequiresRebind) {
		t.Fatalf("import error = %v", err)
	}
	if !result.Fallback || result.Complete || catalogRepo.created != 1 {
		t.Fatalf("import result/created = %#v/%d", result, catalogRepo.created)
	}
	definition := catalogRepo.definitions[result.DefinitionIDs[0]]
	if definition.Configuration.Env["TOKEN"] != "" || definition.Configuration.Headers["Authorization"] != "" {
		t.Fatalf("secret values persisted in configuration = %#v", definition.Configuration)
	}
	if len(selectionRepo.values) != 0 {
		t.Fatalf("unsafe import created selections = %#v", selectionRepo.values)
	}
	state := states.values[states.key("workspace-1", "profile-1")]
	if state.Status != LegacyImportStatusPending || state.FailureCode != "secret_rebind_required" {
		t.Fatalf("fallback state = %#v", state)
	}
	if got := definition.SecretBindings[0].InputName; got != "Authorization" {
		t.Fatalf("secret binding input name = %q, want raw header name", got)
	}
}

func TestLegacyDefinitionInputsSkipReservedNamesAndDisambiguateNormalizedNames(t *testing.T) {
	enabled := true
	inputs, ids, unsafe, err := legacyDefinitionInputs("workspace-1", "profile-1", &ProfileConfig{
		Enabled: enabled,
		Servers: map[string]ServerDef{
			"Kandev":       {Type: ServerTypeStdio, Command: "internal"},
			"Git Hub":      {Type: ServerTypeStdio, Command: "first"},
			"git-hub":      {Type: ServerTypeStdio, Command: "second"},
			"git-hub-2":    {Type: ServerTypeStdio, Command: "third"},
			"kandev.tools": {Type: ServerTypeStdio, Command: "internal"},
		},
	})
	if err != nil {
		t.Fatalf("legacyDefinitionInputs: %v", err)
	}
	if unsafe {
		t.Fatal("reserved and collision inputs should not be marked unsafe")
	}
	if len(inputs) != 3 || len(ids) != 3 {
		t.Fatalf("inputs/ids = %d/%d, want 3/3", len(inputs), len(ids))
	}
	if inputs[0].RuntimeName != "git-hub" || inputs[1].RuntimeName != "git-hub-2" || inputs[2].RuntimeName != "git-hub-2-2" {
		t.Fatalf("runtime names = %q, %q, %q", inputs[0].RuntimeName, inputs[1].RuntimeName, inputs[2].RuntimeName)
	}
}

func TestLegacyDefinitionInputsDisabledConfigIsEmpty(t *testing.T) {
	inputs, ids, unsafe, err := legacyDefinitionInputs("workspace-1", "profile-1", &ProfileConfig{
		Enabled: false,
		Servers: map[string]ServerDef{"server": {Type: ServerTypeStdio, Command: "server"}},
	})
	if err != nil {
		t.Fatalf("legacyDefinitionInputs: %v", err)
	}
	if len(inputs) != 0 || len(ids) != 0 || unsafe {
		t.Fatalf("disabled result = %#v/%#v/%v, want empty and safe", inputs, ids, unsafe)
	}
}
