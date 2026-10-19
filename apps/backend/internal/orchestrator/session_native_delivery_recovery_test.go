package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
func TestRecoverSessionUnknownPromptKeepsNativeHistory(t *testing.T) {
	for _, scenario := range []struct {
		name                            string
		reason                          string
		failLaunch, failAcknowledgement bool
	}{
		{"resume", "unknown_prompt_outcome", false, false},
		{"launch failure", "unknown_prompt_outcome", true, false},
		{"acknowledgement failure", "unknown_prompt_outcome", false, true},
		{"retained journal recovery", durableDeliveryUnresolvedReason, false, false},
		{"retained journal failure", durableDeliveryUnresolvedReason, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, launched, started := nativeResumeFixture(t, scenario.reason, scenario.failLaunch, scenario.failAcknowledgement)
			sessionID, taskID := submission.SessionID, "task1"
			repo := service.repo.(nativeResumeTestRepository)
			response, err := service.RecoverSession(ctx, taskID, sessionID, "resume")
			if scenario.failLaunch || scenario.failAcknowledgement {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, response.Success)
				require.True(t, *started)
			}
			require.NotNil(t, *launched)
			require.Equal(t, "native-conversation", (*launched).ACPSessionID)
			require.Empty(t, manager.capturedPromptCalls, "resume must not replay the interrupted prompt")
			stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
			require.Equal(t, submission.Outcome, stored.Outcome)
			currentSession, err := service.repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			delivery, exists := models.LoadAgentDeliveryRecovery(currentSession.Metadata)
			require.True(t, exists)
			require.Equal(t, submission.ID, delivery.SubmissionID)
			require.Equal(t, block.DeliveryStreamID, delivery.StreamID)
			recovery, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			if scenario.failLaunch || scenario.failAcknowledgement {
				require.Equal(t, models.RecoveryBlockOpen, recovery.State)
				require.Equal(t, models.AgentDeliveryRecoveryUncertain, delivery.Phase)
			} else {
				require.Equal(t, models.RecoveryBlockResolved, recovery.State)
				require.Equal(t, "resume", recovery.AuthorizedAction)
				require.Equal(t, models.AgentDeliveryRecoveryRestored, delivery.Phase)
				require.NotNil(t, recovery.ResolvedAt)
				require.True(t, recovery.ResolvedAt.Equal(delivery.UpdatedAt))
			}
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
func TestNativeResumeStartFailureLeavesUnknownDeliveryUnacknowledged(t *testing.T) {
	service, manager, block, submission, launched, _ := nativeResumeFixture(
		t, "unknown_prompt_outcome", false, false,
	)
	startupErr := errors.New("native agent did not become ready")
	startupFinished := make(chan struct{})
	var acknowledgeCalls atomic.Int32
	originalAcknowledge := manager.acknowledge
	manager.acknowledge = func(ctx context.Context, sessionID string) error {
		acknowledgeCalls.Add(1)
		return originalAcknowledge(ctx, sessionID)
	}
	manager.startAgentProcess = func(ctx context.Context, executionID string) error {
		defer close(startupFinished)
		repo := service.repo.(nativeResumeTestRepository)
		running, err := repo.GetExecutorRunningBySessionID(ctx, submission.SessionID)
		require.NoError(t, err)
		running.AgentExecutionID = executionID
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
		beforeStop, err := service.repo.GetTaskSession(ctx, submission.SessionID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateStarting, beforeStop.State)
		attemptID := executor.ResumeAttemptIDFromContext(ctx)
		require.NotEmpty(t, attemptID, "native startup must carry the active resume attempt identity")
		service.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "task1",
			SessionID:        submission.SessionID,
			AgentExecutionID: executionID,
			AttemptID:        attemptID,
		})
		stopped, err := service.repo.GetTaskSession(ctx, submission.SessionID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateStarting, stopped.State,
			"the current native startup waiter owns failure classification")
		return startupErr
	}

	response, err := service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
	select {
	case <-startupFinished:
	case <-time.After(5 * time.Second):
		t.Fatal("native resume did not attempt to start the agent")
	}
	require.ErrorIs(t, err, startupErr)
	require.Nil(t, response)
	require.Zero(t, acknowledgeCalls.Load(), "native history must not be acknowledged after a failed start")
	failedSession, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, failedSession.State)
	require.NotNil(t, *launched)
	require.Equal(t, "native-conversation", (*launched).ACPSessionID)

	repo := service.repo.(nativeResumeTestRepository)
	stored, err := repo.GetAgentDeliverySubmission(context.Background(), submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, submission.Payload, stored.Payload)
	recovery, err := repo.GetSessionRecoveryBlock(context.Background(), block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, recovery.State)
}

