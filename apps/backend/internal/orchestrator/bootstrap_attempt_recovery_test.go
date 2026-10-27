package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.2
func TestBootstrapAttemptAdmissionPreservesExecutorLookupErrors(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
		models.TaskSessionStateWaitingForInput,
	} {
		t.Run(string(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", state)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			lookupErr := errors.New("executor inventory unavailable")
			svc.repo = &executorLookupErrorRepo{Repository: repo, err: lookupErr}

			candidate, err := svc.loadResumeSessionCandidate(context.Background(), "task1", "session1", executor.ResumeOptions{}, nil)
			require.ErrorIs(t, err, lookupErr)
			require.Nil(t, candidate)
		})
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.1
func TestBootstrapAttemptFailedWaitingSessionRequiresRecovery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session1", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "The agent could not start.", Phase: models.LaunchErrorPhaseBootstrap,
		AgentExecutionID: "failed-execution", ExecutionID: "failed-execution",
		AttemptID: "failed-attempt", StampValue: "failure-stamp", OccurredAt: time.Now().UTC(),
		Causes: []models.AgentErrorCause{{Operation: models.AgentErrorCauseOperationStart, Code: models.AgentErrorCauseCodeAuthenticationRequired}},
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})
	_, err := svc.PromptTask(ctx, "task1", "session1", "hello", "", false, nil, false)
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.NotErrorIs(t, err, ErrSessionRuntimeUnavailable)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	lastError, found := models.LoadLastAgentError(session.Metadata)
	require.True(t, found)
	require.Equal(t, "failure-stamp", lastError.Stamp())
}

func TestBootstrapAttemptAdmissionRetainsTerminalMissingRowRecovery(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateFailed, models.TaskSessionStateCancelled} {
		t.Run(string(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", state)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			candidate, err := svc.loadResumeSessionCandidate(context.Background(), "task1", "session1", executor.ResumeOptions{}, nil)
			require.NoError(t, err)
			require.NotNil(t, candidate)
			require.Nil(t, candidate.running)
		})
	}
}

func TestBootstrapAttemptOfficeRecoveryRequiresSchedulerBlock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, repo.UpdateTask(ctx, task))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session1", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "The agent could not start.", Phase: models.LaunchErrorPhaseBootstrap,
		ExecutionID: "failed-execution", AttemptID: "failed-attempt", StampValue: "failure-stamp",
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	response, err := svc.RecoverSession(ctx, "task1", "session1", "resume", "failure-stamp")
	require.ErrorIs(t, err, errOfficeTaskResumeRequiresScheduler)
	require.Nil(t, response)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.2
