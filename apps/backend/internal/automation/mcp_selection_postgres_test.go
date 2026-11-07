package automation

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPSelectionPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(database, database)
	require.NoError(t, err)
	_, err = NewStore(database, database)
	require.NoError(t, err)
	a := &Automation{WorkspaceID: "workspace", Name: "MCP", MaxConcurrentRuns: 1, MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{"server"}}}
	require.NoError(t, store.CreateAutomation(t.Context(), a))
	service := &Service{store: store}
	count, err := service.CountMCPDefinitionSelections(t.Context(), a.WorkspaceID, "server")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	loaded, err := store.GetAutomation(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, a.MCPSelection, loaded.MCPSelection)
	stale := int64(0)
	req := &UpdateAutomationRequest{MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore}, DefinitionIDs: []string{}}, ExpectedMCPSelectionRevision: &stale}
	require.ErrorIs(t, store.UpdateAutomation(t.Context(), a.ID, req), ErrMCPSelectionConflict)
	current := int64(1)
	req.ExpectedMCPSelectionRevision = &current
	require.NoError(t, store.UpdateAutomation(t.Context(), a.ID, req))
	require.NoError(t, store.UpdateAutomation(t.Context(), a.ID, &UpdateAutomationRequest{}))
	loaded, err = store.GetAutomation(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), loaded.MCPSelectionRevision)
	count, err = service.CountMCPDefinitionSelections(t.Context(), a.WorkspaceID, "server")
	require.NoError(t, err)
	require.Zero(t, count)
	require.Equal(t, req.MCPSelection, loaded.MCPSelection)
}
