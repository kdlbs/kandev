package worktree

import (
	"context"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.2
func TestCountActiveWorktreeReferences_ArchivedBorrowerMatrix(t *testing.T) {
	cases := []struct {
		name     string
		state    models.TaskSessionState
		archived bool
		runtime  bool
		want     int
	}{
		{"ArchivedCompletedReleased", models.TaskSessionStateCompleted, true, false, 0},
		{"ArchivedFailedReleased", models.TaskSessionStateFailed, true, false, 0},
		{"ArchivedCancelledReleased", models.TaskSessionStateCancelled, true, false, 0},
		{"ArchivedCancelledWithRuntimeHeld", models.TaskSessionStateCancelled, true, true, 1},
		{"ArchivedCreatedHeld", models.TaskSessionStateCreated, true, false, 1},
		{"ArchivedStartingHeld", models.TaskSessionStateStarting, true, false, 1},
		{"ArchivedRunningHeld", models.TaskSessionStateRunning, true, false, 1},
		{"ArchivedWaitingForInputHeld", models.TaskSessionStateWaitingForInput, true, false, 1},
		{"ArchivedIdleHeld", models.TaskSessionStateIdle, true, false, 1},
		{"LiveCompletedHeld", models.TaskSessionStateCompleted, false, false, 1},
		{"LiveFailedHeld", models.TaskSessionStateFailed, false, false, 1},
		{"LiveCancelledHeld", models.TaskSessionStateCancelled, false, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			ctx := context.Background()
			seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
			seedArchivedBorrowerWorktree(t, store, "env-owner", "wt-matrix")
			seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", tc.state)
			if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
				t.Fatalf("link borrower session: %v", err)
			}
			if tc.archived {
				archiveBorrowerTask(t, store, "task-borrower")
			}
			if tc.runtime {
				seedBorrowerRuntime(t, store, "session-borrower", "task-borrower")
			}

			count, err := store.CountActiveWorktreeReferences(ctx, "wt-matrix", nil)
			if err != nil {
				t.Fatalf("count active worktree references: %v", err)
			}
			if count != tc.want {
				t.Fatalf("active foreign worktree references = %d, want %d", count, tc.want)
			}
		})
	}
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.2
func TestCountActiveWorktreeReferences_UnjoinableBorrowerTaskHolds(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
	seedArchivedBorrowerWorktree(t, store, "env-owner", "wt-unjoinable")

	// The tasks join cannot determine archive state for a session whose task
	// row is missing, so the hold must fail closed. Foreign keys are disabled
	// only around this insert to model a legacy row that outlived its task.
	if _, err := store.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	t.Cleanup(func() {
		if _, err := store.db.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`); err != nil {
			t.Errorf("restore foreign keys: %v", err)
		}
	})
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		VALUES ('session-missing-task', 'task-missing', ?, 'env-owner', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, string(models.TaskSessionStateCancelled)); err != nil {
		t.Fatalf("seed unjoinable borrower session: %v", err)
	}

	count, err := store.CountActiveWorktreeReferences(ctx, "wt-unjoinable", nil)
	if err != nil {
		t.Fatalf("count active worktree references: %v", err)
	}
	if count != 1 {
		t.Fatalf("unjoinable borrower references = %d, want 1", count)
	}
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.2
func TestCleanupWorktrees_RemovesWorktreeHeldOnlyByArchivedBorrower(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateCancelled)
	if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
		t.Fatalf("link borrower session: %v", err)
	}
	archiveBorrowerTask(t, store, "task-borrower")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	receipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("CleanupWorktreesWithReceipt: %v", err)
	}
	if receipt.Attempted != 1 {
		t.Fatalf("cleanup receipt = %+v, want one attempt", receipt)
	}
	if len(receipt.RetainedReasons) != 0 {
		t.Fatalf("cleanup receipt retained reasons = %+v, want none", receipt.RetainedReasons)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("released borrower hold should remove the worktree, stat error = %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
	assertSessionSurvives(t, store, "session-borrower")
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.1
func TestCleanupWorktrees_RetainsWorktreeWithMixedBorrowers(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", models.TaskSessionStateRunning)
	seedReferenceCleanupSession(t, store, "task-released", "session-released", models.TaskSessionStateCancelled)
	seedReferenceCleanupSession(t, store, "task-live", "session-live", models.TaskSessionStateCompleted)
	for _, sessionID := range []string{"session-released", "session-live"} {
		if err := store.linkSessionToEnvironment(ctx, sessionID, "env-owner"); err != nil {
			t.Fatalf("link borrower %s: %v", sessionID, err)
		}
	}
	archiveBorrowerTask(t, store, "task-released")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	receipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("CleanupWorktreesWithReceipt: %v", err)
	}
	if receipt.Attempted != 1 || receipt.RetainedReasons[RetainedActiveReference] != 1 {
		t.Fatalf("mixed-borrower receipt = %+v, want one active-reference retention", receipt)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("live borrower must keep the shared worktree: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func seedArchivedBorrowerWorktree(t *testing.T, store *SQLiteStore, envID, worktreeID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			status, created_at, updated_at
		) VALUES (?, ?, 'repo-archived-borrower', 'main', ?, '/tmp/' || ?, 'feature/borrower', 0,
			?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "env-repo-"+worktreeID, envID, worktreeID, worktreeID, StatusActive); err != nil {
		t.Fatalf("seed worktree row %s: %v", worktreeID, err)
	}
}

func archiveBorrowerTask(t *testing.T, store *SQLiteStore, taskID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
		UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?
	`, taskID); err != nil {
		t.Fatalf("archive task %s: %v", taskID, err)
	}
}

func seedBorrowerRuntime(t *testing.T, store *SQLiteStore, sessionID, taskID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
		INSERT INTO executors_running (id, session_id, task_id, executor_id, status, created_at, updated_at)
		VALUES (?, ?, ?, 'executor-archived-borrower', 'running', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "runtime-"+sessionID, sessionID, taskID); err != nil {
		t.Fatalf("seed borrower runtime %s: %v", sessionID, err)
	}
}

func assertSessionSurvives(t *testing.T, store *SQLiteStore, sessionID string) {
	t.Helper()
	var count int
	if err := store.ro.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM task_sessions WHERE id = ?
	`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count surviving session %s: %v", sessionID, err)
	}
	if count != 1 {
		t.Fatalf("surviving sessions for %s = %d, want 1", sessionID, count)
	}
}
