package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func TestCleanupWorktrees_PreservesSharedActiveReference(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateRunning)

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	borrowed := *wt
	borrowed.TaskID = "task-borrower"
	borrowed.SessionID = "session-borrower"
	if err := store.CreateWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("create borrower worktree reference: %v", err)
	}
	count, err := store.CountActiveWorktreeReferences(ctx, wt.ID, []string{"session-owner"})
	if err != nil {
		t.Fatalf("count active worktree references: %v", err)
	}
	if count != 1 {
		t.Fatalf("active foreign worktree references = %d, want 1", count)
	}

	if err := mgr.CleanupWorktrees(ctx, []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-owner", StatusDeleted)
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-borrower", StatusActive)
}

func TestCleanupWorktrees_RemovesLastActiveReference(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("last-reference worktree path should be removed, stat error = %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-owner", StatusDeleted)
}

func TestReleaseWorktreeReference_MissingAssociationIsIdempotent(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateRunning)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	borrowed := *wt
	borrowed.TaskID = "task-borrower"
	borrowed.SessionID = "session-borrower"
	if err := store.CreateWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("create borrower worktree reference: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_session_worktrees WHERE session_id = ?`, wt.SessionID); err != nil {
		t.Fatalf("delete owner worktree reference: %v", err)
	}

	if err := mgr.ReleaseWorktreeReference(ctx, wt); err != nil {
		t.Fatalf("release missing owner reference: %v", err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, borrowed.SessionID, StatusActive)
}

func newReferenceCleanupTestManager(t *testing.T) (*Manager, *SQLiteStore) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "cleanup.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	store, err := NewSQLiteStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("create worktree store: %v", err)
	}
	if err := taskRepo.CreateWorkspace(context.Background(), &models.Workspace{ID: "workspace", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr, store
}

func seedReferenceCleanupSession(
	t *testing.T,
	store *SQLiteStore,
	taskID string,
	sessionID string,
	state models.TaskSessionState,
) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, priority, created_at, updated_at)
		VALUES (?, 'workspace', ?, 'medium', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, taskID, taskID); err != nil {
		t.Fatalf("create task %s: %v", taskID, err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, sessionID, taskID, state); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

func createReferenceCleanupWorktree(t *testing.T, mgr *Manager, taskID, sessionID string) *Worktree {
	t.Helper()
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         taskID,
		SessionID:      sessionID,
		TaskTitle:      "Shared cleanup",
		RepositoryID:   "repository",
		RepositoryPath: initGitRepoWithRemote(t),
		BaseBranch:     "main",
		TaskDirName:    taskID,
		RepoName:       "repository",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	return wt
}

func assertWorktreeReferenceStatus(t *testing.T, store *SQLiteStore, worktreeID, sessionID, want string) {
	t.Helper()
	var got string
	if err := store.ro.QueryRowContext(context.Background(), `
		SELECT status FROM task_session_worktrees
		WHERE worktree_id = ? AND session_id = ?
	`, worktreeID, sessionID).Scan(&got); err != nil {
		t.Fatalf("load worktree reference status: %v", err)
	}
	if got != want {
		t.Fatalf("worktree reference status = %q, want %q", got, want)
	}
}
