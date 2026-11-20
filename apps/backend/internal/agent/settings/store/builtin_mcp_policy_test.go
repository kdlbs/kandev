package store

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-003.1
func TestBuiltinStoreRevisionsDefaultsAndAtomicGrant(t *testing.T) {
	testBuiltinStoreRevisionsDefaultsAndAtomicGrant(t, newBuiltinStore(t))
}

func testBuiltinStoreRevisionsDefaultsAndAtomicGrant(t *testing.T, repo *sqliteRepository) {
	store, ok := any(repo).(mcpconfig.BuiltinPolicyRepository)
	require.True(t, ok, "settings store must implement atomic built-in policy persistence")
	ctx := context.Background()
	initial, err := store.GetBuiltinPolicySnapshot(ctx, "workspace", "task")
	require.NoError(t, err)
	require.Zero(t, initial.Policy.Revision)
	require.Zero(t, initial.Preferences.Revision)
	policy := initial.Policy
	policy.UpdatedBy, policy.UpdatedAt = "human", time.Now().UTC()
	policy.Packages[p.PackageDiagnostics] = "available"
	saved, err := store.PutBuiltinWorkspacePolicy(ctx, policy, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, saved.Revision)
	_, err = store.PutBuiltinWorkspacePolicy(ctx, policy, 0)
	require.ErrorIs(t, err, mcpconfig.ErrBuiltinPolicyConflict)
	preferences := initial.Preferences
	preferences.UpdatedBy, preferences.UpdatedAt = "human", time.Now().UTC()
	preferences.Packages = mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}
	_, err = store.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.ErrorIs(t, err, mcpconfig.ErrBuiltinPolicyConflict)
	snapshot, err := store.PutBuiltinTaskPreferences(ctx, preferences, 0, 1, p.SurfaceKanbanTask)
	require.NoError(t, err)
	require.EqualValues(t, 1, snapshot.Preferences.Revision)
	unchanged, err := store.PutBuiltinTaskPreferences(ctx, preferences, 1, 1, p.SurfaceKanbanTask)
	require.NoError(t, err)
	require.EqualValues(t, 1, unchanged.Preferences.Revision, "no-op save keeps the revision")
	policy = snapshot.Policy
	policy.Packages[p.PackageConfiguration] = "blocked"
	_, err = store.PutBuiltinWorkspacePolicy(ctx, policy, 1)
	require.NoError(t, err)
	_, err = store.PutBuiltinTaskPreferences(ctx, preferences, 1, 2, p.SurfaceKanbanTask)
	require.ErrorIs(t, err, mcpconfig.ErrBuiltinPackageBlocked)
	current, err := store.GetBuiltinPolicySnapshot(ctx, "workspace", "task")
	require.NoError(t, err)
	require.Equal(t, "on", current.Preferences.Packages[p.PackageConfiguration], "workspace blocking preserves the saved task choice")
	manifest, err := mcpconfig.ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, current.Policy, current.Preferences)
	require.NoError(t, err)
	require.NotContains(t, manifest.Packages, p.PackageConfiguration)
}

func TestBuiltinStoreRejectsForeignTaskAndCorruptPolicy(t *testing.T) {
	testBuiltinStoreRejectsForeignTaskAndCorruptPolicy(t, newBuiltinStore(t))
}

func testBuiltinStoreRejectsForeignTaskAndCorruptPolicy(t *testing.T, repo *sqliteRepository) {
	store, ok := any(repo).(mcpconfig.BuiltinPolicyRepository)
	require.True(t, ok)
	ctx := context.Background()
	_, err := store.GetBuiltinPolicySnapshot(ctx, "foreign", "task")
	require.Error(t, err)
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy("workspace")
	policy.Packages[p.PackageRichOutput] = "blocked"
	policy.UpdatedBy, policy.UpdatedAt = "human", time.Now().UTC()
	_, err = store.PutBuiltinWorkspacePolicy(ctx, policy, 0)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE workspace_builtin_mcp_policies SET packages_json = '{broken' WHERE workspace_id = 'workspace'`)
	require.NoError(t, err)
	snapshot, err := store.GetBuiltinPolicySnapshot(ctx, "workspace", "task")
	require.Error(t, err)
	require.Nil(t, snapshot, "storage failure must not become permissive defaults")
}

func newBuiltinStore(t *testing.T) *sqliteRepository {
	t.Helper()
	repo := newTestRepo(t).(*sqliteRepository)
	_, err := repo.db.Exec(`PRAGMA foreign_keys = ON`)
	require.NoError(t, err)
	seedBuiltinStore(t, repo)
	return repo
}

func seedBuiltinStore(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	tasks, err := tasksqlite.NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)
	require.NoError(t, tasks.CreateWorkspace(context.Background(), &models.Workspace{ID: "workspace", Name: "Built-in policy"}))
	require.NoError(t, tasks.CreateTask(context.Background(), &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Package choices"}))
}

func TestBuiltinStoreDoesNotInheritGrantsAndCleansDeletedOwners(t *testing.T) {
	testBuiltinStoreDoesNotInheritGrantsAndCleansDeletedOwners(t, newBuiltinStore(t))
}

func testBuiltinStoreDoesNotInheritGrantsAndCleansDeletedOwners(t *testing.T, repo *sqliteRepository) {
	store := mcpconfig.BuiltinPolicyRepository(repo)
	ctx := context.Background()
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy("workspace")
	policy.Packages[p.PackageDiagnostics] = "blocked"
	policy.UpdatedBy, policy.UpdatedAt = "human", time.Now().UTC()
	_, err := store.PutBuiltinWorkspacePolicy(ctx, policy, 0)
	require.NoError(t, err)
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err = store.PutBuiltinTaskPreferences(ctx, preferences, 0, 1, p.SurfaceKanbanTask)
	require.NoError(t, err)
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	require.NoError(t, tasks.CreateTask(ctx, &models.Task{ID: "child", ParentID: "task", WorkspaceID: "workspace", Title: "Child"}))
	child, err := store.GetBuiltinPolicySnapshot(ctx, "workspace", "child")
	require.NoError(t, err)
	require.Empty(t, child.Preferences.Packages)
	require.NoError(t, tasks.DeleteTask(ctx, "child"))
	require.NoError(t, tasks.DeleteTask(ctx, "task"))
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM task_builtin_mcp_preferences`))
	require.Zero(t, count)
	require.NoError(t, repo.DeleteMCPWorkspaceData(ctx, "workspace"))
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM workspace_builtin_mcp_policies`))
	require.Zero(t, count)
}

func TestBuiltinStorePostgres(t *testing.T) {
	for name, check := range map[string]func(*testing.T, *sqliteRepository){
		"revisions":    testBuiltinStoreRevisionsDefaultsAndAtomicGrant,
		"corrupt data": testBuiltinStoreRejectsForeignTaskAndCorruptPolicy,
		"cleanup":      testBuiltinStoreDoesNotInheritGrantsAndCleansDeletedOwners,
	} {
		t.Run(name, func(t *testing.T) {
			db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			repo, err := newSQLiteRepositoryWithDB(db, db, nil)
			require.NoError(t, err)
			seedBuiltinStore(t, repo)
			check(t, repo)
		})
	}
}
