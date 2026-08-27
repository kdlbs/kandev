package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	// The borrower's session references the owner's environment.
	if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
		t.Fatalf("link borrower session: %v", err)
	}

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
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
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func TestCleanupWorktrees_PreservesParentWorkspaceWhenInheritedSubtaskDeleted(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-parent", "session-parent", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-child", "session-child", models.TaskSessionStateCancelled)
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET parent_id = ? WHERE id = ?`, "task-parent", "task-child"); err != nil {
		t.Fatalf("link inherited subtask: %v", err)
	}

	parent := createReferenceCleanupWorktree(t, mgr, "task-parent", "session-parent")
	// The inherited subtask borrows the parent's environment. Its inventory
	// (queried through its own task row) therefore contains no worktrees.
	if err := store.linkSessionToEnvironment(ctx, "session-child", "env-parent"); err != nil {
		t.Fatalf("link inherited subtask session: %v", err)
	}
	childInventory, err := store.GetWorktreesByTaskID(ctx, "task-child")
	if err != nil {
		t.Fatalf("child inventory: %v", err)
	}
	if len(childInventory) != 0 {
		t.Fatalf("child inventory = %d worktrees, want 0 (worktrees belong to the parent env)", len(childInventory))
	}
	// The parent inventory must return exactly one row per environment
	// repository, not one per session sharing the environment.
	parentInventory, err := store.GetWorktreesByTaskID(ctx, "task-parent")
	if err != nil {
		t.Fatalf("parent inventory: %v", err)
	}
	if len(parentInventory) != 1 || parentInventory[0].ID != parent.ID {
		t.Fatalf("parent inventory = %+v, want exactly one deduplicated worktree", parentInventory)
	}

	if _, err := store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, "task-child"); err != nil {
		t.Fatalf("delete inherited subtask: %v", err)
	}

	if err := mgr.CleanupWorktrees(ctx, childInventory); err != nil {
		t.Fatalf("cleanup inherited subtask worktrees: %v", err)
	}

	if _, err := os.Stat(parent.Path); err != nil {
		t.Fatalf("parent workspace should survive inherited subtask deletion: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, parent.ID, StatusActive)
	var parentTasks int
	if err := store.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = ?`, "task-parent").Scan(&parentTasks); err != nil {
		t.Fatalf("count surviving parent task: %v", err)
	}
	if parentTasks != 1 {
		t.Fatalf("surviving parent tasks = %d, want 1", parentTasks)
	}
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
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestReleaseWorktreeReference_MissingAssociationIsIdempotent(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environment_repos WHERE worktree_id = ?`, wt.ID); err != nil {
		t.Fatalf("delete worktree record: %v", err)
	}

	if err := mgr.ReleaseWorktreeReference(ctx, wt); err != nil {
		t.Fatalf("release missing worktree record: %v", err)
	}
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
	envID := "env-" + strings.TrimPrefix(taskID, "task-")
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_environments (id, task_id, executor_type, status, workspace_path, created_at, updated_at)
		VALUES (?, ?, 'worktree', 'ready', '/tmp/' || ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, envID, taskID, taskID); err != nil {
		t.Fatalf("create environment %s: %v", envID, err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, sessionID, taskID, state, envID); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

// linkSessionToEnvironment re-points a session at another environment,
// modeling a borrower session that shares an owner's workspace.
func (s *SQLiteStore) linkSessionToEnvironment(ctx context.Context, sessionID, envID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`), envID, sessionID)
	return err
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

func assertWorktreeReferenceStatus(t *testing.T, store *SQLiteStore, worktreeID, want string) {
	t.Helper()
	var got string
	if err := store.ro.QueryRowContext(context.Background(), `
		SELECT status FROM task_environment_repos
		WHERE worktree_id = ?
	`, worktreeID).Scan(&got); err != nil {
		t.Fatalf("load worktree status: %v", err)
	}
	if got != want {
		t.Fatalf("worktree status = %q, want %q", got, want)
	}
}

// TestCreateWorktree_RejectedByTaskCleanupBarrier proves the worktree store
// refuses to admit a physical worktree once the owning task's lifecycle
// cleanup barrier is active, and that the manager compensates by removing the
// just-created directory.
func TestCreateWorktree_RejectedByTaskCleanupBarrier(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-barrier", "session-barrier", models.TaskSessionStateRunning)

	// First creation succeeds and leaves a live directory behind.
	first := createReferenceCleanupWorktree(t, mgr, "task-barrier", "session-barrier")
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("first worktree dir missing: %v", err)
	}

	// Archive preparation reserves its barrier before inventory capture.
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_resource_cleanup_jobs (
			id, operation_id, task_id, trigger, state, resource_snapshot, attempts, created_at, updated_at
		) VALUES ('barrier-job', 'op-barrier', 'task-barrier', 'archive', 'prepared', '{}', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("reserve barrier: %v", err)
	}

	// A second materialization must be rejected: the physical directory the
	// manager just created is compensated away.
	second, err := mgr.Create(ctx, CreateRequest{
		TaskID:         "task-barrier",
		SessionID:      "session-barrier",
		TaskTitle:      "Barrier",
		RepositoryID:   "repository",
		RepositoryPath: initGitRepoWithRemote(t),
		BaseBranch:     "main",
		TaskDirName:    "task-barrier",
		RepoName:       "repository",
		BranchSlug:     "second",
	})
	if err == nil {
		t.Fatalf("expected barrier rejection, got worktree %s", second.ID)
	}
	if !strings.Contains(err.Error(), "task cleanup in progress") {
		t.Fatalf("error = %v, want task cleanup barrier rejection", err)
	}
	if _, statErr := os.Stat(first.Path); statErr != nil {
		t.Fatalf("pre-existing worktree dir must survive compensation: %v", statErr)
	}
}

// TestRemoveByID_NeverDeletesBorrowedWorktree locks in the borrow protection:
// GetWorktreeByID returns an arbitrary session of the owning environment, and
// removeWorktree must not exclude it when counting references — otherwise a
// borrower's workspace could be physically deleted.
func TestRemoveByID_NeverDeletesBorrowedWorktree(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateRunning)
	if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
		t.Fatalf("link borrower session: %v", err)
	}

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	// Prove the loaded record can carry the borrower's session: with the
	// LEFT JOIN the session column is arbitrary, so force the exact shape
	// the blocker described.
	loaded, err := store.GetWorktreeByID(ctx, wt.ID)
	if err != nil {
		t.Fatalf("GetWorktreeByID: %v", err)
	}
	loaded.SessionID = "session-borrower"

	if err := mgr.RemoveByID(ctx, loaded.ID, true); err != nil {
		t.Fatalf("RemoveByID: %v", err)
	}

	// The borrowed worktree must survive both physically and as an active row.
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("borrowed worktree path must be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}
