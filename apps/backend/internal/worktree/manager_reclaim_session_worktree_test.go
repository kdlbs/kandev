package worktree

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

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

// TestReclaimSessionWorktree_ReclaimsAfterBothSharingSessionsDeleted covers
// the spec's sequential sharing scenario directly (docs/specs/
// session-delete-resource-cleanup: "GIVEN sessions S1 and S2 both holding W
// at path P, WHEN S1 is deleted and then S2 is deleted, THEN after the
// second delete's job succeeds P no longer exists"). The two halves — "still
// shared, preserved" and "exclusively held, removed" — are each covered in
// isolation by sibling tests, but neither proves the second delete's live
// reference-count query actually observes the first delete's already-gone
// row rather than some stale view.
func TestReclaimSessionWorktree_ReclaimsAfterBothSharingSessionsDeleted(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	borrowed := *wt
	borrowed.TaskID = "task-borrower"
	borrowed.SessionID = "session-borrower"
	if err := store.CreateWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("create borrower worktree reference: %v", err)
	}

	// S1 deleted first: still preserved because S2's row is live.
	simulateSessionCascadeDelete(t, store, "session-owner")
	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree (S1): %v", err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("worktree should survive while S2 still holds it: %v", err)
	}

	// S2 deleted second: now the last live reference is gone too.
	simulateSessionCascadeDelete(t, store, "session-borrower")
	if err := mgr.ReclaimSessionWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("ReclaimSessionWorktree (S2): %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree should be reclaimed once both sharing sessions are deleted, stat error = %v", err)
	}
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

// TestReclaimSessionWorktree_SerializesWithConcurrentTargetPathCreate
// reproduces a data-loss race found while validating session.delete's
// worktree reclamation: a task's worktree directory name is derived only
// from task_dir + repo name (worktree.go's naming, independent of worktree
// ID), so a brand-new worktree created for the task right after a session
// delete (e.g. via EnsureSession's auto-continuation for a workflow step
// that allows auto-start) can land at the exact same path the just-deleted
// session's worktree occupied. gitAddWorktree(Locked) already serializes
// worktree creation against that target path via acquireWorktreeTargetPath;
// ReclaimSessionWorktree must join the same lock, or its
// `git worktree remove --force <path>` can run concurrently with — and
// destroy — a different, newly-created worktree that has since taken over
// the path.
func TestReclaimSessionWorktree_SerializesWithConcurrentTargetPathCreate(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	// Simulate a concurrent worktree creation already holding the target
	// path lock for this exact path (as gitAddWorktreeLocked does before
	// running `git worktree add`).
	release, err := acquireWorktreeTargetPath(ctx, wt.Path)
	if err != nil {
		t.Fatalf("acquireWorktreeTargetPath: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- mgr.ReclaimSessionWorktree(ctx, wt)
	}()

	select {
	case err := <-done:
		t.Fatalf("ReclaimSessionWorktree returned (err=%v) before the concurrent "+
			"target-path holder released the lock — it is not serializing with "+
			"worktree creation at the same path", err)
	case <-time.After(200 * time.Millisecond):
		// Expected: still blocked behind the held target-path lock.
	}

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ReclaimSessionWorktree after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReclaimSessionWorktree did not complete after the target-path lock was released")
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
