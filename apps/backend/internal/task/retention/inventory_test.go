package retention

import (
	"context"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetainedInventorySurvivesEveryJobState(t *testing.T) {
	for _, state := range []string{"prepared", "pending", "running", "succeeded", "failed", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			db, err := sqlx.Open("sqlite3", ":memory:")
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			db.MustExec("CREATE TABLE task_resource_cleanup_jobs (task_id TEXT, state TEXT, resource_snapshot TEXT, workspace_protection TEXT DEFAULT 'unclassified')")
			db.MustExec("INSERT INTO task_resource_cleanup_jobs (task_id,state,resource_snapshot) VALUES (?, ?, ?)", "deleted", state, `{"workspace_cleanup":"retain","retention_inventory_captured":true,"task_environment":{"workspace_path":"/tasks/kept","container_id":"container"},"worktrees":[{"id":"worktree","path":"/tasks/kept/repo"}]}`)
			inv, err := Load(context.Background(), db)
			require.NoError(t, err)
			require.True(t, inv.Protects("deleted", "", ""))
			require.True(t, inv.Protects("", "worktree", ""))
			require.True(t, inv.Protects("", "", "/tasks/kept/repo"))
			require.Contains(t, inv.ResourceIDs, "container")
			require.NotContains(t, inv.ResourceIDs, "")
			require.NotContains(t, inv.WorktreeIDs, "")
		})
	}
}
func TestRetentionInventoryFailsClosed(t *testing.T) {
	for _, payload := range []string{`{`, `{"workspace_cleanup":"unknown"}`, `{"workspace_cleanup":"retain"}`} {
		db, err := sqlx.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		db.MustExec("CREATE TABLE task_resource_cleanup_jobs (task_id TEXT, resource_snapshot TEXT, state TEXT DEFAULT 'pending', workspace_protection TEXT DEFAULT 'unclassified')")
		db.MustExec("INSERT INTO task_resource_cleanup_jobs (task_id,resource_snapshot) VALUES (?, ?)", "task", payload)
		_, err = Load(context.Background(), db)
		require.Error(t, err)
		_ = db.Close()
	}
}

func TestRetainedTaskProtectsEarlierCleanupInventory(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.MustExec("CREATE TABLE task_resource_cleanup_jobs (task_id TEXT, resource_snapshot TEXT, state TEXT DEFAULT 'pending', workspace_protection TEXT DEFAULT 'unclassified')")
	db.MustExec("INSERT INTO task_resource_cleanup_jobs (task_id,resource_snapshot) VALUES (?, ?)", "task", `{"worktrees":[{"id":"old","path":"/tasks/old"}]}`)
	db.MustExec("INSERT INTO task_resource_cleanup_jobs (task_id,resource_snapshot) VALUES (?, ?)", "task", `{"workspace_cleanup":"retain","retention_inventory_captured":true}`)
	inventory, err := Load(context.Background(), db)
	require.NoError(t, err)
	require.True(t, inventory.Protects("", "", "/tasks/old"))
}

func TestRetainedRelativePathMakesInventoryUnavailable(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.MustExec("CREATE TABLE task_resource_cleanup_jobs (task_id TEXT, resource_snapshot TEXT, state TEXT DEFAULT 'pending', workspace_protection TEXT DEFAULT 'unclassified')")
	db.MustExec("INSERT INTO task_resource_cleanup_jobs (task_id,resource_snapshot) VALUES (?, ?)", "task", `{"workspace_cleanup":"retain","retention_inventory_captured":true,"worktrees":[{"id":"unknown","path":"relative"}]}`)
	_, err = Load(context.Background(), db)
	require.Error(t, err)
}
