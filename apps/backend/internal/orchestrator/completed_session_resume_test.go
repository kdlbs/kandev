package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestCompletedSessionResumePreservesConversation covers the explicit recovery
// path for a completed session. Recovery must relaunch the existing provider
// conversation in place instead of creating a sibling or silently losing the
// provider resume identity.
func TestCompletedSessionResumePreservesConversation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()

	var captured *executor.LaunchAgentRequest
	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				captured = req
				return &executor.LaunchAgentResponse{
					AgentExecutionID: "exec-completed-follow-up",
					Status:           v1.AgentStatusStarting,
				}, nil
			},
		},
		repo:          repo,
		sessionID:     "session-completed-follow-up",
		taskID:        "task-completed-follow-up",
		onStartCalled: &started,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(
		t,
		repo,
		"task-completed-follow-up",
		"session-completed-follow-up",
		models.TaskSessionStateCompleted,
	)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID:        "executor-completed-follow-up",
		Name:      "Worktree",
		Type:      models.ExecutorTypeWorktree,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID:            "repo-completed-follow-up",
		WorkspaceID:   "ws1",
		Name:          "backend",
		SourceType:    "local",
		LocalPath:     t.TempDir(),
		DefaultBranch: "main",
		CreatedAt:     now,
		UpdatedAt:     now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           "task-repo-completed-follow-up",
		TaskID:       "task-completed-follow-up",
		RepositoryID: "repo-completed-follow-up",
		BaseBranch:   "main",
		Position:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))

	session, err := repo.GetTaskSession(ctx, "session-completed-follow-up")
	require.NoError(t, err)
	session.AgentProfileID = "profile-completed-follow-up"
	session.ExecutorID = "executor-completed-follow-up"
	session.RepositoryID = "repo-completed-follow-up"
	session.BaseBranch = "main"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "running-completed-follow-up",
		SessionID:        "session-completed-follow-up",
		TaskID:           "task-completed-follow-up",
		AgentExecutionID: "exec-before-completed-follow-up",
		ResumeToken:      "acp-session-completed-follow-up",
		Resumable:        true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	response, err := svc.RecoverSession(
		ctx,
		"task-completed-follow-up",
		"session-completed-follow-up",
		"resume",
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "task-completed-follow-up", response.TaskID)
	require.Equal(t, "session-completed-follow-up", response.SessionID)
	require.NotNil(t, captured)
	require.Equal(t, "session-completed-follow-up", captured.SessionID)
	require.Equal(t, "acp-session-completed-follow-up", captured.ACPSessionID)
	require.True(t, started)

	reloaded, err := repo.GetTaskSession(ctx, "session-completed-follow-up")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, reloaded.State)
	require.Equal(t, session.ID, reloaded.ID)
	require.NotNil(t, reloaded.Metadata)
	require.Equal(t, true, reloaded.Metadata["completion_follow_up"])

	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-completed-follow-up")
	require.NoError(t, err)
	require.Equal(t, "acp-session-completed-follow-up", running.ResumeToken)
	sessions, err := repo.ListTaskSessions(ctx, "task-completed-follow-up")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

func TestCompletedSessionFollowUpOwnership(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-completed-ownership", "session-follow-up", models.TaskSessionStateCompleted)

	followUp, err := repo.GetTaskSession(ctx, "session-follow-up")
	require.NoError(t, err)
	followUp.AgentProfileID = "profile-ownership"
	followUp.UpdatedAt = time.Now().UTC().Add(2 * time.Minute)
	require.NoError(t, repo.UpdateTaskSession(ctx, followUp))
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx,
		"session-follow-up",
		models.SessionMetaKeyCompletionFollowUp,
		true,
	))

	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "session-reusable",
		TaskID:         "task-completed-ownership",
		AgentProfileID: "profile-ownership",
		State:          models.TaskSessionStateWaitingForInput,
		StartedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC().Add(time.Minute),
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	reusable, err := svc.findReusableSessionForProfile(
		ctx,
		"task-completed-ownership",
		"profile-ownership",
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, reusable)
	require.Equal(t, "session-reusable", reusable.ID)
}

