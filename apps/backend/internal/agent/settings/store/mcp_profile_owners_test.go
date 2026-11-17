package store

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-002.1
func TestMCPProfileOwnersMetadata(t *testing.T) {
	testMCPProfileOwnersMetadata(t, newTestRepo(t).(*sqliteRepository))
}
func TestPostgresMCPProfileOwners(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	require.NoError(t, err)
	testMCPProfileOwnersMetadata(t, repo)
}
func testMCPProfileOwnersMetadata(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	global := seedAgentProfile(t, repo, "Global", "agent-global")
	local := seedAgentProfile(t, repo, "Workspace", "agent-local")
	foreign := seedAgentProfile(t, repo, "Foreign", "agent-foreign")
	removed := seedAgentProfile(t, repo, "Deleted", "agent-deleted")
	_, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_profiles SET workspace_id = 'w', enabled = 0 WHERE id = ?`), local)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_profiles SET workspace_id = 'other' WHERE id = ?`), foreign)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_profiles SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`), removed)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_profiles SET env_vars = 'private malformed config', command_prefix = 'private-launcher' WHERE id = ?`), local)
	require.NoError(t, err)
	reader, ok := any(repo).(mcpconfig.MCPProfileOwnerRepository)
	require.True(t, ok, "store must provide metadata-only profile inventory")
	first, err := reader.ListMCPProfileOwners(ctx, "w", "", 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := reader.ListMCPProfileOwners(ctx, "w", first[0].ID, 1)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.ElementsMatch(t, []string{global, local}, []string{first[0].ID, second[0].ID})
	owner, err := reader.GetMCPProfileOwner(ctx, "w", local)
	require.NoError(t, err)
	require.Equal(t, "w", owner.WorkspaceID)
	require.False(t, owner.Enabled)
	require.Equal(t, "agent-local", owner.AgentName)
	agent, err := repo.GetAgentByName(ctx, "agent-local")
	require.NoError(t, err)
	require.Equal(t, agent.ID, owner.AgentID)
	payload, err := json.Marshal(owner)
	require.NoError(t, err)
	for _, key := range []string{"env_vars", "command_prefix", "provider_api_key", "configuration", "model", "private"} {
		require.NotContains(t, string(payload), key)
	}
	for _, id := range []string{foreign, removed, "missing"} {
		owner, err = reader.GetMCPProfileOwner(ctx, "w", id)
		require.NoError(t, err)
		require.Nil(t, owner)
	}
}
