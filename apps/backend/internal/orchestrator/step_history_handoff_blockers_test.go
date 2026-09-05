package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestExecuteStepTransition_RecordsHandoffAndBlockersMetadata covers
// AC-002.1: the legacy funnel's audit metadata gains signal_blockers and
// signal_handoff alongside signal_source/signal_summary.
func TestExecuteStepTransition_RecordsHandoffAndBlockersMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder

	signal := models.PendingStepCompletionSignal{
		StepID:   "step1",
		Source:   models.StepCompletionSourceAgent,
		Summary:  "did the work",
		Handoff:  "watch out for X",
		Blockers: "needs a design decision",
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
	if got.metadata["signal_handoff"] != "watch out for X" {
		t.Errorf("metadata[signal_handoff] = %v, want 'watch out for X'", got.metadata["signal_handoff"])
	}
	if got.metadata["signal_blockers"] != "needs a design decision" {
		t.Errorf("metadata[signal_blockers] = %v, want 'needs a design decision'", got.metadata["signal_blockers"])
	}
}

// TestExecuteStepTransition_OmitsBlankHandoffAndBlockers covers AC-002.2: an
// empty or whitespace-only handoff/blockers is omitted from the audit
// metadata entirely rather than written as an empty entry.
func TestExecuteStepTransition_OmitsBlankHandoffAndBlockers(t *testing.T) {
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
	if _, ok := got.metadata["signal_handoff"]; ok {
		t.Errorf("metadata must omit signal_handoff when blank, got %v", got.metadata["signal_handoff"])
	}
	if _, ok := got.metadata["signal_blockers"]; ok {
		t.Errorf("metadata must omit signal_blockers when blank, got %v", got.metadata["signal_blockers"])
	}
}

// TestApplyEngineTransition_RecordsHandoffAndBlockersMetadata covers the
// engine funnel's half of AC-002.1, proving both consuming-transition sites
// were updated (the design's named drift risk).
func TestApplyEngineTransition_RecordsHandoffAndBlockersMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder
	onEnterDone := make(chan struct{})
	svc.onProcessOnEnterComplete = func() { close(onEnterDone) }

	signal := models.PendingStepCompletionSignal{
		StepID:   "step1",
		Source:   models.StepCompletionSourceAgent,
		Summary:  "engine path done",
		Handoff:  "engine handoff text",
		Blockers: "engine blocker text",
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
	<-onEnterDone

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded transition, got %d", len(calls))
	}
	got := calls[0]
	if got.metadata["signal_handoff"] != "engine handoff text" {
		t.Errorf("metadata[signal_handoff] = %v, want 'engine handoff text'", got.metadata["signal_handoff"])
	}
	if got.metadata["signal_blockers"] != "engine blocker text" {
		t.Errorf("metadata[signal_blockers] = %v, want 'engine blocker text'", got.metadata["signal_blockers"])
	}
}
