package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestProcessTaskPRWorkflowEvent_IgnoresMergeBeforeStepArrival pins that a
// merge that happened before the task entered its current step never fires,
// so a later poller refresh of an already merged PR cannot replay it.
func TestProcessTaskPRWorkflowEvent_IgnoresMergeBeforeStepArrival(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	mergedAt := time.Now().UTC().Add(-time.Hour)
	merged := linkedTaskPR(1, "merged")
	merged.MergedAt = &mergedAt
	f.github.taskPRs = []*github.TaskPR{merged}

	if f.process(t, merged) {
		t.Fatal("expected a merge older than the step arrival to be ignored")
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept", got)
	}

	later := time.Now().UTC().Add(time.Second)
	merged.MergedAt = &later
	if !f.process(t, merged) {
		t.Fatal("expected a merge after the step arrival to fire")
	}
	f.waitOnEnter(t)
}

// TestApplyTaskPRWorkflowTransition_DoesNotOverwriteNewerMove pins the
// compare-and-swap commit: a decision evaluated on step_accept must not move a
// task that has since been moved elsewhere.
func TestApplyTaskPRWorkflowTransition_DoesNotOverwriteNewerMove(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	ctx := context.Background()
	task, err := f.repo.GetTask(ctx, prWorkflowTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	session, err := f.repo.GetTaskSession(ctx, prWorkflowSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	moved := *task
	moved.WorkflowStepID = "step_exec"
	if err := f.repo.UpdateTask(ctx, &moved); err != nil {
		t.Fatalf("move task to step_exec: %v", err)
	}

	applied, err := f.svc.applyTaskPRWorkflowTransition(ctx, task, session, true, engine.HandleResult{
		Transitioned: true, FromStepID: "step_accept", ToStepID: "step_done",
	}, engine.TriggerOnPRMerged)
	if err != nil {
		t.Fatalf("applyTaskPRWorkflowTransition: %v", err)
	}
	if applied {
		t.Fatal("expected a stale decision not to apply")
	}
	if got := f.taskStep(t); got != "step_exec" {
		t.Fatalf("task step = %q, want step_exec (newer move kept)", got)
	}
	if got := len(f.recorder.Calls()); got != 0 {
		t.Fatalf("step history calls = %d, want 0", got)
	}
}

// TestHandleTaskPRWorkflowEvent_RetriesAfterError pins that a failed
// evaluation is retried instead of dropped, since the poller publishes no new
// event for an unchanged PR.
func TestHandleTaskPRWorkflowEvent_RetriesAfterError(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	f.svc.taskPRWorkflowRetryDelays = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	merged := linkedTaskPR(1, "merged")
	var calls atomic.Int32
	f.github.taskPRListFunc = func(context.Context, []string) (map[string][]*github.TaskPR, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("database is locked")
		}
		return map[string][]*github.TaskPR{prWorkflowTaskID: {merged}}, nil
	}

	f.svc.handleTaskPRWorkflowEvent(context.Background(), merged)
	f.waitOnEnter(t)
	if got := f.taskStep(t); got != "step_done" {
		t.Fatalf("task step = %q, want step_done after retry", got)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("ListTaskPRs calls = %d, want a retry", got)
	}
}

// TestHandleTaskPRWorkflowEvent_RetryStopsWhenPRReopened pins that a retry
// re-checks the stored PR: a close that failed must not move the task once the
// PR has been reopened.
func TestHandleTaskPRWorkflowEvent_RetryStopsWhenPRReopened(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	f.svc.taskPRWorkflowRetryDelays = []time.Duration{10 * time.Millisecond}
	closedAt := time.Now().UTC()
	closed := linkedTaskPR(1, "closed")
	closed.ClosedAt = &closedAt
	reopened := linkedTaskPR(1, "open")
	var calls atomic.Int32
	f.github.taskPRListFunc = func(context.Context, []string) (map[string][]*github.TaskPR, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("database is locked")
		}
		return map[string][]*github.TaskPR{prWorkflowTaskID: {reopened}}, nil
	}

	f.svc.handleTaskPRWorkflowEvent(context.Background(), closed)
	f.svc.ciAutomationWorkers.Wait()

	if got := calls.Load(); got != 2 {
		t.Fatalf("ListTaskPRs calls = %d, want the failed attempt plus one retry", got)
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept (PR was reopened)", got)
	}
}

// TestProcessTaskPRWorkflowEvent_DelayedCloseAfterReopenIgnored pins that the
// first delivery is checked against the stored PR too: a late closed snapshot
// does not move the task once the stored PR is open again.
func TestProcessTaskPRWorkflowEvent_DelayedCloseAfterReopenIgnored(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	closedAt := time.Now().UTC()
	closed := linkedTaskPR(1, "closed")
	closed.ClosedAt = &closedAt
	f.github.taskPRs = []*github.TaskPR{linkedTaskPR(1, "open")}

	if f.process(t, closed) {
		t.Fatal("expected a delayed close of a reopened PR to be ignored")
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept", got)
	}
}
