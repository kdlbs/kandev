package worktree

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
func TestSQLiteStore_CreateWorktreePlacement(t *testing.T) {
	testCreateWorktreePlacement(t, newTestStore(t))
}

func testCreateWorktreePlacement(t *testing.T, store *SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at, updated_at)
		VALUES ('workspace', 'Workspace', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	store.seedSessionWithEnvironment(t, "placement-session", "placement-task")
	for index, slug := range []string{"main", "topic"} {
		wt := &Worktree{
			ID: "placement-" + slug, SessionID: "placement-session", RepositoryID: "placement-repo",
			BranchSlug: slug, WorkspaceRelativePath: "kandev/" + slug, Position: index + 2,
			Path: filepath.Join(t.TempDir(), slug), Branch: "feature/" + slug,
		}
		require.NoError(t, store.CreateWorktree(ctx, wt))
		loaded, err := store.GetWorktreeByID(ctx, wt.ID)
		require.NoError(t, err)
		require.Equal(t, wt.WorkspaceRelativePath, loaded.WorkspaceRelativePath)
		require.Equal(t, wt.Position, loaded.Position)
		assertPlacementConflictPreserved(t, store, loaded)
	}
	for _, status := range []string{"failed", StatusDeleted} {
		now := time.Now().UTC()
		require.NoError(t, store.CreateWorktree(ctx, &Worktree{
			ID: status, SessionID: "placement-session", RepositoryID: "placement-repo", BranchSlug: status,
			WorkspaceRelativePath: "unrelated/" + status, Position: 9, Status: status, DeletedAt: &now,
		}))
	}
	for index, slug := range []string{"main", "topic"} {
		wt, err := store.GetWorktreeBySessionAndRepository(ctx, "placement-session", "placement-repo", slug)
		require.NoError(t, err)
		require.NotNil(t, wt)
		require.Equal(t, "kandev/"+slug, wt.WorkspaceRelativePath)
		require.Equal(t, index+2, wt.Position)
	}
}

func assertPlacementConflictPreserved(t *testing.T, store *SQLiteStore, original *Worktree) {
	t.Helper()
	ctx := context.Background()
	var rowID string
	require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind(
		`SELECT id FROM task_environment_repos WHERE worktree_id = ?`), original.ID).Scan(&rowID))
	for index, placement := range []string{"", "different/placement"} {
		update := *original
		update.WorkspaceRelativePath, update.Position = placement, index*99
		update.Path = original.Path + "-updated"
		require.NoError(t, store.CreateWorktree(ctx, &update))
		loaded, err := store.GetWorktreeByID(ctx, original.ID)
		require.NoError(t, err)
		require.Equal(t, original.WorkspaceRelativePath, loaded.WorkspaceRelativePath)
		require.Equal(t, original.Position, loaded.Position)
		require.Equal(t, update.Path, loaded.Path)
		var updatedID string
		require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind(
			`SELECT id FROM task_environment_repos WHERE worktree_id = ?`), original.ID).Scan(&updatedID))
		require.Equal(t, rowID, updatedID)
	}
}
