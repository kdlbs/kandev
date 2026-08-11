package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeStepHistoryRecorder captures CreateStepTransition calls for assertion.
type fakeStepHistoryRecorder struct {
	mu    sync.Mutex
	calls []recordedStepTransition
	err   error
}

type recordedStepTransition struct {
	sessionID            string
	fromStepID, toStepID string
	trigger              wfmodels.StepTransitionTrigger
	actorID              *string
	metadata             map[string]interface{}
}

func (f *fakeStepHistoryRecorder) CreateStepTransition(_ context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedStepTransition{
		sessionID: sessionID, fromStepID: fromStepID, toStepID: toStepID,
		trigger: trigger, actorID: actorID, metadata: metadata,
	})
	return f.err
}

func (f *fakeStepHistoryRecorder) Calls() []recordedStepTransition {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedStepTransition, len(f.calls))
	copy(out, f.calls)
	return out
}

func twoStepGetter() *mockStepGetter {
	sg := newMockStepGetter()
	sg.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0}
	sg.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
	return sg
}

// --- Legacy path (executeStepTransition) ---

func TestExecuteStepTransition_RecordsAutoStepHistoryOnTurnComplete(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", true)

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.sessionID != "s1" || got.fromStepID != "step1" || got.toStepID != "step2" {
		t.Errorf("unexpected call: %+v", got)
	}
	if got.trigger != wfmodels.StepTransitionTriggerAutoComplete {
		t.Errorf("trigger = %q, want auto_complete", got.trigger)
	}
	if got.metadata != nil {
		t.Errorf("metadata = %v, want nil (no pending signal)", got.metadata)
	}
}

func TestExecuteStepTransition_RecordsConsumedSignalMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	signal := models.PendingStepCompletionSignal{
		StepID:  "step1",
		Source:  models.StepCompletionSourceAgent,
		Summary: "did the work",
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", true)

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.metadata["signal_source"] != models.StepCompletionSourceAgent {
		t.Errorf("metadata[signal_source] = %v, want %q", got.metadata["signal_source"], models.StepCompletionSourceAgent)
	}
	if got.metadata["signal_summary"] != "did the work" {
		t.Errorf("metadata[signal_summary] = %v, want 'did the work'", got.metadata["signal_summary"])
	}
}

func TestExecuteStepTransition_OnTurnStartDoesNotAttachSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	// A signal is pending for step1, but this is an on_turn_start move
	// (triggerOnEnter=false) — it must not consume or report the signal.
	signal := models.PendingStepCompletionSignal{StepID: "step1", Source: models.StepCompletionSourceAgent, Summary: "premature"}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", false)

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.trigger != wfmodels.StepTransitionTriggerAutoComplete {
		t.Errorf("trigger = %q, want auto_complete", got.trigger)
	}
	if got.metadata != nil {
		t.Errorf("metadata = %v, want nil (on_turn_start does not consume the signal)", got.metadata)
	}
}

func TestExecuteStepTransition_HistoryWriteFailureDoesNotBlockTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{err: context.DeadlineExceeded}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", true)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("expected transition to still apply despite audit failure, got step %q", task.WorkflowStepID)
	}
}

// --- Engine path (applyEngineTransition) ---

func TestApplyEngineTransition_RecordsConsumedSignalMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	signal := models.PendingStepCompletionSignal{
		StepID:  "step1",
		Source:  models.StepCompletionSourceAgent,
		Summary: "engine path done",
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	result := engine.HandleResult{Transitioned: true, FromStepID: "step1", ToStepID: "step2"}
	transitioned := svc.applyEngineTransition(ctx, "t1", session, result, engine.TriggerOnTurnComplete, "desc", true)
	if !transitioned {
		t.Fatalf("expected transition to apply")
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.sessionID != "s1" || got.fromStepID != "step1" || got.toStepID != "step2" {
		t.Errorf("unexpected call: %+v", got)
	}
	if got.trigger != wfmodels.StepTransitionTriggerAutoComplete {
		t.Errorf("trigger = %q, want auto_complete", got.trigger)
	}
	if got.metadata["signal_source"] != models.StepCompletionSourceAgent {
		t.Errorf("metadata[signal_source] = %v, want %q", got.metadata["signal_source"], models.StepCompletionSourceAgent)
	}
	if got.metadata["signal_summary"] != "engine path done" {
		t.Errorf("metadata[signal_summary] = %v, want 'engine path done'", got.metadata["signal_summary"])
	}
}

func TestApplyEngineTransition_OnTurnStartDoesNotAttachSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	signal := models.PendingStepCompletionSignal{StepID: "step1", Source: models.StepCompletionSourceAgent, Summary: "premature"}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	result := engine.HandleResult{Transitioned: true, FromStepID: "step1", ToStepID: "step2"}
	transitioned := svc.applyEngineTransition(ctx, "t1", session, result, engine.TriggerOnTurnStart, "", false)
	if !transitioned {
		t.Fatalf("expected transition to apply")
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.trigger != wfmodels.StepTransitionTriggerAutoComplete {
		t.Errorf("trigger = %q, want auto_complete", got.trigger)
	}
	if got.metadata != nil {
		t.Errorf("metadata = %v, want nil (on_turn_start does not consume the signal)", got.metadata)
	}
}

func TestApplyEngineTransition_ChildrenCompletedRecordsNoSignalMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	result := engine.HandleResult{Transitioned: true, FromStepID: "step1", ToStepID: "step2"}
	transitioned := svc.applyEngineTransition(ctx, "t1", session, result, engine.TriggerOnChildrenCompleted, "desc", true)
	if !transitioned {
		t.Fatalf("expected transition to apply")
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	if calls[0].trigger != wfmodels.StepTransitionTriggerAutoComplete {
		t.Errorf("trigger = %q, want auto_complete", calls[0].trigger)
	}
	if calls[0].metadata != nil {
		t.Errorf("metadata = %v, want nil (on_children_completed never consumes a signal)", calls[0].metadata)
	}
}
