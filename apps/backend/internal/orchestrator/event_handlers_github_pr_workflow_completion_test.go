package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-WORKFLOW-PR-EVENTS-001.4, AC-TASKS-WORKFLOW-PR-EVENTS-001.6
func TestRegisteredPRWorkflowCompletionReleasesWatcherCapacity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal bool
		complete bool
	}{
		{name: "live session completes", complete: true},
		{name: "terminal session completes", terminal: true, complete: true},
		{name: "unchecked final step stays open"},
		{name: "terminal session unchecked final stays open", terminal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPRWorkflowFixture(t, prMergedAcceptEvents())
			ctx := context.Background()
			sessionState := models.TaskSessionStateWaitingForInput
			if tc.terminal {
				sessionState = models.TaskSessionStateCompleted
			}
			setSessionState(t, ctx, f.repo, prWorkflowSessionID, sessionState)
			getter := f.svc.workflowStepGetter.(*mockStepGetter)
			getter.steps["step_done"].CompleteTaskOnEnter = tc.complete
			task, err := f.repo.GetTask(ctx, prWorkflowTaskID)
			if err != nil {
				t.Fatal(err)
			}
			task.Metadata = map[string]interface{}{"linear_issue_watch_id": "completion-watch"}
			if err := f.repo.UpdateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			registerPRWorkflowFixture(t, f, &prWorkflowFaultRepository{Repository: f.repo})
			f.svc.SetWatcherTaskCounter(f.repo)
			cap := 1
			if release, allowed := f.svc.acquireWatcherSlot(ctx, "linear", "linear_issue_watch_id", "completion-watch", &cap); allowed {
				release()
				t.Fatal("watcher admitted work before existing task completed")
			}
			publishPRWorkflowFixture(t, f)
			if !tc.terminal {
				f.waitOnEnter(t)
			}
			assertPRCompletionTaskAndCapacity(t, f, tc.complete)
			assertPRCompletionSessionAndHistory(t, f, sessionState)
		})
	}
}

func assertPRCompletionTaskAndCapacity(t *testing.T, f *prWorkflowFixture, complete bool) {
	t.Helper()
	ctx := context.Background()
	cap := 1
	task, err := f.repo.GetTask(ctx, prWorkflowTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.WorkflowStepID != "step_done" {
		t.Fatalf("task step = %s, want step_done", task.WorkflowStepID)
	}
	if (task.State == v1.TaskStateCompleted) != complete {
		t.Errorf("persisted task state = %s, completion enabled = %v", task.State, complete)
	}
	wantOpen := 1
	if complete {
		wantOpen = 0
	}
	count, err := f.repo.CountOpenWatcherCreatedTasks(ctx, "linear_issue_watch_id", "completion-watch")
	if err != nil || count != wantOpen {
		t.Errorf("real watcher count = %d, error = %v, want %d", count, err, wantOpen)
	}
	release, allowed := f.svc.acquireWatcherSlot(ctx, "linear", "linear_issue_watch_id", "completion-watch", &cap)
	if allowed {
		defer release()
	}
	if allowed != complete {
		t.Errorf("watcher admission = %v, want %v", allowed, complete)
	}
}

func assertPRCompletionSessionAndHistory(t *testing.T, f *prWorkflowFixture, sessionState models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	session, err := f.repo.GetTaskSession(ctx, prWorkflowSessionID)
	if err != nil || session.State != sessionState {
		t.Errorf("session state changed: session = %+v, error = %v", session, err)
	}
	calls := f.recorder.Calls()
	if len(calls) != 1 || calls[0].toStepID != "step_done" || calls[0].trigger != "on_pr_merged" {
		t.Errorf("PR completion history = %+v, want one on_pr_merged transition", calls)
	}
}
