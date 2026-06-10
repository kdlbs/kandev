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

	// Sentry watch with the same id — must be counted under "sentry", not
	// bleed into linear/jira. 1 open + 1 completed → count should be 1.
	mkTask("t-sentry-open", "sentry_issue_watch_id", "watch-a", v1.TaskStateInProgress, false)
	mkTask("t-sentry-done", "sentry_issue_watch_id", "watch-a", v1.TaskStateCompleted, false)

	// User-created task with no watcher metadata.
	mkTask("t-user", "unrelated_key", "watch-a", v1.TaskStateTODO, false)

	got, err := repo.CountOpenWatcherCreatedTasks(ctx, "linear", "watch-a")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected 2 open linear tasks for watch-a, got %d", got)
	}

	gotB, err := repo.CountOpenWatcherCreatedTasks(ctx, "linear", "watch-b")
	if err != nil {
		t.Fatalf("count watch-b: %v", err)
	}
	if gotB != 1 {
		t.Fatalf("expected 1 open linear task for watch-b, got %d", gotB)
	}

	gotJira, err := repo.CountOpenWatcherCreatedTasks(ctx, "jira", "watch-a")
	if err != nil {
		t.Fatalf("count jira: %v", err)
	}
	if gotJira != 1 {
		t.Fatalf("expected 1 open jira task for watch-a, got %d", gotJira)
	}

	gotSentry, err := repo.CountOpenWatcherCreatedTasks(ctx, "sentry", "watch-a")
	if err != nil {
		t.Fatalf("count sentry: %v", err)
	}
	if gotSentry != 1 {
		t.Fatalf("expected 1 open sentry task for watch-a, got %d", gotSentry)
	}

	// Unknown integration falls through without erroring.
	gotUnknown, err := repo.CountOpenWatcherCreatedTasks(ctx, "github", "watch-a")
	if err != nil {
		t.Fatalf("count github: %v", err)
	}
	if gotUnknown != 0 {
		t.Fatalf("expected 0 open github tasks, got %d", gotUnknown)
	}
}