func TestAgentStoppedAfterNativeResumeStartFailureDoesNotCompleteTurn(t *testing.T) {
	service, manager, block, submission, launched, _ := nativeResumeFixture(
		t, "unknown_prompt_outcome", false, false,
	)
	repo := service.repo.(nativeResumeTestRepository)
	var attemptID string
	var executionID string
	var acknowledgeCalls atomic.Int32
	originalAcknowledge := manager.acknowledge
	manager.acknowledge = func(ctx context.Context, sessionID string) error {
		acknowledgeCalls.Add(1)
		return originalAcknowledge(ctx, sessionID)
	}
	turnRepo := service.repo.(*sqliterepo.Repository)
	turns := &nativeResumeTurnServiceSpy{TurnService: &repoTurnService{repo: turnRepo}}
	service.turnService = turns
	manager.startAgentProcess = func(ctx context.Context, currentExecutionID string) error {
		executionID = currentExecutionID
		attemptID = executor.ResumeAttemptIDFromContext(ctx)
		running, err := repo.GetExecutorRunningBySessionID(ctx, submission.SessionID)
		require.NoError(t, err)
		running.AgentExecutionID = currentExecutionID
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
		now := time.Now().UTC()
		require.NoError(t, turnRepo.CreateTurn(ctx, &models.Turn{
			ID: "turn-native-resume-startup-failure", TaskID: "task1",
			TaskSessionID: submission.SessionID, StartedAt: now,
		}))
		return errors.New("native agent did not become ready")
	}

	response, err := service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
	require.Error(t, err)
	require.Nil(t, response)
	require.NotEmpty(t, attemptID)
	require.NotEmpty(t, executionID)
	require.False(t, service.activeResumeAttemptOwnsStartup(submission.SessionID, executionID, attemptID),
		"the failed attempt must have left the active registry")
	require.True(t, service.resumeAttemptAllowsExecution(submission.SessionID, executionID, attemptID),
		"the exact failed attempt remains in the callback tombstone")
	attemptNumber, _ := parseResumeAttemptIdentity(attemptID)
	registry := service.resumeAttemptStore()
	registry.mu.Lock()
	current := registry.attempts[submission.SessionID]
	tombstone, tombstoneFound := registry.tombstoneLocked(submission.SessionID, attemptNumber)
	registry.mu.Unlock()
	require.Nil(t, current, "the failed attempt must have left the active registry")
	require.True(t, tombstoneFound)
	require.Equal(t, executionID, tombstone.executionID,
		"the allocated execution identity must survive the startup error")
	require.False(t, tombstone.accepted)
	require.False(t, tombstone.cancelled)
	require.True(t, service.resumeAttemptOwnsUnacceptedStartupStop(submission.SessionID, executionID, attemptID),
		"the exact latest failed startup remains an unaccepted stop owner")

	activeTurn, err := turns.TurnService.GetActiveTurn(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn, "the failed startup's unresolved turn must remain open")
	require.Equal(t, "turn-native-resume-startup-failure", activeTurn.ID)
	require.Nil(t, activeTurn.CompletedAt)
	failedSession, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, failedSession.State)
	turns.completed = 0
	service.handleAgentStopped(context.Background(), watcher.AgentEventData{
		TaskID: "task1", SessionID: submission.SessionID,
		AgentExecutionID: executionID, AttemptID: attemptID,
	})
	require.Zero(t, turns.completed, "a delayed stop from failed native startup must not complete its unresolved turn")
	require.Zero(t, acknowledgeCalls.Load(), "native history must not be acknowledged after a failed start")
	activeTurn, err = turns.TurnService.GetActiveTurn(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn, "delayed startup cleanup must leave the unresolved turn open")
	require.Equal(t, "turn-native-resume-startup-failure", activeTurn.ID)
	require.Nil(t, activeTurn.CompletedAt)

	failedSession, err = service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, failedSession.State)
	require.NotNil(t, *launched)
	require.Equal(t, "native-conversation", (*launched).ACPSessionID)
	stored, err := repo.GetAgentDeliverySubmission(context.Background(), submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, submission.Payload, stored.Payload)
	recovery, err := repo.GetSessionRecoveryBlock(context.Background(), block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, recovery.State)
}

