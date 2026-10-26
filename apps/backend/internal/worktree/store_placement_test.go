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
	seedPlacementSession(t, store)
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

func seedPlacementSession(t *testing.T, store *SQLiteStore) {
	t.Helper()
	for _, seed := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		  VALUES (?, 'workspace', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, []any{"placement-task", "Placement"}},
		{`INSERT INTO task_environments (id, task_id, executor_type, status, workspace_path, created_at, updated_at)
		  VALUES (?, ?, 'worktree', 'ready', '/tmp/placement-task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			[]any{"env-placement-session", "placement-task"}},
		{`INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		  VALUES (?, ?, 'COMPLETED', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			[]any{"placement-session", "placement-task", "env-placement-session"}},
	} {
		_, err := store.db.ExecContext(context.Background(), store.db.Rebind(seed.query), seed.args...)
		require.NoError(t, err)
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
func TestSQLiteStore_CreateWorktreeReactivationPlacement(t *testing.T) {
	testCreateWorktreeReactivationPlacement(t, newTestStore(t))
}

func testCreateWorktreeReactivationPlacement(t *testing.T, store *SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at, updated_at)
		VALUES ('workspace', 'Workspace', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	seedPlacementSession(t, store)
	for _, tc := range []struct {
		name            string
		oldStatus       string
		oldDeleted      bool
		incomingStatus  string
		incomingDeleted bool
		emptyPlacement  bool
		replace         bool
	}{
		{name: "fully deleted", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: StatusActive, replace: true},
		{name: "empty and zero", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: StatusActive, emptyPlacement: true, replace: true},
		{name: "active", oldStatus: StatusActive, incomingStatus: StatusActive},
		{name: "deleted without time", oldStatus: StatusDeleted, incomingStatus: StatusActive},
		{name: "active with time", oldStatus: StatusActive, oldDeleted: true, incomingStatus: StatusActive},
		{name: "failed with time", oldStatus: "failed", oldDeleted: true, incomingStatus: StatusActive},
		{name: "incoming failed", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: "failed"},
		{name: "incoming deleted", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: StatusDeleted},
		{name: "incoming active with time", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: StatusActive, incomingDeleted: true},
		{name: "incoming deleted with time", oldStatus: StatusDeleted, oldDeleted: true, incomingStatus: StatusDeleted, incomingDeleted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
			old := &Worktree{
				ID: "old-" + tc.name, SessionID: "placement-session", RepositoryID: "placement-repo", BranchSlug: tc.name,
				Path: filepath.Join(t.TempDir(), "old"), Branch: "feature/old", Status: tc.oldStatus,
				WorkspaceRelativePath: "old/placement", Position: 4,
			}
			if tc.oldDeleted {
				old.DeletedAt = &deleted
			}
			require.NoError(t, store.CreateWorktree(ctx, old))
			var rowID string
			require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind(
				`SELECT id FROM task_environment_repos WHERE worktree_id = ?`), old.ID).Scan(&rowID))
			incoming := *old
			incoming.ID, incoming.Path, incoming.Branch = "new-"+tc.name, old.Path+"-replacement", "feature/new"
			incoming.WorkspaceRelativePath, incoming.Position = "new/placement", 9
			incoming.Status, incoming.DeletedAt = tc.incomingStatus, nil
			if tc.incomingDeleted {
				incoming.DeletedAt = &deleted
			}
			if tc.emptyPlacement {
				incoming.WorkspaceRelativePath, incoming.Position = "", 0
			}
			require.NoError(t, store.CreateWorktree(ctx, &incoming))
			loaded, err := store.GetWorktreeByID(ctx, incoming.ID)
			require.NoError(t, err)
			require.NotNil(t, loaded)
			wantPath, wantPosition := old.WorkspaceRelativePath, old.Position
			if tc.replace {
				wantPath, wantPosition = incoming.WorkspaceRelativePath, incoming.Position
			}
			require.Equal(t, wantPath, loaded.WorkspaceRelativePath)
			require.Equal(t, wantPosition, loaded.Position)
			require.Equal(t, incoming.Path, loaded.Path)
			require.Equal(t, incoming.Branch, loaded.Branch)
			require.Equal(t, incoming.Status, loaded.Status)
			require.Equal(t, incoming.DeletedAt, loaded.DeletedAt)
			var currentID string
			require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind(
				`SELECT id FROM task_environment_repos WHERE worktree_id = ?`), incoming.ID).Scan(&currentID))
			require.Equal(t, rowID, currentID)
		})
	}
}