func TestBootstrapAttemptExplicitRecoveryAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, stamp, mutation                      string
		missingSnapshot, newerExecution, wantError bool
	}{
		{name: "matching failure", stamp: "failure-stamp"},
		{name: "missing stamp", wantError: true},
		{name: "stale stamp", stamp: "old-stamp", wantError: true},
		{name: "missing snapshot", stamp: "failure-stamp", missingSnapshot: true, wantError: true},
		{name: "newer execution", stamp: "failure-stamp", newerExecution: true, wantError: true},
		{name: "changed error", stamp: "failure-stamp", mutation: "error", wantError: true},
		{name: "cancelled", stamp: "failure-stamp", mutation: "cancel", wantError: true},
		{name: "changed generation", stamp: "failure-stamp", mutation: "generation", wantError: true},
		{name: "changed executor", stamp: "failure-stamp", mutation: "executor", wantError: true},
		{name: "deleted executor", stamp: "failure-stamp", mutation: "deleted-executor", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
			require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: "executor1", Name: "Local", Type: models.ExecutorTypeLocal}))
			session, err := repo.GetTaskSession(ctx, "session1")
			require.NoError(t, err)
			session.AgentExecutionID = "failed-execution"
			session.AgentProfileID = "profile1"
			session.ExecutorID = "executor1"
			session.AgentProfileSnapshot = map[string]interface{}{"id": "profile1", "agent_id": "agent1"}
			session.ExecutorSnapshot = map[string]interface{}{"id": "executor1", "executor_type": "local"}
			if tc.missingSnapshot {
				session.AgentProfileSnapshot = nil
			}
			session.Metadata = map[string]interface{}{models.SessionMetaKeyLastAgentError: models.LastAgentError{
				Message: "The agent could not start.", Phase: models.LaunchErrorPhaseBootstrap,
				AgentExecutionID: "failed-execution", ExecutionID: "failed-execution",
				AttemptID: "failed-attempt", StampValue: "failure-stamp", OccurredAt: time.Now().UTC(),
			}}
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError, session.Metadata[models.SessionMetaKeyLastAgentError]))
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			_, recoveryCtx, err := svc.prepareSessionRecoveryLaunchContext(ctx, session, recoveryActionResume, tc.stamp)
			if err == nil {
				switch tc.mutation {
				case "error":
					failure, _ := models.LoadLastAgentError(session.Metadata)
					failure.StampValue = "successor-failure"
					require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError, failure))
				case "cancel":
					session.State = models.TaskSessionStateCancelled
				case "generation":
					session.RouteGeneration++
				case "executor":
					session.ExecutorID = "executor2"
				case "deleted-executor":
					require.NoError(t, repo.DeleteExecutor(ctx, "executor1"))
				}
				require.NoError(t, repo.UpdateTaskSession(ctx, session))
				if tc.newerExecution {
					seedExecutorRunning(t, repo, "session1", "task1", "new-execution")
				}
				var candidate *resumeSessionCandidate
				candidate, err = svc.loadResumeSessionCandidate(recoveryCtx, "task1", "session1", executor.ResumeOptions{}, nil)
				if !tc.wantError {
					require.NoError(t, err)
					require.Equal(t, "session1", candidate.session.ID)
					require.Nil(t, candidate.running)
				}
			}
			if tc.wantError {
				require.ErrorIs(t, err, ErrSessionRecoveryRequired)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
func TestBootstrapAttemptQueueStaysBoundAndBlocked(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session1", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "The agent could not start.", Phase: models.LaunchErrorPhaseBootstrap,
		ExecutionID: "failed-execution", AttemptID: "failed-attempt", StampValue: "failure-stamp",
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.NoError(t, svc.QueueUserPrompt(ctx, "task1", "session1", "retained prompt", "", false, nil, nil, false))
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task1", "session1")
	require.NoError(t, err)
	for range 2 {
		dispatched, drainErr := svc.drainQueuedMessageForPromptableSessionForIdentity(ctx, identity)
		require.False(t, dispatched)
		require.ErrorIs(t, drainErr, ErrSessionRecoveryRequired)
	}
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "session1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "retained prompt", entries[0].Content)
	require.Equal(t, "session1", entries[0].SessionID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
func TestBootstrapAttemptFailureAfterQueueClaimRetainsPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	msg := &messagequeue.QueuedMessage{ID: "queue1", SessionID: "session1", TaskID: "task1", Content: "retained prompt", QueuedBy: messagequeue.QueuedByUser}
	svc.handleQueuedMessageExecutionError(ctx, "session1", msg, nil, false, false, bootstrapRecoveryRequired("session1"))
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "session1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "queue1", entries[0].ID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.1
// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
func TestBootstrapAttemptAuthenticationRecoveryKeepsSessionAndDispatchesOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.AgentProfileID = "profile1"
	session.ExecutorID = "executor1"
	session.DownstreamACPSessionID = "native-original"
	session.AgentProfileSnapshot = map[string]interface{}{"id": "profile1", "agent_id": "agent1"}
	session.ExecutorSnapshot = map[string]interface{}{"id": "executor1", "executor_type": "local"}
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: "executor1", Name: "Local", Type: models.ExecutorTypeLocal}))
	seedExecutorRunning(t, repo, "session1", "task1", "failed-execution")
	var ready atomic.Bool
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn:       func(context.Context, string) bool { return ready.Load() },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if req.SessionID != "session1" || req.ExecutorType != "local" || req.AgentProfileID != "profile1" ||
				req.ACPSessionID != "native-original" || req.ReplaceNativeConversation || req.RecoveryAction != recoveryActionResume {
				return nil, errors.New("recovery changed the launch binding")
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "recovered-runtime", SessionID: req.SessionID, TaskID: req.TaskID,
				AgentExecutionID: "recovered-execution", ExecutorID: "executor1", Status: "running",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "recovered-execution", Status: v1.AgentStatusStarting}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	bootReady := make(chan struct{})
	manager.startAgentProcessFunc = func(startCtx context.Context, _ string) error {
		before, loadErr := repo.GetTaskSession(startCtx, "session1")
		if loadErr != nil {
			return loadErr
		}
		failure, found := models.LoadLastAgentError(before.Metadata)
		if !found || failure.IsDismissed() {
			return errors.New("failure cleared before readiness")
		}
		ready.Store(true)
		svc.handleAgentBootReady(startCtx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1", AgentExecutionID: "recovered-execution",
			AttemptID: executor.ResumeAttemptIDFromContext(startCtx),
		})
		close(bootReady)
		return nil
	}
	require.True(t, svc.handleAgentStartFailed(ctx, "task1", "session1", "failed-execution", &lifecycle.BootstrapFailure{
		Operation: models.AgentErrorCauseOperationStart, Code: models.AgentErrorCauseCodeAuthenticationRequired,
	}, false))
	require.NoError(t, repo.DeleteExecutorRunningBySessionID(ctx, "session1"))
	failed, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, failed.State)
	failure, found := models.LoadLastAgentError(failed.Metadata)
	require.True(t, found)
	require.Equal(t, models.LaunchErrorPhaseBootstrap, failure.Phase)
	require.NoError(t, svc.QueueUserPrompt(ctx, "task1", "session1", "retained prompt", "", false, nil, nil, false))
	workerDone := make(chan struct{}, 2)
	svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }
	response, err := svc.RecoverSession(ctx, "task1", "session1", recoveryActionRuntimeRetry, failure.Stamp())
	require.NoError(t, err)
	require.Equal(t, "session1", response.SessionID)
	require.Equal(t, "recovered-execution", response.AgentExecutionID)
	select {
	case <-bootReady:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not reach readiness")
	}
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not dispatch queued work")
	}
	manager.mu.Lock()
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Equal(t, []string{"retained prompt"}, prompts)
	recovered, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	prior, found := models.LoadLastAgentError(recovered.Metadata)
	require.True(t, found)
	require.Equal(t, failure.Stamp(), prior.Stamp())
	require.True(t, prior.IsDismissed())
	sessions, err := repo.ListTaskSessions(ctx, "task1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

func TestBootstrapAttemptDirectResumeCannotBypassRecoveryStamp(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session1", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "The agent could not start.", Phase: models.LaunchErrorPhaseBootstrap,
		ExecutionID: "failed-execution", AttemptID: "failed-attempt", StampValue: "failure-stamp",
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	candidate, err := svc.loadResumeSessionCandidate(ctx, "task1", "session1", executor.ResumeOptions{}, nil)
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.Nil(t, candidate)
}