func TestAgentStoppedOnlyDefersForActiveNativeStartup(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		state     models.TaskSessionState
		attemptID bool
		accepted  bool
		finished  bool
		wantState models.TaskSessionState
	}{
		{name: "active attempt while starting", state: models.TaskSessionStateStarting, attemptID: true, wantState: models.TaskSessionStateStarting},
		{name: "active attempt after running", state: models.TaskSessionStateRunning, attemptID: true, wantState: models.TaskSessionStateCancelled},
		{name: "accepted attempt while starting", state: models.TaskSessionStateStarting, attemptID: true, accepted: true, wantState: models.TaskSessionStateCancelled},
		{name: "finished accepted attempt while starting", state: models.TaskSessionStateStarting, attemptID: true, accepted: true, finished: true, wantState: models.TaskSessionStateCancelled},
		{name: "untagged stop while starting", state: models.TaskSessionStateStarting, wantState: models.TaskSessionStateCancelled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, _, _, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			setNativeResumeSessionState(t, service, submission.SessionID, scenario.state)
			var attemptID string
			if scenario.attemptID {
				attempt := beginNativeResumeTestAttempt(t, service, submission.SessionID, "old-execution")
				attemptID = attempt.identity()
				if scenario.accepted {
					require.True(t, service.resumeAttemptStore().accept(attempt, "old-execution"))
				}
				if scenario.finished {
					attempt.finish(service.resumeAttemptStore())
				}
			}

			service.handleAgentStopped(context.Background(), watcher.AgentEventData{
				TaskID: "task1", SessionID: submission.SessionID,
				AgentExecutionID: "old-execution", AttemptID: attemptID,
			})

			stored, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
			require.NoError(t, err)
			require.Equal(t, scenario.wantState, stored.State)
		})
	}
}

type nativeResumeTurnServiceSpy struct {
	TurnService
	completed int
}

func (spy *nativeResumeTurnServiceSpy) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return spy.TurnService.GetActiveTurn(ctx, sessionID)
}

func (spy *nativeResumeTurnServiceSpy) CompleteTurn(ctx context.Context, turnID string) error {
	spy.completed++
	return spy.TurnService.CompleteTurn(ctx, turnID)
}

func TestAgentStoppedFromSupersededResumeAttemptCannotCancelSuccessor(t *testing.T) {
	service, _, _, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	setNativeResumeSessionState(t, service, submission.SessionID, models.TaskSessionStateStarting)
	registry := service.resumeAttemptStore()
	previous, owner := registry.begin(context.Background(), "task1", submission.SessionID)
	require.True(t, owner)
	previous.setExecutionID("old-execution")
	require.True(t, registry.invalidate(submission.SessionID))
	current, owner := registry.begin(context.Background(), "task1", submission.SessionID)
	require.True(t, owner)
	current.setExecutionID("new-execution")
	setNativeResumeExecution(t, service, submission.SessionID, "new-execution")

	service.handleAgentStopped(context.Background(), watcher.AgentEventData{
		TaskID: "task1", SessionID: submission.SessionID,
		AgentExecutionID: "old-execution", AttemptID: previous.identity(),
	})

	stored, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, stored.State)
}

