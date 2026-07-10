package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestResetTaskSessionBasesForRepository covers the side effect of the
// changes-panel "Compare against" picker: when the user changes the recorded
// base_branch, every session of the same (task, repository) pair gets its
// base_branch rewritten and its base_commit_sha cleared so the git log /
// cumulative diff endpoints stop filtering against the SHA captured at the
// OLD base.
func TestResetTaskSessionBasesForRepository(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "T",
	})
	// Two sessions on the same repo + one on a different repo to confirm
	// the WHERE clause is scoped tightly.
	sessions := []*models.TaskSession{
		{ID: "sess-a", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main", BaseCommitSHA: "old-sha-a"},
		{ID: "sess-b", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main", BaseCommitSHA: "old-sha-b"},
		{ID: "sess-other", TaskID: "task-1", RepositoryID: "repo-2", BaseBranch: "main", BaseCommitSHA: "old-sha-other"},
	}
	for _, s := range sessions {
		if err := repo.CreateTaskSession(ctx, s); err != nil {
			t.Fatalf("CreateTaskSession %s: %v", s.ID, err)
		}
	}

	rows, err := repo.ResetTaskSessionBasesForRepository(ctx, "task-1", "repo-1", "staging")
	if err != nil {
		t.Fatalf("ResetTaskSessionBasesForRepository: %v", err)
	}
	if rows != 2 {
		t.Errorf("rows affected = %d, want 2", rows)
	}

	for _, id := range []string{"sess-a", "sess-b"} {
		reread, err := repo.GetTaskSession(ctx, id)
		if err != nil {
			t.Fatalf("GetTaskSession %s: %v", id, err)
		}
		if reread.BaseBranch != "staging" {
			t.Errorf("session %s base_branch = %q, want staging", id, reread.BaseBranch)
		}
		if reread.BaseCommitSHA != "" {
			t.Errorf("session %s base_commit_sha = %q, want empty (cleared)", id, reread.BaseCommitSHA)
		}
	}

	// Cross-repo session must NOT have been touched.
	other, err := repo.GetTaskSession(ctx, "sess-other")
	if err != nil {
		t.Fatalf("GetTaskSession sess-other: %v", err)
	}
	if other.BaseBranch != "main" || other.BaseCommitSHA != "old-sha-other" {
		t.Errorf("cross-repo session was modified: base_branch=%q base_commit_sha=%q", other.BaseBranch, other.BaseCommitSHA)
	}
}

// TestResetTaskSessionBasesForRepository_RequiresIDs guards the input
// validation: empty task_id / repository_id are not silently turned into
// "update every row" UPDATEs.
func TestResetTaskSessionBasesForRepository_RequiresIDs(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if _, err := repo.ResetTaskSessionBasesForRepository(ctx, "", "repo-1", "main"); err == nil {
		t.Error("empty task_id should error")
	}
	if _, err := repo.ResetTaskSessionBasesForRepository(ctx, "task-1", "", "main"); err == nil {
		t.Error("empty repository_id should error")
	}
}
