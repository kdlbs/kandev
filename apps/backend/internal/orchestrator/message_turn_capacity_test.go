package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func markTargetPromptReady(t *testing.T, svc *Service, taskID, sessionID, executionID string) {
	t.Helper()
	registry := svc.resumeAttemptStore()
	attempt, owner := registry.begin(context.Background(), taskID, sessionID)
	require.True(t, owner)
	attempt.setExecutionID(executionID)
	require.True(t, registry.markPromptReady(attempt))
	attempt.finish(registry)
}

func capacityTestService(
	t *testing.T, ceiling int,
) (*Service, *mockAgentManager, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	manager := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = withTestCapabilities(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
	svc.sessionCeiling = newSessionCeilingController(ceiling, nil, nil)
	return svc, manager, repo
}

// TestMessageTurnCapacity_WarmIdleRefusesAtLimit is the issue #4998
// regression: a warm, prompt-ready runtime must not start a new turn while the
// enabled ceiling is full, through either the public manual wrapper or an
// automatic dispatch.
func TestMessageTurnCapacity_WarmIdleRefusesAtLimit(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "filler-task", "filler-session", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "target-session", "target-task", "target-exec")
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler-task", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})
	markTargetPromptReady(t, svc, "target-task", "target-session", "target-exec")

	_, err := svc.PromptTask(ctx, "target-task", "target-session", "manual follow-up", "", false, nil, false)
	require.True(t, IsMessageTurnCapacityRefusal(err),
		"public PromptTask must be bounded at full capacity, got %v", err)

	_, err = svc.promptTask(ctx, "target-task", "target-session", "automatic follow-up", "", false, nil, false,
		launchOriginAutomatic, promptTaskOptions{claimEntryID: "queued-entry-1"})
	require.True(t, IsMessageTurnCapacityRefusal(err),
		"automatic warm dispatch must be bounded at full capacity, got %v", err)
	require.Empty(t, manager.capturedPrompts, "no provider turn may start while capacity is full")

	// Releasing the counted filler admits exactly the next warm turn.
	svc.releaseCeilingReservation("filler-session")
	_, err = svc.PromptTask(ctx, "target-task", "target-session", "after release", "", false, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"after release"}, manager.capturedPrompts)
}

// TestMessageTurnCapacity_ColdResumeRefusesBeforeLaunch proves the bounded
// decision precedes cold resume work.
func TestMessageTurnCapacity_ColdResumeRefusesBeforeLaunch(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "filler-task", "filler-session", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "target-session", "target-task", "target-exec")
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler-task", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})
	manager.isAgentRunning = false

	_, err := svc.PromptTask(ctx, "target-task", "target-session", "cold follow-up", "", false, nil, false)
	require.True(t, IsMessageTurnCapacityRefusal(err), "cold resume must be bounded, got %v", err)
	require.Empty(t, manager.capturedPrompts)
}

// TestMessageTurnCapacity_ExplicitOverrideIsAdmitted keeps Send Now/Start/
// Resume authority separate from the bounded message policy.
func TestMessageTurnCapacity_ExplicitOverrideIsAdmitted(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "filler-task", "filler-session", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "target-session", "target-task", "target-exec")
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler-task", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})
	markTargetPromptReady(t, svc, "target-task", "target-session", "target-exec")

	_, err := svc.promptTask(ctx, "target-task", "target-session", "send now", "", false, nil, false,
		launchOriginManual, promptTaskOptions{capacityIntent: turnCapacityOverride})
	require.NoError(t, err)
	require.Equal(t, []string{"send now"}, manager.capturedPrompts)

	session, getErr := repo.GetTaskSession(ctx, "target-session")
	require.NoError(t, getErr)
	require.NotNil(t, session.Metadata[ceilingManualOverrideMetadataKey],
		"an explicit override must keep its audit record")
}

// TestMessageTurnCapacity_ConcurrentFinalSlotHasOneWinner pins the single
// controller critical section.
func TestMessageTurnCapacity_ConcurrentFinalSlotHasOneWinner(t *testing.T) {
	ctx := context.Background()
	controller := newSessionCeilingController(1, nil, nil)

	var wg sync.WaitGroup
	admitted := make(chan bool, 2)
	for _, id := range []string{"a", "b"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, refusal := controller.admitTurn(ctx, turnAdmissionRequest{
				taskID: "task-" + id, sessionID: "session-" + id,
				intent: turnCapacityBounded, seam: "concurrent",
			})
			admitted <- refusal == nil
		}()
	}
	wg.Wait()
	close(admitted)
	count := 0
	for ok := range admitted {
		if ok {
			count++
		}
	}
	require.Equal(t, 1, count, "exactly one contender may take the final slot")
}

// TestMessageTurnCapacity_OldTicketCannotReleaseSuccessor pins attempt-token
// ownership: releasing an already-consumed ticket must not free its successor.
func TestMessageTurnCapacity_OldTicketCannotReleaseSuccessor(t *testing.T) {
	ctx := context.Background()
	controller := newSessionCeilingController(1, nil, nil)
	req := turnAdmissionRequest{taskID: "task", sessionID: "session", intent: turnCapacityBounded, seam: "ownership"}
	first, refusal := controller.admitTurn(ctx, req)
	require.Nil(t, refusal)
	first.release()
	second, refusal := controller.admitTurn(ctx, req)
	require.Nil(t, refusal)
	require.NotNil(t, second)

	first.release()
	require.False(t, controller.turnCapacityAvailable(ctx),
		"an old attempt's late release must not free the successor's slot")
	second.release()
	require.True(t, controller.turnCapacityAvailable(ctx))
}

