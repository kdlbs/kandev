package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestUpdateExecutorRunningWorktreeBranchRejectsRotatedExecution(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-branch-cas")
	seedExecutorRunningCleanupSession(t, repo, "task-branch-cas", "session-branch-cas", models.TaskSessionStateRunning, "exec-successor")

	err := repo.UpdateExecutorRunningWorktreeBranch(ctx, "session-branch-cas", "exec-rotated", "feature/stale")
	if !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("rotated branch snapshot error = %v, want ErrExecutionRotated", err)
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-branch-cas")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if running.WorktreeBranch != "" {
		t.Fatalf("rotated write changed branch to %q", running.WorktreeBranch)
	}

	if err := repo.UpdateExecutorRunningWorktreeBranch(ctx, "session-branch-cas", "exec-successor", "feature/final-title"); err != nil {
		t.Fatalf("current execution branch snapshot: %v", err)
	}
	running, err = repo.GetExecutorRunningBySessionID(ctx, "session-branch-cas")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID after current write: %v", err)
	}
	if running.WorktreeBranch != "feature/final-title" {
		t.Fatalf("current execution branch = %q, want feature/final-title", running.WorktreeBranch)
	}
}
