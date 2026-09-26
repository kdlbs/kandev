package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCreatedSession_ActiveSessionReturnsBusy(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-active", "session-active", state)
			agentManager := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

			_, err := svc.StartCreatedSession(
				ctx, "task-active", "session-active", "profile", "follow-up", true, false, true, nil, nil,
			)

			require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
		})
	}
}

func TestStartCreatedSessionForPeerMessage_ContendedLaunchReturnsBusy(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-active", "session-active", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "session-active")
	require.NoError(t, err)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	release := svc.acquireSessionLifecycleLock(session.ID)
	defer release()

	completed := make(chan error, 1)
	go func() {
		_, startErr := svc.StartCreatedSessionForPeerMessage(
			ctx, identity, "profile", "follow-up", true, false, true, nil, nil,
		)
		completed <- startErr
	}()

	select {
	case err := <-completed:
		require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
	case <-time.After(time.Second):
		t.Fatal("peer-message admission waited for the active launch lock")
	}
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateCreated, current.State)
}

func TestBeginPeerMessageStartDoesNotWaitForCancelGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-guard-contention", "session-guard-contention", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "session-guard-contention")
	require.NoError(t, err)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	guard := svc.lockCancelInFlightGuard(session.ID)
	defer guard.release()

	completed := make(chan error, 1)
	go func() {
		admission, admissionErr := svc.BeginPeerMessageStart(ctx, identity)
		if admission != nil {
			admission.Release()
		}
		completed <- admissionErr
	}()

	select {
	case err := <-completed:
		require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
	case <-time.After(time.Second):
		t.Fatal("peer-message admission waited for a cancel-in-flight guard")
	}

	releaseLifecycle, acquired := svc.tryAcquireSessionLifecycleLock(session.ID)
	if acquired {
		releaseLifecycle()
	}
	assert.True(t, acquired, "busy admission must release the lifecycle lock it acquired")
}

func TestStartCreatedSessionForPeerMessage_ActiveRuntimeReturnsBusy(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-active", "session-active", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "session-active")
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-active", TaskID: session.TaskID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "same-execution",
	}))
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}

	_, err = svc.StartCreatedSessionForPeerMessage(
		ctx, identity, "profile", "follow-up", true, false, true, nil, nil,
	)

	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateCreated, current.State)
}

func TestHasActiveExecutionForPeerMessage_AllowsWorkspacePreparation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-active", "session-active", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "session-active")
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-preparing", TaskID: session.TaskID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusStarting,
	}))
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	active, err := svc.hasActiveExecutionForPeerMessage(ctx, session.ID)

	require.NoError(t, err)
	assert.False(t, active, "workspace preparation can still admit the first prompt")
}

func TestStartCreatedSessionForPeerMessage_RejectsReplacedSessionIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-active", "session-active", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "session-active")
	require.NoError(t, err)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: "old-incarnation",
	}

	_, err = svc.StartCreatedSessionForPeerMessage(
		ctx, identity, "profile", "follow-up", true, false, true, nil, nil,
	)

	require.ErrorIs(t, err, messagequeue.ErrSessionIdentityMismatch)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateCreated, current.State)
}

func TestNewServiceSessionStartingCallbackClassifiesRunningCASConflict(t *testing.T) {
	ctx := context.Background()
	repo := &newServiceRunningConflictRepository{Repository: setupTestRepo(t)}
	seedSession(t, repo.Repository, "task-production-cas", "session-production-cas", "step-production-cas")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-production-cas", models.TaskSessionStateWaitingForInput, ""))
	session, err := repo.GetTaskSession(ctx, "session-production-cas")
	require.NoError(t, err)
	session.ExecutorID = "executor-production-cas"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-production-cas", TaskID: session.TaskID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusPrepared, AgentExecutionID: "execution-production-cas",
	}))

	manager := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "ws1", Title: "CAS task", State: v1.TaskStateInProgress}
	taskRepo.tasks[task.ID] = task
	svc := NewService(
		DefaultServiceConfig(), nil, manager, taskRepo, repo,
		nil, nil, nil, testLogger(),
	)
	_, err = svc.executor.LaunchPreparedSession(ctx, task, session.ID, executor.LaunchOptions{
		AgentProfileID: session.AgentProfileID, ExecutorID: session.ExecutorID,
		Prompt: "initial prompt", StartAgent: true,
	})

	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning,
		"NewService must classify the CAS conflict through its installed orchestrator callback")
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateRunning, current.State,
		"the concurrent launch's state must remain the winner")
}

func TestLaunchInitialCreatePromptBusyDoesNotFailLiveSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-initial-create-busy", "session-initial-create-busy", "")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-initial-create-busy", models.TaskSessionStateRunning, ""))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-initial-create-busy", TaskID: "task-initial-create-busy",
		SessionID: "session-initial-create-busy", Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "execution-initial-create-busy",
	}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-initial-create-busy"] = &v1.Task{
		ID: "task-initial-create-busy", WorkspaceID: "ws1", Title: "Initial create",
		State: v1.TaskStateInProgress,
	}
	agentManager := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.turnService = &repoTurnService{repo: repo}
	_, err := svc.turnService.StartTurn(ctx, "session-initial-create-busy")
	require.NoError(t, err)

	_, err = svc.launchInitialCreatePrompt(ctx, &LaunchSessionRequest{
		TaskID: "task-initial-create-busy", SessionID: "session-initial-create-busy",
		Prompt: "initial creation prompt", AgentProfileID: "profile-initial-create",
	})

	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
	current, err := repo.GetTaskSession(ctx, "session-initial-create-busy")
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateRunning, current.State)
	running, err := repo.GetExecutorRunningBySessionID(ctx, current.ID)
	require.NoError(t, err)
	require.NotNil(t, running)
	assert.Equal(t, models.ExecutorRunningStatusRunning, running.Status)
	task, err := repo.GetTask(ctx, current.TaskID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateInProgress, task.State)
}

type newServiceRunningConflictRepository struct {
	*sqliterepo.Repository
	conflicted atomic.Bool
}

func (r *newServiceRunningConflictRepository) UpdateTaskSessionIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	if r.conflicted.CompareAndSwap(false, true) {
		current, err := r.GetTaskSession(ctx, session.ID)
		if err != nil {
			return false, err
		}
		current.State = models.TaskSessionStateRunning
		if err := r.UpdateTaskSession(ctx, current); err != nil {
			return false, err
		}
		return false, nil
	}
	return r.UpdateTaskSessionIfCurrentState(ctx, session, expected)
}
