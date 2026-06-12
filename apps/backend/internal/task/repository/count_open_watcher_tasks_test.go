package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSQLiteRepository_CountOpenWatcherCreatedTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})

	// Linear watch A has 2 open tasks + 1 completed + 1 archived → count should be 2.
	mkTask := func(id, watchKey, watchID string, state v1.TaskState, archived bool) {
		t.Helper()
		task := &models.Task{
			ID:             id,
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf-1",
			WorkflowStepID: "step-1",
			Title:          id,
			State:          state,
			Metadata: map[string]interface{}{
				watchKey: watchID,
			},
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", id, err)
		}
		if archived {
			if err := repo.ArchiveTask(ctx, id); err != nil {
				t.Fatalf("archive task %s: %v", id, err)
			}
		}
	}

	mkTask("t-open-1", "linear_issue_watch_id", "watch-a", v1.TaskStateTODO, false)
	mkTask("t-open-2", "linear_issue_watch_id", "watch-a", v1.TaskStateInProgress, false)
	mkTask("t-completed", "linear_issue_watch_id", "watch-a", v1.TaskStateCompleted, false)
	mkTask("t-archived", "linear_issue_watch_id", "watch-a", v1.TaskStateTODO, true)

	// Different watch — must not count toward watch-a.
	mkTask("t-other-linear", "linear_issue_watch_id", "watch-b", v1.TaskStateTODO, false)

	// Different integration — Jira watch with the same id must not bleed in.
	mkTask("t-jira", "jira_issue_watch_id", "watch-a", v1.TaskStateTODO, false)

	// Sentry watch with the same id — must be counted under its own key, not
	// bleed into linear/jira. 1 open + 1 completed → count should be 1.
	mkTask("t-sentry-open", "sentry_issue_watch_id", "watch-a", v1.TaskStateInProgress, false)
	mkTask("t-sentry-done", "sentry_issue_watch_id", "watch-a", v1.TaskStateCompleted, false)

	// A hypothetical future integration whose key the repository was never
	// taught about. The count must still work — proving the repository is
	// agnostic of which integrations exist and keys purely on the metadata key.
	mkTask("t-future", "gitlab_issue_watch_id", "watch-a", v1.TaskStateTODO, false)

	// User-created task with no watcher metadata.
	mkTask("t-user", "unrelated_key", "watch-a", v1.TaskStateTODO, false)

	// The caller supplies the integration's task-metadata key directly; the
	// repository keys the COUNT on it with no knowledge of integration names.
	cases := []struct {
		name      string
		metaKey   string
		watchID   string
		wantCount int
	}{
		{"linear watch-a (2 open, excl. completed+archived)", "linear_issue_watch_id", "watch-a", 2},
		{"linear watch-b", "linear_issue_watch_id", "watch-b", 1},
		{"jira watch-a (no bleed from linear/sentry)", "jira_issue_watch_id", "watch-a", 1},
		{"sentry watch-a (excl. completed)", "sentry_issue_watch_id", "watch-a", 1},
		{"unregistered future integration key still counts", "gitlab_issue_watch_id", "watch-a", 1},
		{"key with no matching tasks", "never_used_key", "watch-a", 0},
	}
	for _, tc := range cases {
		got, err := repo.CountOpenWatcherCreatedTasks(ctx, tc.metaKey, tc.watchID)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.wantCount {
			t.Fatalf("%s: expected %d, got %d", tc.name, tc.wantCount, got)
		}
	}

	// Empty watch id is a no-op (0, nil), not an error.
	if n, err := repo.CountOpenWatcherCreatedTasks(ctx, "linear_issue_watch_id", ""); err != nil || n != 0 {
		t.Fatalf("empty watchID: expected (0, nil), got (%d, %v)", n, err)
	}

	// A malformed metadata key (not a bare identifier) is rejected rather than
	// spliced into the json_extract path — guards against SQL injection and
	// surfaces wiring bugs as a loud error.
	for _, bad := range []string{"", "bad-key", "a.b", "key'); DROP TABLE tasks;--"} {
		if _, err := repo.CountOpenWatcherCreatedTasks(ctx, bad, "watch-a"); err == nil {
			t.Fatalf("expected error for malformed metadata key %q, got nil", bad)
		}
	}
}
