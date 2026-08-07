package worktree

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// simulateSessionCascadeDelete removes one session's task_session_worktrees
// row without touching the shared worktree directory or other sessions'
// rows, mirroring what task_sessions' ON DELETE CASCADE does to
// task_session_worktrees when a session row is deleted. Tests call this
// before ReclaimSessionWorktree to reproduce the exact DB state the durable
// session-delete cleanup job observes: the deleted session's own claim is
// already gone by the time reclamation runs.
func simulateSessionCascadeDelete(t *testing.T, store *SQLiteStore, sessionID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(),
		`DELETE FROM task_session_worktrees WHERE session_id = ?`, sessionID,
	); err != nil {
		t.Fatalf("simulate session cascade delete for %s: %v", sessionID, err)
	}
}

func TestReclaimSessionWorktree_RemovesExclusivelyHeldWorktree(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("exclusively held worktree path should be removed, stat error = %v", err)
	}
}

func TestReclaimSessionWorktree_PreservesSharedWorktree(t *testing.T) {
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
	simulateSessionCascadeDelete(t, store, "session-owner")

	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-borrower", StatusActive)
}

func TestReclaimSessionWorktree_DoesNotDeleteBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree: %v", err)
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", wt.Branch)
	cmd.Dir = wt.RepositoryPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("branch %s should survive reclamation: %v: %s", wt.Branch, err, strings.TrimSpace(string(output)))
	}
}

func TestReclaimSessionWorktree_IdempotentWhenDirectoryAlreadyAbsent(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("pre-remove worktree directory: %v", err)
	}

	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree on already-absent directory: %v", err)
	}
}

func TestReclaimSessionWorktree_NilAndEmptyPathsAreNoop(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	ctx := context.Background()

	if err := mgr.ReclaimSessionWorktree(ctx, nil); err != nil {
		t.Fatalf("ReclaimSessionWorktree(nil): %v", err)
	}
	if err := mgr.ReclaimSessionWorktree(ctx, &Worktree{}); err != nil {
		t.Fatalf("ReclaimSessionWorktree(empty): %v", err)
	}
}
