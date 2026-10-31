package store

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-AGENTS-MCP-MANAGEMENT-001.1, AC-AGENTS-MCP-MANAGEMENT-001.3
func TestMCPCatalogUsageStore(t *testing.T) {
	testMCPCatalogUsageStore(t, newTestRepo(t).(*sqliteRepository))
}
func TestPostgresMCPCatalogUsage(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	require.NoError(t, err)
	testMCPCatalogUsageStore(t, repo)
	testMCPCatalogUsageProfileMetadata(t, repo)
}
func testMCPCatalogUsageStore(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	def := testMCPDefinition()
	def.Enabled = false
	require.NoError(t, repo.CreateMCPServerDefinition(ctx, def))
	profileID := seedAgentProfile(t, repo, "Named profile", "usage-agent")
	scopes := []mcpconfig.SelectionScope{mcpconfig.SelectionScopeProfile, mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession}
	for _, scope := range scopes {
		owner := "owner"
		if scope == mcpconfig.SelectionScopeProfile {
			owner = profileID
		}
		require.NoError(t, repo.ReplaceMCPSelections(ctx, scope, def.WorkspaceID, owner, []string{def.ID}))
	}
	// A stale profile association for a different workspace must not leak.
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO workspace_agent_profile_mcp_selections (workspace_id, profile_id, mcp_server_id) VALUES (?, ?, ?)`), "other", profileID, def.ID)
	require.NoError(t, err)
	usage, ok := any(repo).(mcpconfig.CatalogUsageRepository)
	require.True(t, ok, "store must expose read-only usage")
	first, err := usage.ListMCPUsageCandidates(ctx, def.WorkspaceID, def.ID, mcpconfig.UsageReference{}, 2)
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, mcpconfig.SelectionScopeProfile, first[0].Scope)
	require.Equal(t, mcpconfig.SelectionScopeRepository, first[1].Scope)
	next, err := usage.ListMCPUsageCandidates(ctx, def.WorkspaceID, def.ID, first[1], 2)
	require.NoError(t, err)
	require.Len(t, next, 2)
	require.Equal(t, mcpconfig.SelectionScopeTaskSession, next[1].Scope)
	other, err := usage.ListMCPUsageCandidates(ctx, "other", def.ID, mcpconfig.UsageReference{}, 100)
	require.NoError(t, err)
	require.Empty(t, other)
	selected, err := repo.ListMCPSelections(ctx, mcpconfig.SelectionScopeProfile, "other", profileID)
	require.NoError(t, err)
	require.Equal(t, []string{def.ID}, selected)
}

func TestMCPCatalogUsageProfileMetadata(t *testing.T) {
	testMCPCatalogUsageProfileMetadata(t, newTestRepo(t).(*sqliteRepository))
}

func testMCPCatalogUsageProfileMetadata(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "Visible profile", "metadata-agent")
	reader, ok := any(repo).(interface {
		GetMCPUsageProfile(context.Context, string, string) (*mcpconfig.UsageOwner, error)
	})
	require.True(t, ok, "usage must have a metadata-only profile reader")
	owner, err := reader.GetMCPUsageProfile(ctx, "workspace", id)
	require.NoError(t, err)
	require.Equal(t, "Visible profile", owner.DisplayName)
	require.Equal(t, "workspace", owner.WorkspaceID)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_profiles SET workspace_id = ?, enabled = 0 WHERE id = ?`), "workspace", id)
	require.NoError(t, err)
	owner, err = reader.GetMCPUsageProfile(ctx, "workspace", id)
	require.NoError(t, err)
	require.Equal(t, "disabled", owner.OwnerState)
	owner, err = reader.GetMCPUsageProfile(ctx, "other", id)
	require.NoError(t, err)
	require.Nil(t, owner)
	owner, err = reader.GetMCPUsageProfile(ctx, "workspace", "removed")
	require.NoError(t, err)
	require.Nil(t, owner)
}
