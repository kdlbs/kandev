package mcpconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinNamesCannotBeCreatedByClaimingLegacySource(t *testing.T) {
	service := NewCatalogService(newCatalogRepositoryFake())
	for _, name := range []string{"kandev-configuration", "KANDEV-DIAGNOSTICS", "kandev-rich-output", "kandev-walkthroughs"} {
		_, err := service.Create(context.Background(), CreateDefinitionInput{WorkspaceID: "workspace", RuntimeName: name, DisplayName: "Imported", Source: DefinitionSourceLegacyImport, ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP, Configuration: MCPServerConfiguration{URL: "https://example.test/mcp"}})
		require.ErrorIs(t, err, ErrMCPRuntimeNameReserved, name)
	}
}

func TestBuiltinNameCollisionSurvivesLegacyImportForRename(t *testing.T) {
	reader := &legacyConfigReaderFake{config: &ProfileConfig{Enabled: true, Servers: map[string]ServerDef{
		"kandev-configuration": {Type: ServerTypeHTTP, URL: "https://example.test/owned"},
	}}}
	repo := newCatalogRepositoryFake()
	catalog := NewCatalogService(repo)
	selections := NewSelectionService(newSelectionRepositoryFake(), repo)
	importer := NewLegacyImporter(reader, &legacyWorkspaceListerFake{workspaceIDs: []string{"workspace"}}, catalog, selections, newLegacyImportStateFake())
	_, err := importer.ImportProfile(context.Background(), "profile")
	require.NoError(t, err)
	definitions, err := catalog.List(context.Background(), "workspace")
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "kandev-configuration", definitions[0].RuntimeName)
	require.Equal(t, "https://example.test/owned", definitions[0].Configuration.URL)
	newName := "my-configuration"
	updated, err := catalog.Update(context.Background(), UpdateDefinitionInput{WorkspaceID: "workspace", ID: definitions[0].ID, ExpectedRevision: definitions[0].Revision, RuntimeName: &newName})
	require.NoError(t, err)
	require.Equal(t, newName, updated.RuntimeName)
}
