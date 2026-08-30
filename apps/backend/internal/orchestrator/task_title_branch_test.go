package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRenderTitleBranchNameUsesFinalTitleAndRepositoryTemplate(t *testing.T) {
	got, err := renderTitleBranchNameForTaskRepository(
		"Improve login flow",
		&models.Task{ID: "task-123", Identifier: "KAN-42"},
		&models.Repository{WorktreeBranchTemplate: "feature/{ticket}-{title}-{suffix}"},
		nil,
		"abc",
	)
	if err != nil {
		t.Fatalf("renderTitleBranchNameForTaskRepository returned error: %v", err)
	}
	if got != "feature/kan-42-improve-login-flow-abc" {
		t.Fatalf("renderTitleBranchNameForTaskRepository = %q, want final-title branch", got)
	}
}

func TestRenderTitleBranchNameUsesTaskPolicySnapshot(t *testing.T) {
	got, err := renderTitleBranchNameForTaskRepository(
		"Fix login flow",
		&models.Task{ID: "task-123", Identifier: "KAN-42"},
		&models.Repository{WorktreeBranchTemplate: "feature/{title}"},
		&models.TaskRepository{BranchPolicyBranchTemplate: "bugfix/{ticket}-{title}-{suffix}"},
		"abc",
	)
	if err != nil {
		t.Fatalf("renderTitleBranchNameForTaskRepository returned error: %v", err)
	}
	if got != "bugfix/kan-42-fix-login-flow-abc" {
		t.Fatalf("renderTitleBranchNameForTaskRepository = %q, want policy snapshot branch", got)
	}
}

func TestTitleBranchRenameStatusDistinguishesPartialFailure(t *testing.T) {
	result := aggregateTitleBranchRenameStatus(
		[]TitleBranchRename{{RepositoryID: "repo-a"}},
		[]TitleBranchPreservation{{RepositoryID: "repo-b", Reason: "remote_checkout"}},
		[]TitleBranchFailure{{RepositoryID: "repo-c", Message: "collision"}},
	)
	if result != TitleBranchStatusPartial {
		t.Fatalf("aggregateTitleBranchRenameStatus = %q, want %q", result, TitleBranchStatusPartial)
	}
}
