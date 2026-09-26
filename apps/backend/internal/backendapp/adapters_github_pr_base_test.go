package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSelectTaskRepositoryForPRDisambiguatesByHeadBranch(t *testing.T) {
	taskRepos := []*models.TaskRepository{
		{ID: "task-repo-parent", RepositoryID: "repo-1", CheckoutBranch: "feature/parent"},
		{ID: "task-repo-child", RepositoryID: "repo-1", CheckoutBranch: "feature/child"},
		{ID: "task-repo-other", RepositoryID: "repo-2", CheckoutBranch: "feature/child"},
	}

	got, err := selectTaskRepositoryForPR(taskRepos, "repo-1", "feature/child")
	if err != nil {
		t.Fatalf("selectTaskRepositoryForPR() error: %v", err)
	}
	if got.ID != "task-repo-child" {
		t.Fatalf("task repository ID = %q, want task-repo-child", got.ID)
	}
}
