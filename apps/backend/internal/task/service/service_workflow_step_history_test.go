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
	// ctxErr captures ctx.Err() at call time — used to prove the recorder
	// received a live (non-cancelled) context even when the caller's parent
	// context was cancelled before the record* helper ran.
	ctxErr error
}

func (f *fakeStepHistoryRecorder) CreateStepTransition(ctx context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedTransition{
		sessionID: sessionID, fromStepID: fromStepID, toStepID: toStepID,
		trigger: trigger, actorID: actorID, metadata: metadata,
		ctxErr: ctx.Err(),
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

func TestService_MoveTaskAgentDoesNotUseOwnerActor(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)
	createMoveTask(t, ctx, repo, "task-agent-history", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-agent-history", "task-agent-history", models.TaskSessionStateWaitingForInput, "")

	_, err := svc.MoveTaskWithOptions(ctx, "task-agent-history", "wf-source", "step-review-target", 0, MoveTaskOptions{
		StepHistoryActor: wfmodels.StepTransitionActorAgent,
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	if calls[0].actorID != nil {
		t.Fatalf("agent transition actorID = %v, want nil", *calls[0].actorID)
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

func TestService_ApproveSessionRecordsApprovalStepHistoryNotManual(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Approved", Position: 2,
	}
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	createMoveTask(t, ctx, repo, "task-approval-history", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-approval-history", "task-approval-history",
		models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)

	if _, err := svc.ApproveSession(ctx, "session-approval-history"); err != nil {
		t.Fatalf("ApproveSession: %v", err)
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.sessionID != "session-approval-history" || got.fromStepID != "step-review-target" || got.toStepID != "step-done" {
		t.Errorf("unexpected call: %+v", got)
	}
	if got.trigger != wfmodels.StepTransitionTriggerApproval {
		t.Errorf("trigger = %q, want approval (not manual)", got.trigger)
	}
}

// TestService_ApproveSessionRecordsApprovalStepHistoryForApprovedSessionNotPrimary
// covers a task with two active sessions where the primary session is NOT the
// one being approved. Review round 3 flagged that MoveTaskWithOptions re-derives
// the audit-row session via resolvePrimaryOrActiveSession instead of using the
// session ApproveSession actually approved, so the row was attributed to the
// primary session instead of the approved one.
func TestService_ApproveSessionRecordsApprovalStepHistoryForApprovedSessionNotPrimary(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Approved", Position: 2,
	}
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	createMoveTask(t, ctx, repo, "task-multi-session", "wf-source", "step-review-target", nil)
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-primary",
		TaskID:    "task-multi-session",
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
	}))
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:           "session-review",
		TaskID:       "task-multi-session",
		State:        models.TaskSessionStateWaitingForInput,
		IsPrimary:    false,
		ReviewStatus: models.ReviewStatusPending,
	}))

	if _, err := svc.ApproveSession(ctx, "session-review"); err != nil {
		t.Fatalf("ApproveSession: %v", err)
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.sessionID != "session-review" {
		t.Errorf("sessionID = %q, want session-review (the approved session, not the primary)", got.sessionID)
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

// TestService_RecordManualStepTransitionSurvivesCancelledParentContext covers
// Review round 3's finding: the audit insert previously ran on the caller's
// ctx after the step change already committed, so a cancelled parent context
// (client disconnect on httpMoveTask) could silently drop the row. The
// recorder must see a live context even when the caller passes a cancelled
// one.
func TestService_RecordManualStepTransitionSurvivesCancelledParentContext(t *testing.T) {
	svc, _, _ := createTestService(t)
	recorder := &fakeStepHistoryRecorder{}
	svc.SetStepHistoryRecorder(recorder)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.recordManualStepTransition(cancelledCtx, "session-x", "step-a", "step-b", wfmodels.StepTransitionTriggerManual)

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	if calls[0].ctxErr != nil {
		t.Errorf("recorder received a cancelled context: %v", calls[0].ctxErr)
	}
}
