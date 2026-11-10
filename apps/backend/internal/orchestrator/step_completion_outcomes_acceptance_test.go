package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func configureReviewDestination(t *testing.T, fixture *profileSwitchFixture, reviewStepID, prompt string) {
	t.Helper()
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Build", Position: 0,
		AgentProfileID:            "profile-a",
		ProfileSessionEndPolicy:   models.WorkflowProfileSessionEndPolicyPark,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": reviewStepID},
		}}},
	}
	fixture.stepGetter.steps[reviewStepID] = &wfmodels.WorkflowStep{
		ID: reviewStepID, WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID:            "profile-b",
		Prompt:                    prompt,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type: wfmodels.OnEnterAutoStartAgent,
		}}},
	}
	now := time.Now().UTC()
	require.NoError(t, fixture.repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "session-review", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, IsPrimary: false, StartedAt: now, UpdatedAt: now,
	}))
	seedExecutorRunning(t, fixture.repo, "session-review", "t1", "execution-b")
	fixture.agentMgr.isAgentRunning = true
}

func claimReviewHandoff(t *testing.T, fixture *profileSwitchFixture, turnID, summary, handoff string) models.PendingStepCompletionSignal {
	t.Helper()
	entry := signalEntryForSession(t, fixture.repo, fixture.current.ID)
	stored, claimed, err := fixture.repo.ClaimStepCompletionSignal(context.Background(), "t1", fixture.current.ID, models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step-a", SourceTurnID: turnID, Summary: summary, Handoff: handoff,
	})
	require.NoError(t, err)
	require.True(t, claimed)
	return stored
}

func armOnEnterWait(fixture *profileSwitchFixture) <-chan struct{} {
	done := make(chan struct{})
	fixture.svc.onProcessOnEnterComplete = func() { close(done) }
	return done
}

func waitForOnEnter(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("destination on_enter did not complete")
	}
}

// TestStepCompletionOutcome_ReviewRecipientGetsLatestHandoffOnce covers the source-specific completion contract.
func TestStepCompletionOutcome_ReviewRecipientGetsLatestHandoffOnce(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	configureReviewDestination(t, fixture, "step-review", "Review the change.")
	fixture.svc.initWorkflowEngine()

	// An earlier accepted source is replaced by a later turn before commit.
	now := time.Now().UTC()
	_, err := fixture.repo.CreateTurnWithStepStamp(ctx, &models.Turn{
		ID: "turn-1", TaskSessionID: fixture.current.ID, TaskID: "t1",
		StartedAt: now.Add(-2 * time.Second), CreatedAt: now.Add(-2 * time.Second),
	})
	require.NoError(t, err)
	claimReviewHandoff(t, fixture, "turn-1", "old summary", "old review handoff")
	_, err = fixture.repo.CreateTurnWithStepStamp(ctx, &models.Turn{
		ID: "turn-2", TaskSessionID: fixture.current.ID, TaskID: "t1",
		StartedAt: now, CreatedAt: now,
	})
	require.NoError(t, err)
	latest := claimReviewHandoff(t, fixture, "turn-2", "latest summary", "latest review handoff")

	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	done := armOnEnterWait(fixture)
	require.True(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session))
	waitForOnEnter(t, done)

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-review", task.WorkflowStepID)
	review, err := fixture.repo.GetTaskSession(ctx, "session-review")
	require.NoError(t, err)
	require.True(t, review.IsPrimary, "the configured Review recipient becomes primary")
	require.NotEqual(t, fixture.current.ID, review.ID)

	fixture.agentMgr.mu.Lock()
	calls := append([]promptCall(nil), fixture.agentMgr.capturedPromptCalls...)
	fixture.agentMgr.mu.Unlock()
	require.Len(t, calls, 1, "the Review recipient must receive exactly one prompt")
	require.Equal(t, "execution-b", calls[0].ExecutionID)
	require.Contains(t, calls[0].Prompt, "Review the change.")
	require.Contains(t, calls[0].Prompt, "latest review handoff")
	require.NotContains(t, calls[0].Prompt, "old review handoff")

	sourceReceipt, ok, err := fixture.repo.GetStepCompletionReceipt(ctx, fixture.current.ID, latest.SignalID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, sourceReceipt.Outcome)
	require.Equal(t, "step-review", sourceReceipt.Transition.StepID)
}

// TestStepCompletionOutcome_CredentialRefusalKeepsSourceRoute covers the source-specific completion contract.
func TestStepCompletionOutcome_CredentialRefusalKeepsSourceRoute(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	configureDestinationOnlyCredentialPreflightFailure(t, fixture)
	fixture.stepGetter.steps["step-a"].AutoAdvanceRequiresSignal = true
	fixture.svc.initWorkflowEngine()

	stored := claimReviewHandoff(t, fixture, "", "destination needs credentials", "credential handoff")
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.False(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session))
	consumed, held := fixture.svc.turnCompletionConsumedGeneration.Load(fixture.current.ID)
	require.True(t, held)
	require.Equal(t, session.UpdatedAt, consumed, "a deterministic refusal must not release the generation")
	require.False(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session),
		"an unchanged refused signal must not repeat the preflight")

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-a", task.WorkflowStepID)
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.True(t, source.IsPrimary)
	require.Equal(t, "profile-a", source.AgentProfileID)
	destination, err := fixture.repo.GetTaskSession(ctx, "session-destination")
	require.NoError(t, err)
	require.False(t, destination.IsPrimary)
	require.Empty(t, fixture.agentMgr.capturedPrompts, "refused destination must not run")

	receipt, ok, err := fixture.repo.GetStepCompletionReceipt(ctx, fixture.current.ID, stored.SignalID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeBlocked, receipt.Outcome)
	require.Equal(t, models.StepCompletionReasonDestinationUnavailable, receipt.Reason)
	require.Nil(t, receipt.Transition)
	pending, hasPending := models.LoadPendingStepSignal(source.Metadata)
	require.True(t, hasPending)
	require.Equal(t, stored.SignalID, pending.SignalID)

}

