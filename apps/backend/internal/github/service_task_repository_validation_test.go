package github

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestAssociatePRWithTask_UsesCanonicalTaskRepositoryID(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-A"},
		repos: []*taskmodels.TaskRepository{{
			ID:           "task-repo-row",
			RepositoryID: "repo-canonical",
		}},
	})
	ctx := context.Background()
	pr := &PR{
		Number:    42,
		RepoOwner: "org",
		RepoName:  "repo",
		HTMLURL:   "https://github.com/org/repo/pull/42",
	}

	if _, err := svc.AssociatePRWithTask(ctx, "task-A", "task-repo-row", pr); !errors.Is(err, ErrTaskPRRepositoryMismatch) {
		t.Fatalf("associate with task_repositories row ID error = %v, want ErrTaskPRRepositoryMismatch", err)
	}
	rows, err := store.ListTaskPRsByTask(ctx, "task-A")
	if err != nil {
		t.Fatalf("list PR associations after rejected ID: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d PR associations after rejected ID, want none", len(rows))
	}

	if _, err := svc.AssociatePRWithTask(ctx, "task-A", "repo-canonical", pr); err != nil {
		t.Fatalf("associate with canonical repository ID: %v", err)
	}
	rows, err = store.ListTaskPRsByTask(ctx, "task-A")
	if err != nil {
		t.Fatalf("list PR associations after canonical ID: %v", err)
	}
	if len(rows) != 1 || rows[0].RepositoryID != "repo-canonical" {
		t.Fatalf("got PR associations %+v, want one row for repo-canonical", rows)
	}
}
