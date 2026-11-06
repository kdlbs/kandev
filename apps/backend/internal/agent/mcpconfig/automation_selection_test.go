package mcpconfig

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-OFFICE-AUTOMATION-MCP-001.2, AC-OFFICE-AUTOMATION-MCP-003.5
func TestAutomationMCPSelectionUnion(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	d, err := catalog.Create(t.Context(), CreateDefinitionInput{WorkspaceID: "workspace", RuntimeName: "reports", DisplayName: "Reports", ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP, Configuration: MCPServerConfiguration{URL: "https://reports.example.test/mcp"}})
	require.NoError(t, err)
	selections := newSelectionRepositoryFake()
	selections.values[selections.key(SelectionScopeProfile, "workspace", "profile")] = []string{d.ID}
	rc := ResolutionContext{WorkspaceID: "workspace", TaskID: "task", ProfileID: "profile", Automation: &AutomationMCPContribution{AutomationID: "automation", WorkspaceID: "workspace", TaskID: "task", Revision: 1, DefinitionIDs: []string{d.ID}}}
	result, err := NewResolver(catalogRepo, selections).Resolve(t.Context(), rc, Policy{AllowHTTP: true})
	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Len(t, result.Servers[0].Origins, 2)
	require.Equal(t, "automation", string(result.Servers[0].Origins[1].Scope))
}

func TestAutomationMCPDefinitionDeletionRequiresConfirmation(t *testing.T) {
	repo := newCatalogRepositoryFake()
	catalog := NewCatalogService(repo)
	writer, ok := any(catalog).(interface {
		SetAutomationUsageReader(func(context.Context, string) (map[string]int, error))
	})
	require.True(t, ok, "catalog deletion must include automation references")

	d, err := catalog.Create(t.Context(), CreateDefinitionInput{WorkspaceID: "workspace", RuntimeName: "reports", DisplayName: "Reports", ExecutionMode: ExecutionModeRemote, Transport: ServerTypeHTTP, Configuration: MCPServerConfiguration{URL: "https://reports.example.test/mcp"}})
	require.NoError(t, err)
	writer.SetAutomationUsageReader(func(context.Context, string) (map[string]int, error) { return map[string]int{d.ID: 1}, nil })
	require.ErrorIs(t, catalog.Delete(t.Context(), "workspace", d.ID, d.Revision, false), ErrMCPDefinitionSelected)
	_, err = catalog.Get(t.Context(), "workspace", d.ID)
	require.NoError(t, err)
	require.NoError(t, catalog.Delete(t.Context(), "workspace", d.ID, d.Revision, true))
	_, err = catalog.Get(t.Context(), "workspace", d.ID)
	require.ErrorIs(t, err, ErrMCPServerDefinitionNotFound)
}