func TestAgentStoppedFromOldExecutionCannotCancelCurrentResumeAttempt(t *testing.T) {
	service, _, _, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	setNativeResumeSessionState(t, service, submission.SessionID, models.TaskSessionStateStarting)
	current := beginNativeResumeTestAttempt(t, service, submission.SessionID, "new-execution")
	setNativeResumeExecution(t, service, submission.SessionID, "new-execution")

	service.handleAgentStopped(context.Background(), watcher.AgentEventData{
		TaskID: "task1", SessionID: submission.SessionID,
		AgentExecutionID: "old-execution", AttemptID: current.identity(),
	})

	stored, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, stored.State)
}

func TestAgentStoppedFromCancelledResumeAttemptCannotOverrideExplicitCancel(t *testing.T) {
	service, _, _, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	attempt := beginNativeResumeTestAttempt(t, service, submission.SessionID, "old-execution")
	require.True(t, service.resumeAttemptStore().invalidate(submission.SessionID))
	setNativeResumeSessionState(t, service, submission.SessionID, models.TaskSessionStateCancelled)

	service.handleAgentStopped(context.Background(), watcher.AgentEventData{
		TaskID: "task1", SessionID: submission.SessionID,
		AgentExecutionID: "old-execution", AttemptID: attempt.identity(),
	})

	stored, err := service.repo.GetTaskSession(context.Background(), submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
}

func TestResumeAttemptTombstoneRejectsOlderAttemptWithReusedExecution(t *testing.T) {
	const sessionID = "session-latest-resume-tombstone"
	service := &Service{resumeAttempts: newResumeAttemptRegistry()}
	registry := service.resumeAttemptStore()
	older, owner := registry.begin(context.Background(), "task1", sessionID)
	require.True(t, owner)
	older.setExecutionID("execution-reused")
	olderID := older.identity()
	older.finish(registry)

	latest, owner := registry.begin(context.Background(), "task1", sessionID)
	require.True(t, owner)
	latest.setExecutionID("execution-reused")
	latestID := latest.identity()
	latest.finish(registry)

	require.False(t, service.resumeAttemptAllowsExecution(sessionID, "execution-reused", olderID),
		"a later attempt supersedes an older tag even when the runtime reuses its execution ID")
	require.True(t, service.resumeAttemptAllowsExecution(sessionID, "execution-reused", latestID))
	require.False(t, service.resumeAttemptOwnsUnacceptedStartupStop(sessionID, "execution-reused", olderID))
	require.True(t, service.resumeAttemptOwnsUnacceptedStartupStop(sessionID, "execution-reused", latestID))
}

func beginNativeResumeTestAttempt(t *testing.T, service *Service, sessionID, executionID string) *resumeAttempt {
	t.Helper()
	attempt, owner := service.resumeAttemptStore().begin(context.Background(), "task1", sessionID)
	require.True(t, owner)
	attempt.setExecutionID(executionID)
	return attempt
}

func setNativeResumeSessionState(t *testing.T, service *Service, sessionID string, state models.TaskSessionState) {
	t.Helper()
	repo := service.repo.(interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
		UpdateTaskSession(context.Context, *models.TaskSession) error
	})
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	session.State = state
	require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
}

func setNativeResumeExecution(t *testing.T, service *Service, sessionID, executionID string) {
	t.Helper()
	repo := service.repo.(nativeResumeTestRepository)
	running, err := repo.GetExecutorRunningBySessionID(context.Background(), sessionID)
	require.NoError(t, err)
	running.AgentExecutionID = executionID
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), running))
}

// The optional capability is present on the production lifecycle adapter.
type nativeResumeTestManager struct {
	*sessionUpdatingAgentManager
	acknowledge       func(context.Context, string) error
	startAgentProcess func(context.Context, string) error
}

func (m *nativeResumeTestManager) StartAgentProcess(ctx context.Context, executionID string) error {
	if m.startAgentProcess != nil {
		return m.startAgentProcess(ctx, executionID)
	}
	return m.sessionUpdatingAgentManager.StartAgentProcess(ctx, executionID)
}

func (m *nativeResumeTestManager) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	return m.acknowledge(ctx, sessionID)
}

