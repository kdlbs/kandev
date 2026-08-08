package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// TestReclaimSessionWorktree_ReturnsRemovalFailure proves ReclaimSessionWorktree
// actually returns a directory-removal failure rather than only logging it,
// which is what the durable cleanup job's retry budget and the spec's
// "Reclamation failure SHALL be observable" (docs/specs/
// session-delete-resource-cleanup) depend on. test-supervisor (round 2)
// mutation-proved this gap: swapping the production `return fmt.Errorf(...)`
// for a Warn log + `return nil` left the whole package green.
func TestReclaimSessionWorktree_ReturnsRemovalFailure(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	// Make the worktree's parent directory read-only so both `git worktree
	// remove --force` and the os.RemoveAll/`rm -rf` fallback fail to unlink
	// the worktree directory from it.
	parent := filepath.Dir(wt.Path)
	probePath := filepath.Join(parent, ".permission-probe")
	if err := os.WriteFile(probePath, []byte("probe"), 0o644); err != nil {
		t.Fatalf("create permission probe file: %v", err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatalf("chmod parent read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	// Probe first (per apps/backend/CLAUDE.md "Filesystem permission tests"):
	// only assert permission-denied behavior once we've confirmed this
	// executor actually enforces the permission bit change — a root-like
	// Sprite executor may bypass it entirely.
	if err := os.Remove(probePath); err == nil {
		t.Skip("executor does not enforce directory write permission (likely running as root); " +
			"cannot exercise the removal-failure path")
	}

	err := mgr.ReclaimSessionWorktree(ctx, wt)
	if err == nil {
		t.Fatal("expected ReclaimSessionWorktree to return an error when directory removal fails, got nil")
	}
	if !strings.Contains(err.Error(), wt.Path) {
		t.Fatalf("error = %q, want it to name the unreclaimed worktree path %s", err.Error(), wt.Path)
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

// TestReclaimSessionWorktree_MatchesCreatorLockOrder proves ReclaimSessionWorktree
// cannot deadlock against a concurrent worktree creation for the same path.
// Every creation path (Create -> gitAddWorktree(Locked)/gitAddWorktreeForRecreate,
// manager_lifecycle.go) acquires the per-repo lock first and the target-path
// lock second, nested inside, holding the repo lock across its whole
// critical section. If ReclaimSessionWorktree acquired the two locks in the
// opposite order, a reclaim holding the target-path lock while waiting on
// the repo lock could deadlock against a concurrent creator holding the
// repo lock while waiting on the target-path lock — a classic AB-BA lock
// inversion, and a strictly worse failure mode (a permanent hang requiring
// a backend restart) than the directory-clobbering race the target-path
// lock itself was added to close.
func TestReclaimSessionWorktree_MatchesCreatorLockOrder(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	// Simulate a concurrent worktree creator that has already taken its
	// first lock — repoLock — matching every real creation path.
	repoLock := mgr.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()

	reclaimDone := make(chan error, 1)
	go func() {
		reclaimDone <- mgr.ReclaimSessionWorktree(ctx, wt)
	}()

	// Give reclaim time to reach (and, under a buggy target-path-then-repo
	// order, acquire) the target-path lock before the simulated creator's
	// nested acquisition attempt below — mirroring the interleaving that
	// produces the AB-BA deadlock.
	time.Sleep(50 * time.Millisecond)

	creatorTargetPathDone := make(chan error, 1)
	go func() {
		release, err := acquireWorktreeTargetPath(ctx, wt.Path)
		if err != nil {
			creatorTargetPathDone <- err
			return
		}
		release()
		creatorTargetPathDone <- nil
	}()

	select {
	case err := <-creatorTargetPathDone:
		if err != nil {
			t.Fatalf("simulated creator's nested target-path acquisition: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: the simulated creator (holding repoLock first, matching every real " +
			"creation path) could not acquire the target-path lock — ReclaimSessionWorktree must be " +
			"holding it while blocked waiting on repoLock, an AB-BA lock-order inversion")
	}

	// The creator's simulated critical section is done; release repoLock so
	// reclaim (blocked on it, under the correct order) can proceed.
	repoLock.Unlock()
	mgr.releaseRepoLock(wt.RepositoryPath)

	select {
	case err := <-reclaimDone:
		if err != nil {
			t.Fatalf("ReclaimSessionWorktree: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReclaimSessionWorktree did not complete after the simulated creator released repoLock")
	}
}

// TestReclaimSessionWorktree_PreservesReplacementWorktreeAtSamePath covers
// the second concurrency defect found in review-round 1
// (docs/specs/session-delete-resource-cleanup, "removeWorktreeDir operates
// by path, not worktree ID"): worktree directory paths are derived only
// from task dir + repo name (prepareTaskWorktreePath), independent of
// worktree_id. If a replacement worktree — a different ID, for the same
// task — legitimately occupies wt.Path by the time this call acquires its
// locks (e.g. EnsureSession's auto-continuation launching a fresh session
// for the task right after this one was deleted), removeWorktreeDir must
// not blindly force-remove whatever is currently registered at that path:
// that would destroy the replacement's live data instead of leaving it
// alone. The target-path lock alone does not prevent this — it only
// prevents the two operations from running literally concurrently, not the
// sequential case where the replacement's creation already won.
func TestReclaimSessionWorktree_PreservesReplacementWorktreeAtSamePath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	simulateSessionCascadeDelete(t, store, "session-owner")

	// Simulate the replacement: a different worktree_id, for the same task,
	// already persisted at the exact same path. Insert the session row
	// directly rather than via seedReferenceCleanupSession, which would also
	// try (and fail) to re-insert the already-seeded task-owner task row.
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, 'task-owner', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "session-replacement", models.TaskSessionStateRunning); err != nil {
		t.Fatalf("create replacement session: %v", err)
	}
	replacement := *wt
	replacement.ID = "replacement-worktree-id"
	replacement.SessionID = "session-replacement"
	if err := store.CreateWorktree(ctx, &replacement); err != nil {
		t.Fatalf("create replacement worktree at the same path: %v", err)
	}

	if err := mgr.ReclaimSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("ReclaimSessionWorktree: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("replacement worktree's directory should survive reclamation of the worktree it replaced, stat error = %v", err)
	}
	assertWorktreeReferenceStatus(t, store, replacement.ID, "session-replacement", StatusActive)
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