// TestStepCompletionOutcome_QueuedReviewAndPromotion covers the source-specific completion contract.
func TestStepCompletionOutcome_QueuedReviewAndPromotion(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	configureReviewDestination(t, fixture, "step-review", "Review the change.")
	fixture.stepGetter.steps["step-review"].WIPLimit = 1
	fixture.svc.initWorkflowEngine()
	require.NoError(t, fixture.repo.CreateTask(ctx, &models.Task{
		ID: "occupant", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-review",
		Title: "Occupant", WIPAdmitted: true,
	}))

	latest := claimReviewHandoff(t, fixture, "", "queued summary", "queued review handoff")
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.True(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session))

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-review", task.WorkflowStepID)
	require.Equal(t, "step-review", task.QueuedForStepID)
	require.False(t, task.WIPAdmitted)
	require.Empty(t, fixture.agentMgr.capturedPrompts, "a queued destination must not receive its prompt yet")
	receipt, ok, err := fixture.repo.GetStepCompletionReceipt(ctx, fixture.current.ID, latest.SignalID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeDestinationQueued, receipt.Outcome)
	require.True(t, receipt.Transition.Queued)

	// Reopen repository state and promote after the occupant vacates the step.
	reopened := sqliterepo.NewWithInitializedDB(sqlx.NewDb(fixture.repo.DB(), "sqlite3"), sqlx.NewDb(fixture.repo.DB(), "sqlite3"), nil)
	occupant, err := reopened.GetTask(ctx, "occupant")
	require.NoError(t, err)
	occupant.WorkflowStepID = "step-a"
	require.NoError(t, reopened.UpdateTask(ctx, occupant))
	queued, err := reopened.GetTask(ctx, "t1")
	require.NoError(t, err)
	queued.WIPAdmitted = true
	queued.QueuedForStepID = ""
	queued.QueuedAt = nil
	if queued.Metadata == nil {
		queued.Metadata = map[string]interface{}{}
	}
	queued.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{"from_step_id": "step-review"}
	claimed, err := reopened.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, queued, "step-review", "step-review", 1)
	require.NoError(t, err)
	require.True(t, claimed)

	handleQueuePromotionAndWait(t, fixture)

	promoted, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.True(t, promoted.WIPAdmitted)
	require.Empty(t, promoted.QueuedForStepID)
	review, err := fixture.repo.GetTaskSession(ctx, "session-review")
	require.NoError(t, err)
	require.True(t, review.IsPrimary)
	fixture.agentMgr.mu.Lock()
	calls := append([]promptCall(nil), fixture.agentMgr.capturedPromptCalls...)
	fixture.agentMgr.mu.Unlock()
	require.Len(t, calls, 1, "promotion must deliver the winning handoff once")
	require.Equal(t, "execution-b", calls[0].ExecutionID)
	require.Contains(t, calls[0].Prompt, "queued review handoff")
}

// TestStepCompletionOutcome_RequestChangesIterateReviewIgnoresStaleEvent covers the source-specific completion contract.
func TestStepCompletionOutcome_RequestChangesIterateReviewIgnoresStaleEvent(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	configureReviewDestination(t, fixture, "step-review", "Review the change.")
	fixture.stepGetter.steps["step-iterate"] = &wfmodels.WorkflowStep{
		ID: "step-iterate", WorkflowID: "wf1", Name: "Iterate", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step-review"},
		}}},
	}
	fixture.svc.initWorkflowEngine()

	first := claimReviewHandoff(t, fixture, "", "build done", "first review handoff")
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	done := armOnEnterWait(fixture)
	require.True(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session))
	waitForOnEnter(t, done)

	// The reviewer requests changes: the task moves to Iterate.
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-iterate"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	entry := signalEntryForSession(t, fixture.repo, "session-review")
	second, claimed, err := fixture.repo.ClaimStepCompletionSignal(ctx, "t1", "session-review", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step-iterate", Summary: "changes addressed", Handoff: "second review handoff",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	session, err = fixture.repo.GetTaskSession(ctx, "session-review")
	require.NoError(t, err)
	done = armOnEnterWait(fixture)
	require.True(t, fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", session))
	waitForOnEnter(t, done)

	task, err = fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-review", task.WorkflowStepID)
	_, _, reviewEntry, err := fixture.repo.GetTaskWorkflowStepEntry(ctx, "t1")
	require.NoError(t, err)
	require.Greater(t, reviewEntry, *first.WorkflowStepTransitionID)

	// A delayed event for the earlier Build entry cannot move or re-prompt.
	promptCount := len(fixture.agentMgr.capturedPromptCalls)
	require.Nil(t, fixture.svc.onStepCompletionSignaled(ctx, stepCompletionSignalEvent("t1", fixture.current.ID, "step-a")))
	require.Equal(t, promptCount, len(fixture.agentMgr.capturedPromptCalls))
	task, err = fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-review", task.WorkflowStepID)

	fixture.agentMgr.mu.Lock()
	calls := append([]promptCall(nil), fixture.agentMgr.capturedPromptCalls...)
	fixture.agentMgr.mu.Unlock()
	require.Len(t, calls, 2)
	require.Contains(t, calls[0].Prompt, "first review handoff")
	require.Contains(t, calls[1].Prompt, "second review handoff")
	require.Equal(t, "execution-b", calls[1].ExecutionID)

	receipt, ok, err := fixture.repo.GetStepCompletionReceipt(ctx, "session-review", second.SignalID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
}
