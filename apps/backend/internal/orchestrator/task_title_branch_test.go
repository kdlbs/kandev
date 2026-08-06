package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRenderTitleBranchNameUsesFinalTitleAndRepositoryTemplate(t *testing.T) {
	got, err := renderTitleBranchName(
		"Improve login flow",
		&models.Task{ID: "task-123", Identifier: "KAN-42"},
		&models.Repository{WorktreeBranchTemplate: "feature/{ticket}-{title}-{suffix}"},
		"abc",
	)
	if err != nil {
		t.Fatalf("renderTitleBranchName returned error: %v", err)
	}
	if got != "feature/kan-42-improve-login-flow-abc" {
		t.Fatalf("renderTitleBranchName = %q, want final-title branch", got)
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
