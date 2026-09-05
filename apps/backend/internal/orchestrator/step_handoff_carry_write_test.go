package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

func carryToken(t *testing.T, repo interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
}, taskID string) (models.StepHandoffCarryToken, bool) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	raw, present := task.Metadata[models.MetaKeyStepHandoffCarry]
	if !present {
		return models.StepHandoffCarryToken{}, false
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal carry token: %v", err)
	}
	var token models.StepHandoffCarryToken
	if err := json.Unmarshal(bytes, &token); err != nil {
		t.Fatalf("unmarshal carry token: %v", err)
	}
	return token, true
}

// TestExecuteStepTransition_WritesHandoffCarryToken covers AC-001.2 on the
// legacy funnel: a consumed signal carrying a non-blank handoff is written as
// a carry token addressed to the destination step.
func TestExecuteStepTransition_WritesHandoffCarryToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}

	signal := models.PendingStepCompletionSignal{
		StepID:  "step1",
		Source:  models.StepCompletionSourceAgent,
		Summary: "did the work",
		Handoff: "watch out for X",
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", true)

	token, present := carryToken(t, repo, "t1")
	if !present {
		t.Fatal("expected a carry token to be written")
	}
	if token.Handoff != "watch out for X" || token.StepID != "step2" || token.Stamp == "" {
		t.Fatalf("token = %+v, want handoff for step2 with a stamp", token)
	}
}

// TestExecuteStepTransition_BlankHandoffRemovesToken covers AC-001.2's other
// half: a consumed signal with no handoff text removes any existing token
// rather than writing a blank one.
func TestExecuteStepTransition_BlankHandoffRemovesToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "stale", StepID: "step2", Stamp: "stale-stamp",
	}); err != nil {
		t.Fatalf("seed stale token: %v", err)
	}
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}

	signal := models.PendingStepCompletionSignal{StepID: "step1", Source: models.StepCompletionSourceAgent, Summary: "done"}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", true)

	if _, present := carryToken(t, repo, "t1"); present {
		t.Fatal("expected the stale token to be removed when the consumed signal has no handoff")
	}
}

// TestExecuteStepTransition_OnTurnStartDoesNotTouchToken covers AC-001.2a: a
// non-consuming (on_turn_start) transition must not write or remove the
// token, since only the consuming transition may act on it.
func TestExecuteStepTransition_OnTurnStartDoesNotTouchToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "keep me", StepID: "step3", Stamp: "kept-stamp",
	}); err != nil {
		t.Fatalf("seed existing token: %v", err)
	}
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}

	signal := models.PendingStepCompletionSignal{StepID: "step1", Source: models.StepCompletionSourceAgent, Summary: "premature"}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.executeStepTransition(ctx, "t1", "s1", sg.steps["step1"], "step2", false)

	token, present := carryToken(t, repo, "t1")
	if !present || token.StepID != "step3" || token.Handoff != "keep me" {
		t.Fatalf("on_turn_start transition must leave the existing token untouched, got present=%v token=%+v", present, token)
	}
}

// TestApplyEngineTransition_WritesHandoffCarryToken covers AC-001.2 on the
// engine funnel, proving both consuming-transition sites carry the token.
func TestApplyEngineTransition_WritesHandoffCarryToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}
	onEnterDone := make(chan struct{})
	svc.onProcessOnEnterComplete = func() { close(onEnterDone) }

	signal := models.PendingStepCompletionSignal{
		StepID:  "step1",
		Source:  models.StepCompletionSourceAgent,
		Summary: "engine path done",
		Handoff: "engine handoff text",
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

	token, present := carryToken(t, repo, "t1")
	if !present {
		t.Fatal("expected a carry token to be written by the engine funnel")
	}
	if token.StepID != "step2" {
		// The dispatch side may have already claimed it as part of on_enter
		// processing once wired; either the token is still addressed to
		// step2 or it has already been claimed (removed). Both are
		// acceptable outcomes here — what must NOT happen is a token
		// addressed to any other step.
		t.Fatalf("token = %+v, want step2", token)
	}
}

// TestApplyEngineTransition_OnTurnStartDoesNotTouchToken covers AC-001.2a on
// the engine funnel: a non-consuming trigger must not touch the token.
func TestApplyEngineTransition_OnTurnStartDoesNotTouchToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "keep me", StepID: "step3", Stamp: "kept-stamp",
	}); err != nil {
		t.Fatalf("seed existing token: %v", err)
	}
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}

	signal := models.PendingStepCompletionSignal{StepID: "step1", Source: models.StepCompletionSourceAgent, Summary: "premature"}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	result := engine.HandleResult{Transitioned: true, FromStepID: "step1", ToStepID: "step2"}
	transitioned := svc.applyEngineTransition(ctx, "t1", session, result, engine.TriggerOnTurnStart, "desc", false)
	if !transitioned {
		t.Fatalf("expected transition to apply")
	}

	token, present := carryToken(t, repo, "t1")
	if !present || token.StepID != "step3" || token.Handoff != "keep me" {
		t.Fatalf("on_turn_start transition must leave the existing token untouched, got present=%v token=%+v", present, token)
	}
}

// TestApplyEngineTransition_GuardedDecisionDoesNotTouchToken is the F15
// guarded-decision carve-out: a quorum/approval decision carries
// TriggerOnTurnComplete too, with no turn having ended, so a trigger-only
// gate would wipe an unclaimed token the instant a reviewer approves. Only
// the compound `sessionLifecycle && trigger == TriggerOnTurnComplete` gate
// must apply.
func TestApplyEngineTransition_GuardedDecisionDoesNotTouchToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "unclaimed", StepID: "step3", Stamp: "unclaimed-stamp",
	}); err != nil {
		t.Fatalf("seed existing token: %v", err)
	}
	sg := twoStepGetter()
	svc := createTestService(repo, sg, newMockTaskRepo())
	svc.SetWorkflowStepGetter(sg)
	svc.stepHistoryRecorder = &fakeStepHistoryRecorder{}

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	result := engine.HandleResult{Transitioned: true, FromStepID: "step1", ToStepID: "step2"}
	applied := svc.applyEngineTransitionWithCommitMode(
		ctx, "t1", session, result, engine.TriggerOnTurnComplete, "desc",
		transitionLifecycleGuardedDecision,
		func(commitCtx context.Context) (bool, error) { return true, nil },
	)
	if !applied {
		t.Fatalf("expected guarded decision transition to apply")
	}

	token, present := carryToken(t, repo, "t1")
	if !present || token.StepID != "step3" || token.Handoff != "unclaimed" {
		t.Fatalf("guarded decision must leave the unclaimed token untouched, got present=%v token=%+v", present, token)
	}
}
