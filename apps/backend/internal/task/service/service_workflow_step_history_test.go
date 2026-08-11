package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeStepHistoryRecorder captures CreateStepTransition calls for assertion.
type fakeStepHistoryRecorder struct {
	mu    sync.Mutex
	calls []recordedTransition
	err   error
}

type recordedTransition struct {
	sessionID            string
	fromStepID, toStepID string
	trigger              wfmodels.StepTransitionTrigger
	actorID              *string
	metadata             map[string]interface{}
}

func (f *fakeStepHistoryRecorder) CreateStepTransition(_ context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedTransition{
		sessionID: sessionID, fromStepID: fromStepID, toStepID: toStepID,
		trigger: trigger, actorID: actorID, metadata: metadata,
	})
	return f.err
}

func (f *fakeStepHistoryRecorder) Calls() []recordedTransition {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedTransition, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestService_MoveTaskRecordsManualStepHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	createMoveTask(t, ctx, repo, "task-history", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-history", "task-history", models.TaskSessionStateWaitingForInput, "")

	_, err := svc.MoveTask(ctx, "task-history", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.sessionID != "session-history" {
		t.Errorf("sessionID = %q, want session-history", got.sessionID)
	}
	if got.fromStepID != "step-source" {
		t.Errorf("fromStepID = %q, want step-source", got.fromStepID)
	}
	if got.toStepID != "step-review-target" {
		t.Errorf("toStepID = %q, want step-review-target", got.toStepID)
	}
	if got.trigger != wfmodels.StepTransitionTriggerManual {
		t.Errorf("trigger = %q, want manual", got.trigger)
	}
}

func TestService_MoveTaskSameStepDoesNotRecordHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	createMoveTask(t, ctx, repo, "task-samestep", "wf-source", "step-source", nil)

	// Reorder within the same step — position change only, no step change.
	_, err := svc.MoveTask(ctx, "task-samestep", "wf-source", "step-source", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	if calls := recorder.Calls(); len(calls) != 0 {
		t.Fatalf("expected no recorded transitions for a same-step move, got %d", len(calls))
	}
}

func TestService_MoveTaskWithoutSessionDoesNotRecordOrFail(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	// No session created for this task at all.
	createMoveTask(t, ctx, repo, "task-no-session", "wf-source", "step-source", nil)

	_, err := svc.MoveTask(ctx, "task-no-session", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("MoveTask should not fail when there is no session to record against: %v", err)
	}
	if calls := recorder.Calls(); len(calls) != 0 {
		t.Fatalf("expected no recorded transitions when sessionID is empty, got %d", len(calls))
	}
}

func TestService_MoveTaskHistoryWriteFailureDoesNotFailMove(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	recorder := &fakeStepHistoryRecorder{err: context.DeadlineExceeded}
	svc.SetStepHistoryRecorder(recorder)

	createMoveTask(t, ctx, repo, "task-history-fail", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-history-fail", "task-history-fail", models.TaskSessionStateWaitingForInput, "")

	moved, err := svc.MoveTask(ctx, "task-history-fail", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("MoveTask must not fail when the audit write fails: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected task to still move to step-review-target, got %s", moved.Task.WorkflowStepID)
	}
}
