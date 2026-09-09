package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type archiveRestoreProbe struct {
	*mockAgentManager
	restoreCalls atomic.Int32
}

func (p *archiveRestoreProbe) EnsureWorkspaceExecutionForSession(context.Context, string, string) error {
	p.restoreCalls.Add(1)
	return nil
}

func TestGetTaskSessionStatus_ArchivedTaskDisablesRecovery(t *testing.T) {
	for _, test := range []struct {
		name          string
		sessionState  models.TaskSessionState
		errorMessage  string
		resumeToken   string
		resumable     bool
		runningStatus string
	}{
		{
			name: "archive cancelled without runtime row", sessionState: models.TaskSessionStateCancelled,
			errorMessage: models.SessionArchiveCancelReason,
		},
		{
			name: "retained resumable runtime", sessionState: models.TaskSessionStateWaitingForInput,
			resumeToken: "acp-session", resumable: true, runningStatus: "ready",
		},
		{
			name: "ordinary cancellation", sessionState: models.TaskSessionStateCancelled,
			errorMessage: "user stopped the session",
		},
		{name: "completed session", sessionState: models.TaskSessionStateCompleted},
		{
			name: "stopping runtime", sessionState: models.TaskSessionStateStarting,
			runningStatus: "stopping",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-archive", "session-archive", test.sessionState)

			if err := repo.ArchiveTask(ctx, "task-archive"); err != nil {
				t.Fatalf("archive task: %v", err)
			}
			session, err := repo.GetTaskSession(ctx, "session-archive")
			require.NoError(t, err)
			session.ErrorMessage = test.errorMessage
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			if test.resumeToken != "" || test.runningStatus != "" {
				now := time.Now().UTC()
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
					ID: "running-archive", SessionID: session.ID, TaskID: session.TaskID,
					Status: test.runningStatus, Resumable: test.resumable, ResumeToken: test.resumeToken,
					CreatedAt: now, UpdatedAt: now,
				}))
			}

			manager := &archiveRestoreProbe{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

			resp, err := svc.GetTaskSessionStatus(ctx, "task-archive", "session-archive")
			require.NoError(t, err)
			require.Equal(t, string(test.sessionState), resp.State)
			require.False(t, resp.IsAgentRunning)
			require.False(t, resp.IsResumable)
			require.False(t, resp.NeedsResume)
			require.False(t, resp.NeedsWorkspaceRestore)
			require.Equal(t, resumeReasonTaskArchived, resp.ResumeReason)
			require.Equal(t, int32(0), manager.restoreCalls.Load())
		})
	}
}

func TestRecoverSession_ArchivedTaskDoesNotClearResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-archive", "session-archive", models.TaskSessionStateWaitingForInput)
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-archive", SessionID: "session-archive", TaskID: "task-archive",
		Status: "ready", Resumable: true, ResumeToken: "acp-session",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.ArchiveTask(ctx, "task-archive"))

	manager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	_, err := svc.RecoverSession(ctx, "task-archive", "session-archive", "fresh_start")
	require.Error(t, err)
	require.True(t, errors.Is(err, executor.ErrTaskArchived), "expected archived sentinel, got %v", err)

	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-archive")
	require.NoError(t, err)
	require.NotNil(t, running)
	require.Equal(t, "acp-session", running.ResumeToken)
}

func TestLaunchRestoreWorkspace_ArchivedTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-archive", "session-archive", models.TaskSessionStateCompleted)
	require.NoError(t, repo.ArchiveTask(ctx, "task-archive"))

	manager := &archiveRestoreProbe{mockAgentManager: &mockAgentManager{}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	_, err := svc.launchRestoreWorkspace(ctx, &LaunchSessionRequest{
		TaskID: "task-archive", SessionID: "session-archive", Intent: IntentRestoreWorkspace,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, executor.ErrTaskArchived), "expected archived sentinel, got %v", err)
	require.Equal(t, int32(0), manager.restoreCalls.Load())
}
