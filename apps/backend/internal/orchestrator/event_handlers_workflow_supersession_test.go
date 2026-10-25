package orchestrator

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleTaskMoved_SupersededMoveSkipsStaleDestinationLifecycle pins
// AC-005.3 (pin): handleTaskMoved's supersession guard
// (event_handlers_workflow.go, "} else if task.WorkflowStepID !=
// data.ToStepID {"). task.moved events are published after each write and
// can be delivered out of order or delayed; if two moves race (task A:
// step1 -> step2, then step2 -> step3) and the first event is processed
// after the second move already committed, the handler must recognize the
// task has since moved on and skip step2's on_enter/auto-start lifecycle
// entirely — otherwise a superseded destination's auto_start_agent launches
// an agent on a step the task has already left, precisely the defect class
// this whole card exists to prevent. Replacing the guard's condition with
// "false" (never firing) passes the rest of this package's suite untouched
// — this is the only test that pins it.
func TestHandleTaskMoved_SupersededMoveSkipsStaleDestinationLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		// The task's persisted step is step3 — a later move has already landed
		// it there — while the event handled below still names step2 as its own
		// destination, simulating delayed/out-of-order delivery of the earlier
		// move's task.moved event.
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t-superseded",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step3",
			Title:          "Task",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Superseded destination", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))

		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID:     "t-superseded",
			FromStepID: "step1",
			ToStepID:   "step2",
			SessionID:  "",
		})

		// Settle any goroutine the (correctly guarded) call did not spawn, and
		// any the guard failed to prevent under mutation — synctest.Wait
		// blocks until every goroutine in the bubble is durably idle, so
		// failIfLaunched's t.Error would already have fired by the time this
		// returns.
		synctest.Wait()

		task, err := repo.GetTask(ctx, "t-superseded")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task.WorkflowStepID != "step3" {
			t.Fatalf("WorkflowStepID = %q, want step3 unchanged — the guard must return before touching the task", task.WorkflowStepID)
		}
	})
}
