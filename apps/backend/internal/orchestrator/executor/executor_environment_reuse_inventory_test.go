package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1
func TestValidateReuseEnvironmentInventory_ZeroRowsFailsClosed(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
	}
	env := &models.TaskEnvironment{ID: "env-1"}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() with zero recorded rows = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if !req.WorkspaceReuseRequired {
		t.Fatal("zero inventory disabled WorkspaceReuseRequired and authorized materialization")
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

// TestValidateReuseEnvironmentInventory_StagingPy3MismatchReproducesFailClosedError
// reproduces the exact fail-closed error observed in the field for task
// 96cfb14c-62f4-4048-bc03-813f1f123875 / session be3a413d-0891-4982-a563-b631028c36c6
// / branch "staging-py3": the canonical inventory has zero rows for the
// environment, so a Human-QA auto-start (or a fresh provider-only session
// retry) on that task must still be refused rather than silently falling
// through to a fresh checkout.
func TestValidateReuseEnvironmentInventory_StagingPy3MismatchReproducesFailClosedError(t *testing.T) {
	const (
		taskID    = "96cfb14c-62f4-4048-bc03-813f1f123875"
		sessionID = "be3a413d-0891-4982-a563-b631028c36c6"
		branch    = "staging-py3"
	)
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: taskID, RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 taskID,
		SessionID:              sessionID,
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
		Branch:                 branch,
	}
	env := &models.TaskEnvironment{ID: "env-staging-py3", TaskID: taskID}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() for staging-py3 reproduction = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if !strings.Contains(err.Error(), "canonical workspace repository inventory has no matching entry") {
		t.Fatalf("validateReuseEnvironmentInventory() error = %q, want the exact logged fail-closed message", err.Error())
	}
	if !req.WorkspaceReuseRequired {
		t.Fatal("staging-py3 mismatch disabled WorkspaceReuseRequired and authorized materialization")
	}
}

// TestValidateReuseEnvironmentInventory_DevMismatchReproducesFailClosedError
// reproduces the second logged occurrence of the same platform defect on an
// unrelated task, 24cab57c-8e2c-44cc-8214-c0600d559391 / branch "dev": a
// present-but-mismatched canonical row (wrong repository) must be refused
// exactly like the zero-row case, confirming the defect is a reusable
// inventory-identity gap rather than source-task-specific behavior.
func TestValidateReuseEnvironmentInventory_DevMismatchReproducesFailClosedError(t *testing.T) {
	const (
		taskID = "24cab57c-8e2c-44cc-8214-c0600d559391"
		branch = "dev"
	)
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 taskID,
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
		Branch:                 branch,
	}
	env := &models.TaskEnvironment{ID: "env-dev", TaskID: taskID}
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-other", WorktreeID: "worktree-other", BranchSlug: branch},
	}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() for dev reproduction = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if !strings.Contains(err.Error(), "canonical workspace repository inventory has no matching entry") {
		t.Fatalf("validateReuseEnvironmentInventory() error = %q, want the exact logged fail-closed message", err.Error())
	}
}