func TestMessageTurnCapacity_ReleasedOrExpiredTicketCannotAdmitRetry(t *testing.T) {
	ctx := context.Background()
	controller := newSessionCeilingController(1, nil, nil)
	svc := &Service{sessionCeiling: controller}
	session := &models.TaskSession{ID: "target-session", TaskID: "target-task", QueueIncarnationID: "inc-1"}

	ticket, refusal := controller.admitTurn(ctx, turnAdmissionRequest{
		taskID: "target-task", sessionID: "target-session", incarnationID: "inc-1",
		intent: turnCapacityBounded, seam: "retry",
	})
	require.Nil(t, refusal)
	staleCtx := withMessageTurnReservation(ctx, ticket)
	ticket.release()

	other, refusal := controller.admitTurn(ctx, turnAdmissionRequest{
		taskID: "other-task", sessionID: "other-session", incarnationID: "inc-2",
		intent: turnCapacityBounded, seam: "other",
	})
	require.Nil(t, refusal)
	require.NotNil(t, other)
	retry, err := svc.admitPromptTaskMessageTurn(staleCtx, "target-task", "target-session", session, promptTaskOptions{})
	require.Nil(t, retry, "a released context ticket must not authorize a retry")
	require.True(t, IsMessageTurnCapacityRefusal(err),
		"the stale handle must reacquire bounded capacity and fail while the slot is occupied")

	other.release()
	expired, refusal := controller.admitTurn(ctx, turnAdmissionRequest{
		taskID: "expired-task", sessionID: "expired-session", incarnationID: "inc-3",
		intent: turnCapacityBounded, seam: "expired",
	})
	require.Nil(t, refusal)
	expired.takenAt = time.Now().Add(-24 * time.Hour)
	controller.expireStaleReservations()
	expiredCtx := withMessageTurnReservation(ctx, expired)
	replacement, refusal := controller.admitTurn(ctx, turnAdmissionRequest{
		taskID: "replacement-task", sessionID: "replacement-session", incarnationID: "inc-4",
		intent: turnCapacityBounded, seam: "replacement",
	})
	require.Nil(t, refusal)
	require.NotNil(t, replacement)
	// The expired handle is no longer owned; a fresh bounded admission still
	// sees the occupied slot.
	retry, err = svc.admitPromptTaskMessageTurn(expiredCtx, "expired-task", "expired-session", session, promptTaskOptions{})
	require.Nil(t, retry)
	require.True(t, IsMessageTurnCapacityRefusal(err))
}

func TestMessageTurnCapacity_ReplayPromptEnsureStaysDeferredAtCapacity(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "replay-task", "replay-session", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "replay-task")
	require.NoError(t, err)
	filler, refusal := svc.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: "filler-task", sessionID: "filler-session",
		intent: turnCapacityBounded, seam: "filler",
	})
	require.Nil(t, refusal)
	defer filler.release()

	result := svc.replayCeilingLaunchPromptEnsure(ctx, task, map[string]interface{}{
		metaKeySessionID:      "replay-session",
		metaKeyPrompt:         "replayed prompt",
		sessionModelConfigKey: "",
		metaKeyPlanMode:       false,
	})
	require.Equal(t, ceilingReplayStillDeferred, result.outcome)
	require.Empty(t, manager.capturedPrompts, "a replay at full capacity must not reach the provider")
	record := deferredLaunchOf(t, svc, "replay-task")
	require.NotNil(t, record)
	require.Equal(t, string(models.CeilingLaunchPromptEnsure), record[models.CeilingLaunchKindKey])
}

func TestCIAutomationIdlePromptQueuesAtCapacity(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "ci-task", "ci-session", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "ci-session")
	require.NoError(t, err)
	svc.messageCreator = &mockMessageCreator{}
	filler, refusal := svc.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: "filler-task", sessionID: "filler-session",
		intent: turnCapacityBounded, seam: "filler",
	})
	require.Nil(t, refusal)
	defer filler.release()

	pr := &github.TaskPR{TaskID: "ci-task", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}
	result, err := svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", true)
	require.NoError(t, err)
	require.Equal(t, ciAutomationDispatchQueuedInsert, result.kind)
	require.Empty(t, manager.capturedPrompts, "a fresh idle CI round must not dispatch at full capacity")
	status := svc.messageQueue.GetStatus(ctx, "ci-session")
	require.Equal(t, 1, status.Count)
	require.Equal(t, true, status.Entries[0].Metadata[metaKeyUserMessageRecorded],
		"the deferred CI prompt must reuse its recorded transcript")
}

func TestQueuedMessageCapacityOverrideContinuesExplicitAction(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	manager.promptDone = make(chan struct{})
	seedTaskAndSession(t, repo, "clarify-task", "clarify-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "clarify-session", "clarify-task", "clarify-exec")
	markTargetPromptReady(t, svc, "clarify-task", "clarify-session", "clarify-exec")
	filler, refusal := svc.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: "filler-task", sessionID: "filler-session",
		intent: turnCapacityBounded, seam: "filler",
	})
	require.Nil(t, refusal)
	defer filler.release()

	_, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "clarify-session", "clarify-task", "clarification answer", "",
		messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{metaKeyUserMessageRecorded: true, metaKeyCapacityOverride: true},
	)
	require.NoError(t, err)
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, "clarify-session"))
	select {
	case <-manager.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("trusted queued override must dispatch at full capacity")
	}
	manager.mu.Lock()
	require.Equal(t, []string{"clarification answer"}, manager.capturedPrompts)
	manager.mu.Unlock()
}
