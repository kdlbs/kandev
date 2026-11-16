package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// turnlessDeliveryTerminal is a durable delivery terminal whose identity is
// complete except for the turn ID, as published when the prompt was
// dispatched without a Kandev turn.
func turnlessDeliveryTerminal(taskID, sessionID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID:                    taskID,
		SessionID:                 sessionID,
		DeliveryStreamID:          "incarnation-1:g1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 1,
		DeliverySequence:          42,
		DeliverySubmissionID:      "submission-1",
	}
}

// A turn-less delivery terminal is bound to the turn its handler closes, so
// on_turn_complete runs instead of being deferred with an empty effect key.
func TestWorkflowEffectForCompletionBindsClosingTurn(t *testing.T) {
	effect := workflowEffectForCompletion(turnlessDeliveryTerminal("t1", "s1"), "turn-closing")
	require.NotNil(t, effect)
	require.Equal(t, workflowEffectKey("turn-closing"), effect.EffectKey)
	require.True(t, isBoundAgentDeliveryWorkflowEffect(effect),
		"a terminal whose only missing field was the turn must be fully bound")
}

func TestWorkflowEffectForCompletionKeepsOtherEvents(t *testing.T) {
	withTurn := turnlessDeliveryTerminal("t1", "s1")
	withTurn.TurnID = "turn-own"
	effect := workflowEffectForCompletion(withTurn, "turn-closing")
	require.Equal(t, workflowEffectKey("turn-own"), effect.EffectKey, "an event's own turn wins")

	require.Nil(t, workflowEffectForCompletion(watcher.AgentEventData{TaskID: "t1", SessionID: "s1"}, "turn-closing"),
		"an event without delivery identity keeps its existing no-effect behavior")

	unbound := workflowEffectForCompletion(turnlessDeliveryTerminal("t1", "s1"), "")
	require.False(t, isBoundAgentDeliveryWorkflowEffect(unbound),
		"without a resolved closing turn the terminal stays fail-closed")
}

func TestCompletedEventClosingTurnUsesActiveTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	require.Equal(t, turn.ID, svc.completedEventClosingTurn(ctx, turnlessDeliveryTerminal("t1", "s1")))
	require.Empty(t, svc.completedEventClosingTurn(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"}),
		"an event without delivery identity is not bound to the active turn")
}

// A completion that still has to be deferred must not end silently: an Office
// session parks IDLE, outside both watchdog gates, so its pending signal is
// reported at the deferral. It is reported only, never advanced.
func TestDeferredOfficeCompletionSurfacesPendingSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateRunning)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	before := officeStrandedCount(stuckSignalGateDeferredCompletion)
	deferredCtx := withWorkflowEffect(ctx, workflowEffectForEvent(turnlessDeliveryTerminal("t1", "s1")))
	require.False(t, svc.processOnTurnCompleteViaEngine(deferredCtx, "t1", session))

	require.Equal(t, int64(1), officeStrandedCount(stuckSignalGateDeferredCompletion)-before)
	assertOfficeStallNotActedOn(t, repo, "t1", "s1", "step1")
}

func TestDeferredOfficeCompletionStaysDeduplicatedWhileIdle(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateIdle)

	before := officeStrandedCount(stuckSignalGateDeferredCompletion)
	svc.surfaceDeferredOfficeCompletion(ctx, "t1", "s1")
	for range 3 {
		svc.reclaimStuckSignalSessionsOnce(ctx)
		svc.surfaceDeferredOfficeCompletion(ctx, "t1", "s1")
	}

	require.Equal(t, int64(1), officeStrandedCount(stuckSignalGateDeferredCompletion)-before)
	assertOfficeStallNotActedOn(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateIdle, session.State)
}
