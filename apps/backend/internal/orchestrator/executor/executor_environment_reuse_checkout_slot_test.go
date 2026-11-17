package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// These cases exercise checkout-branch slot adoption through the inventory
// admission entry points; the slot selector's own ambiguity and sibling rules
// are covered in executor_environment_reuse_inventory_test.go.

const checkoutSlotBranch = "feature/ver09-m7-slack-offic-6zc"

func checkoutSlotRow(id, branchSlug, worktreeID, worktreeBranch string) *models.TaskEnvironmentRepo {
	return &models.TaskEnvironmentRepo{
		ID: id, TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		BranchSlug: branchSlug, WorktreeID: worktreeID, WorktreeBranch: worktreeBranch,
		WorktreePath: "/tmp/tasks/task-1/repo", Status: taskEnvironmentRepoStatusActive,
	}
}

// A task repository whose checkout_branch was later set to the branch its
// preserved worktree already holds keeps resuming that exact checkout.
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.13
func TestPrepareResumeRepositorySettings_CheckoutBranchOfPreservedWorktreeAdoptsCanonicalSlot(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
		BaseBranch: "main", CheckoutBranch: checkoutSlotBranch,
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	row := checkoutSlotRow("env-repo-1", "main", "worktree-1", checkoutSlotBranch)
	env := &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, Repos: []*models.TaskEnvironmentRepo{row},
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{row}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1", BaseBranch: "main"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: session.ID, ExecutorType: string(models.ExecutorTypeWorktree)}

	if _, _, _, err := e.prepareResumeRepositorySettings(context.Background(), &v1.Task{ID: "task-1"}, session, req, ResumeOptions{}); err != nil {
		t.Fatalf("prepareResumeRepositorySettings() = %v, want nil for the preserved worktree already on the checkout branch", err)
	}
	if !req.WorkspaceReuseRequired {
		t.Fatal("req.WorkspaceReuseRequired = false, want true")
	}
	if req.BranchIdentitySlug != row.BranchSlug {
		t.Fatalf("req.BranchIdentitySlug = %q, want canonical slot %q", req.BranchIdentitySlug, row.BranchSlug)
	}
	if req.WorktreeID != row.WorktreeID {
		t.Fatalf("req.WorktreeID = %q, want preserved worktree %q", req.WorktreeID, row.WorktreeID)
	}
	if req.CheckoutBranch != checkoutSlotBranch {
		t.Fatalf("req.CheckoutBranch = %q, want %q", req.CheckoutBranch, checkoutSlotBranch)
	}
}

func checkoutSlotRequest() *LaunchAgentRequest {
	return &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceReuseRequired: true, UseWorktree: true,
		RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: checkoutSlotBranch,
	}
}

func validateCheckoutSlot(t *testing.T, req *LaunchAgentRequest, rows ...*models.TaskEnvironmentRepo) error {
	t.Helper()
	repo := newMockRepository()
	repo.taskEnvironmentRepos["env-1"] = rows
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	return e.validateReuseEnvironmentInventory(context.Background(), req, &models.TaskEnvironment{ID: "env-1", TaskID: "task-1"})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.13
func TestValidateReuseEnvironmentInventory_CheckoutBranchOnDifferentWorktreeBranchStillRefused(t *testing.T) {
	req := checkoutSlotRequest()
	err := validateCheckoutSlot(t, req, checkoutSlotRow("env-repo-1", "main", "worktree-1", "feature/other"))
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if req.BranchIdentitySlug != "" {
		t.Fatalf("req.BranchIdentitySlug = %q, want unchanged", req.BranchIdentitySlug)
	}
}

// A deleted row is the only holder of the checkout branch: there is no active
// slot to adopt, so admission still fails closed.
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.13
func TestValidateReuseEnvironmentInventory_CheckoutBranchOnDeletedRowStillRefused(t *testing.T) {
	req := checkoutSlotRequest()
	row := checkoutSlotRow("env-repo-1", "main", "worktree-1", checkoutSlotBranch)
	row.Status = taskEnvironmentRepoStatusDeleted
	if err := validateCheckoutSlot(t, req, row); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() = %v, want ErrWorkspaceReuseUnsafe for a deleted row", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.13
func TestValidateReuseEnvironmentInventory_ExactCheckoutSlotIsNotRepinned(t *testing.T) {
	req := checkoutSlotRequest()
	err := validateCheckoutSlot(t, req,
		checkoutSlotRow("env-repo-1", "main", "worktree-1", checkoutSlotBranch),
		checkoutSlotRow("env-repo-2", "feature-ver09-m7-slack-offic-6zc", "worktree-2", "feature/ver09-m7-slack-offic-6zc-2"),
	)
	if err != nil {
		t.Fatalf("validateReuseEnvironmentInventory() = %v, want nil for an exact slot match", err)
	}
	if req.BranchIdentitySlug != "" {
		t.Fatalf("req.BranchIdentitySlug = %q, want unchanged for an exact slot match", req.BranchIdentitySlug)
	}
}
