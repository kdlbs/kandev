package store

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPStoreResolution(t *testing.T) {
	assertAutomationMCPStoreResolution(t, newTestRepo(t).(*sqliteRepository))
}
func TestPostgresAutomationMCPStoreResolution(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database, nil)
	require.NoError(t, err)
	assertAutomationMCPStoreResolution(t, repo)
}
func assertAutomationMCPStoreResolution(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	definition := testMCPDefinition()
	require.NoError(t, repo.CreateMCPServerDefinition(t.Context(), definition))
	rc := mcpconfig.ResolutionContext{WorkspaceID: definition.WorkspaceID, TaskID: "task", Automation: &mcpconfig.AutomationMCPContribution{AutomationID: "automation", WorkspaceID: definition.WorkspaceID, TaskID: "task", Revision: 1, DefinitionIDs: []string{definition.ID}}}
	resolver := mcpconfig.NewResolver(repo, repo)
	resolver.SetSecretResolver(func(_ context.Context, secretID, workspaceID string) (string, error) {
		require.Equal(t, definition.WorkspaceID, workspaceID)
		require.Equal(t, "secret-1", secretID)
		return "automation-secret", nil
	})
	result, err := resolver.Resolve(t.Context(), rc, mcpconfig.Policy{AllowHTTP: true, AllowStdio: true})
	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Len(t, result.Servers[0].Origins, 1)
	require.Equal(t, "automation", string(result.Servers[0].Origins[0].Scope))
	rc.Automation.WorkspaceID = "foreign"
	_, err = resolver.Resolve(t.Context(), rc, mcpconfig.Policy{AllowHTTP: true})
	require.ErrorContains(t, err, "automation MCP contribution owner mismatch")
}
