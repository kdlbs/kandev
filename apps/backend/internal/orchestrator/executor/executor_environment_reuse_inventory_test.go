package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// Regression coverage for the "failed prepare leaves a permanently bricked
// workspace" defect: when a task_environments row was published ready with
// zero task_environment_repos rows (the write-side bug, fixed separately),
// validateReuseEnvironmentInventory used to treat that as a mismatch and
// refuse every subsequent launch forever. Zero rows recorded means the
// canonical inventory was never captured at all, which is recoverable by
// letting the launch rebuild it — distinct from a non-empty but wrong
// inventory, which must still be refused.
func TestValidateReuseEnvironmentInventory_ZeroRowsIsRecoverable(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
	}
	env := &models.TaskEnvironment{ID: "env-1"}

	if err := e.validateReuseEnvironmentInventory(context.Background(), req, env); err != nil {
		t.Fatalf("validateReuseEnvironmentInventory() with zero recorded rows = %v, want nil (recoverable)", err)
	}
	if req.WorkspaceReuseRequired {
		t.Fatal("zero inventory kept WorkspaceReuseRequired=true, want fresh materialization")
	}
}

// The read-side fix must not weaken the guard's actual purpose: a non-empty
// but mismatched canonical inventory (wrong repository, wrong branch, or a
// row explicitly marked failed/deleted) is still an unsafe reuse and must be
// refused.
func TestValidateReuseEnvironmentInventory_MismatchedRowsStillRefused(t *testing.T) {
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
	}
	env := &models.TaskEnvironment{ID: "env-1"}
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-other", WorktreeID: "worktree-other"},
	}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() with mismatched rows = %v, want ErrWorkspaceReuseUnsafe", err)
	}
}