func TestCompletedSessionFollowUpTurnReturnsToWaiting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-completed-ready", "session-completed-ready", models.TaskSessionStateRunning)
	require.NoError(t, repo.UpdateTaskState(ctx, "task-completed-ready", v1.TaskStateCompleted))

	session, err := repo.GetTaskSession(ctx, "session-completed-ready")
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx,
		"session-completed-ready",
		models.SessionMetaKeyCompletionFollowUp,
		true,
	))
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	storedSession, err := repo.GetTaskSession(ctx, "session-completed-ready")
	require.NoError(t, err)
	require.True(t, models.IsCompletionFollowUpSession(storedSession.Metadata))

	svc := createTestServiceWithAgent(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		&mockAgentManager{isAgentRunning: true},
	)

	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID:    "task-completed-ready",
		SessionID: "session-completed-ready",
	})

	reloaded, err := repo.GetTaskSession(ctx, "session-completed-ready")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, reloaded.State)
	require.True(t, models.IsCompletionFollowUpSession(reloaded.Metadata))
	task, err := repo.GetTask(ctx, "task-completed-ready")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, task.State)
}

func TestCompletedSessionResumeRaces(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	startEntered := make(chan struct{})
	startRelease := make(chan struct{})
	startFinished := make(chan struct{})
	var startOnce sync.Once
	var launchCalls atomic.Int32
	started := false
	agentMgr := &blockingCompletedResumeAgentManager{
		sessionUpdatingAgentManager: &sessionUpdatingAgentManager{
			mockAgentManager: &mockAgentManager{
				isAgentRunning: true,
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					launchCalls.Add(1)
					return &executor.LaunchAgentResponse{
						AgentExecutionID: "exec-completed-race",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
			},
			repo:          repo,
			sessionID:     "session-completed-race",
			taskID:        "task-completed-race",
			onStartCalled: &started,
		},
		startEntered:  startEntered,
		startRelease:  startRelease,
		startFinished: startFinished,
		startOnce:     &startOnce,
		finishOnce:    &sync.Once{},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(
		t,
		repo,
		"task-completed-race",
		"session-completed-race",
		models.TaskSessionStateCompleted,
	)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID:        "executor-completed-race",
		Name:      "Worktree",
		Type:      models.ExecutorTypeWorktree,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID:            "repo-completed-race",
		WorkspaceID:   "ws1",
		Name:          "backend",
		SourceType:    "local",
		LocalPath:     t.TempDir(),
		DefaultBranch: "main",
		CreatedAt:     now,
		UpdatedAt:     now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           "task-repo-completed-race",
		TaskID:       "task-completed-race",
		RepositoryID: "repo-completed-race",
		BaseBranch:   "main",
		Position:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	session, err := repo.GetTaskSession(ctx, "session-completed-race")
	require.NoError(t, err)
	session.AgentProfileID = "profile-completed-race"
	session.ExecutorID = "executor-completed-race"
	session.RepositoryID = "repo-completed-race"
	session.BaseBranch = "main"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "running-completed-race",
		SessionID:        "session-completed-race",
		TaskID:           "task-completed-race",
		AgentExecutionID: "exec-before-completed-race",
		ResumeToken:      "acp-session-completed-race",
		Resumable:        true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	type recoveryResult struct {
		response *LaunchSessionResponse
		err      error
	}
	firstResult := make(chan recoveryResult, 1)
	go func() {
		response, recoverErr := svc.RecoverSession(ctx, "task-completed-race", "session-completed-race", "resume")
		firstResult <- recoveryResult{response: response, err: recoverErr}
	}()
	select {
	case <-startEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first completed-session resume")
	}

	secondResult := make(chan recoveryResult, 1)
	go func() {
		response, recoverErr := svc.RecoverSession(ctx, "task-completed-race", "session-completed-race", "resume")
		secondResult <- recoveryResult{response: response, err: recoverErr}
	}()
	close(startRelease)
	first := <-firstResult
	require.NoError(t, first.err)
	require.NotNil(t, first.response)
	select {
	case result := <-secondResult:
		if result.err == nil {
			require.NotNil(t, result.response)
			require.Equal(t, "session-completed-race", result.response.SessionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the duplicate completed-session resume")
	}

	select {
	case <-startFinished:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the resumed agent process")
	}
	require.Equal(t, int32(1), launchCalls.Load())
	require.True(t, started)

	reloaded, err := repo.GetTaskSession(ctx, "session-completed-race")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, reloaded.State)
	sessions, err := repo.ListTaskSessions(ctx, "task-completed-race")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

type blockingCompletedResumeAgentManager struct {
	*sessionUpdatingAgentManager
	startEntered  chan struct{}
	startRelease  <-chan struct{}
	startFinished chan struct{}
	startOnce     *sync.Once
	finishOnce    *sync.Once
}

func (m *blockingCompletedResumeAgentManager) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	m.startOnce.Do(func() { close(m.startEntered) })
	<-m.startRelease
	err := m.sessionUpdatingAgentManager.StartAgentProcess(ctx, agentExecutionID)
	m.finishOnce.Do(func() { close(m.startFinished) })
	return err
}
