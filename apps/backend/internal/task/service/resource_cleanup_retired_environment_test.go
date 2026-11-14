package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestCleanupDestructiveTaskResourcesRemovesRetiredEnvironmentCheckout(t *testing.T) {
	svc, _, _ := createTestService(t)
	cleanup := &recordingWorktreeCleanup{}
	svc.SetWorktreeCleanup(cleanup)

	const taskID = "task-retired-delete"
	// The task row and both environment rows have already cascaded. The
	// snapshot captured the retired environment's checkout and the terminal
	// session that still references it.
	retiredSession := &models.TaskSession{ID: "session-retired", TaskID: taskID, TaskEnvironmentID: "env-retired"}
	unprovenSession := &models.TaskSession{ID: "session-unproven", TaskID: taskID, TaskEnvironmentID: "env-elsewhere"}
	retired := &worktree.Worktree{
		ID: "worktree-retired", TaskID: taskID, SessionID: retiredSession.ID, TaskEnvironmentID: "env-retired",
	}
	unproven := &worktree.Worktree{
		ID: "worktree-unproven", TaskID: taskID, SessionID: unprovenSession.ID, TaskEnvironmentID: "env-captured",
	}

	errs := svc.cleanupDestructiveTaskResources(
		context.Background(), taskID, []*models.TaskSession{retiredSession, unprovenSession},
		[]*worktree.Worktree{retired, unproven}, taskEnvironmentCleanup{}, nil,
	)

	if len(errs) != 0 {
		t.Fatalf("cleanup errors = %v, want none", errs)
	}
	// The unproven checkout's session is bound elsewhere, so only a live
	// lookup could prove its ownership, and that lookup fails after deletion.
	if got := cleanup.cleanedIDs(); len(got) != 1 || got[0] != retired.ID {
		t.Fatalf("cleaned worktrees = %v, want [%s]", got, retired.ID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestCleanupDestructiveTaskResourcesKeepsSharedActiveEnvironmentSkip(t *testing.T) {
	svc, _, _ := createTestService(t)
	cleanup := &recordingWorktreeCleanup{}
	svc.SetWorktreeCleanup(cleanup)

	const taskID = "task-active-owner"
	active := &models.TaskEnvironment{ID: "env-active", TaskID: taskID}
	session := &models.TaskSession{ID: "session-active", TaskID: taskID, TaskEnvironmentID: active.ID}
	wt := &worktree.Worktree{ID: "worktree-active", TaskID: taskID, SessionID: session.ID, TaskEnvironmentID: active.ID}

	kept := svc.filterOwnedWorktreesForTaskCleanup(
		context.Background(), taskID, []*models.TaskSession{session}, []*worktree.Worktree{wt}, active, true,
	)
	if len(kept) != 0 {
		t.Fatalf("active environment worktrees = %d, want the shared-environment skip preserved", len(kept))
	}
}
