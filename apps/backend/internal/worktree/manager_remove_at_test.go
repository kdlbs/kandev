package worktree

import (
	"context"
	"os"
	"testing"
)

// TestRemoveAt_PathFallbackRemovesDirectoryAfterSessionCascade is the
// worktree-package-level regression test for the multi-repo disk leak: once
// the owning session is deleted, task_session_worktrees — the only table
// that stores a worktree's on-disk path for an ID-only lookup — cascades
// away, so GetByID can no longer resolve a path for that worktree_id.
// RemoveAt must still remove the directory using the caller-supplied
// path/repository handles.
func TestRemoveAt_PathFallbackRemovesDirectoryAfterSessionCascade(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := mgr.GetByID(ctx, wt.ID); err == nil {
		t.Fatal("expected GetByID to fail once the session-scoped row has cascaded away")
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be removed, stat error = %v", err)
	}
}

// TestRemoveAt_PreservesSharedActiveReference confirms the safety ordering:
// when the worktree_id still resolves through GetByID (the owner's row is
// still present, and another session's row references the same physical
// worktree), RemoveAt must delegate to the same tracked-row path RemoveByID
// uses — never take the path-only fallback — so the shared/borrowed-worktree
// reference count guard is never bypassed.
func TestRemoveAt_PreservesSharedActiveReference(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "running")
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", "running")

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	borrowed := *wt
	borrowed.TaskID = "task-borrower"
	borrowed.SessionID = "session-borrower"
	if err := store.CreateWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("create borrower worktree reference: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-owner", StatusDeleted)
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-borrower", StatusActive)
}