type nativeResumeTestRepository interface {
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
	GetSessionRecoveryBlock(context.Context, string) (*models.SessionRecoveryBlock, error)
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
	PrepareAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission) (bool, error)
}

func nativeResumeFixture(t *testing.T, reason string, failLaunch, failAcknowledgement bool) (*Service, *nativeResumeTestManager, *models.SessionRecoveryBlock, *models.AgentDeliverySubmission, **executor.LaunchAgentRequest, *bool) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.AgentProfileID = "profile1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running1", SessionID: session.ID, TaskID: session.TaskID,
		AgentExecutionID: "old-execution", ResumeToken: "native-conversation", Resumable: true,
		CreatedAt: now, UpdatedAt: now,
	}))
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:interrupted", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		PayloadHash: "original-hash", Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
		HarnessGeneration: 1, OwnerGeneration: 1, Outcome: "prompt_dispatch_failed",
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, submission)
	require.NoError(t, err)
	block := &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		Reason: reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submission.ID,
		ExpectedGeneration: submission.HarnessGeneration, DeliveryStreamID: "native-resume-stream",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	seedNativeResumeDeliveryMetadata(t, repo, session, block, submission, now)
	var launched *executor.LaunchAgentRequest
	started := false
	manager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			repoForExecutionLookup: repo,
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = req
				if failLaunch {
					return nil, errors.New("runtime unavailable")
				}
				return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution", Status: v1.AgentStatusStarting}, nil
			},
		},
		repo: repo, sessionID: session.ID, taskID: session.TaskID, onStartCalled: &started,
	}
	resumer := &nativeResumeTestManager{sessionUpdatingAgentManager: manager, acknowledge: func(ctx context.Context, sessionID string) error {
		open, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, open.State, "journal must be acknowledged before SQL resolution")
		if failAcknowledgement {
			return errors.New("journal unavailable")
		}
		return nil
	}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), resumer)
	service.executor = executor.NewExecutor(resumer, repo, testLogger(), executor.ExecutorConfig{})
	return service, resumer, block, submission, &launched, &started
}

func seedNativeResumeDeliveryMetadata(t *testing.T, repo *sqliterepo.Repository, session *models.TaskSession, block *models.SessionRecoveryBlock, submission *models.AgentDeliverySubmission, now time.Time) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: submission.HarnessGeneration,
		NativeSessionID: "native-conversation", AgentType: "codex", CreationReason: "initial",
		CreatedAt: now, CommittedAt: now,
	}))
	recovery := &models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1, PromptGeneration: 1,
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		AgentExecutionID: "old-execution", SubmissionID: submission.ID,
		StreamID: block.DeliveryStreamID, HarnessGeneration: submission.HarnessGeneration, UpdatedAt: now,
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = recovery
	encoded, err := json.Marshal(session.Metadata)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = ? WHERE id = ?`, string(encoded), session.ID)
	require.NoError(t, err)
}

func TestNativeResumeRefusesUnresolvedLiveBackendWork(t *testing.T) {
	for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching} {
		t.Run(string(state), func(t *testing.T) {
			service, _, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
			repo := service.repo.(nativeResumeTestRepository)
			live := *submission
			live.ID, live.State = "prompt:live", state
			_, err := repo.PrepareAgentDeliverySubmission(context.Background(), &live)
			require.NoError(t, err)
			_, err = service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
			var blocked *sessionRecoveryRequiredError
			require.ErrorAs(t, err, &blocked)
			require.Nil(t, *launched)
			stored, err := repo.GetSessionRecoveryBlock(context.Background(), block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, stored.State)
		})
	}
}

func TestNativeResumeKeepsOfficeDurableRecoveryWithScheduler(t *testing.T) {
	ctx := context.Background()
	service, _, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	task, err := service.repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, service.repo.UpdateTask(ctx, task))
	_, err = service.RecoverSession(ctx, task.ID, submission.SessionID, "resume")
	var blocked *sessionRecoveryRequiredError
	require.ErrorAs(t, err, &blocked)
	require.Nil(t, *launched)
	stored, err := service.repo.(nativeResumeTestRepository).GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, stored.State)
}
