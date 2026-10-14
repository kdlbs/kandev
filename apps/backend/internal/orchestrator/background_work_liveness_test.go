package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTerminalSessionKeepsBackgroundWorkUntilAgentStops(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateCompleted, "", false)
	require.Empty(t, observer.settledSessions(), "terminal session state is not proof of process termination")
	agent.isAgentRunning = false
	svc.settleOpenBackgroundWork(ctx, "s1")
	require.Equal(t, []string{"s1"}, observer.settledSessions())
}

func TestTerminalSessionKeepsBackgroundWorkWhenProbeFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agent := &failingAgentLivenessProbe{mockAgentManager: &mockAgentManager{}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateFailed, "", false)
	require.Empty(t, observer.settledSessions())
}

func TestFailedWorkflowStopKeepsBackgroundWorkOpen(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agent := &mockAgentManager{isAgentRunning: true, stopAgentErr: errors.New("injected stop failure"), getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "exec", nil }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	svc.completeAndStopSession(ctx, "t1", session)
	require.Empty(t, observer.settledSessions())
	require.Len(t, agent.stopAgentArgs, 1)
}

func TestWorkflowStopSettlesOnlyAfterProcessStops(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agent := &mockAgentManager{isAgentRunning: true, getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "exec", nil }}
	agent.stopAgentFunc = func(context.Context, string, bool) error {
		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateCompleted, session.State)
		agent.isAgentRunning = false
		return nil
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	svc.completeAndStopSession(ctx, "t1", session)
	require.Equal(t, []string{"s1"}, observer.settledSessions())
}

func TestStartupPreservesUnresolvedBackgroundWorkRecovery(t *testing.T) {
	for _, liveness := range []models.ProcessLiveness{models.ProcessLivenessUnknown, models.ProcessLivenessAlive, models.ProcessLivenessDead} {
		t.Run(fmt.Sprint(liveness), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "runtime", TaskID: "t1", SessionID: "s1", AgentExecutionID: "retained-exec"}))
			agent := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "", errors.New("not re-adopted") },
				rowLivenessFn:                func(*models.ExecutorRunning) models.ProcessLiveness { return liveness },
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
			observer := &settlingBackgroundWorkObserver{open: []string{"s1"}}
			svc.SetBackgroundWorkObserver(observer)
			svc.settleOrphanedBackgroundWorkOnStartup(ctx)
			if liveness == models.ProcessLivenessDead {
				require.Equal(t, []string{"s1"}, observer.settledSessions())
			} else {
				require.Empty(t, observer.settledSessions())
			}
		})
	}
}

func TestSynchronousStopWithoutExecutionPreservesUnresolvedBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retained", TaskID: "t1", SessionID: "s1", AgentExecutionID: "retained"}))
	agent := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "", lifecycle.ErrNoExecutionForSession }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	require.NoError(t, svc.StopSessionSynchronously(ctx, "s1", "cleanup", true))
	require.Empty(t, observer.settledSessions())
}

func TestSynchronousStopSettlesObservedExecutionAfterTeardown(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agent := &mockAgentManager{isAgentRunning: true, getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "exec", nil }}
	agent.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error { agent.isAgentRunning = false; return nil }
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	require.NoError(t, svc.StopSessionSynchronously(ctx, "s1", "cleanup", true))
	require.Equal(t, []string{"s1"}, observer.settledSessions())
}

func TestRecoverableCleanupKeepsRetainedBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retained", TaskID: "t1", SessionID: "s1", AgentExecutionID: "retained"}))
	agent := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "", lifecycle.ErrNoExecutionForSession }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	require.True(t, svc.cleanupAgentExecutionWithReason(ctx, "retained", "t1", "s1", lifecycle.StopReasonRecoverableAgentFailure))
	require.Empty(t, observer.settledSessions(), "successful recoverable cleanup may retain its remote owner")
	svc.cleanupAgentExecution("retained", "t1", "s1")
	require.Equal(t, []string{"s1"}, observer.settledSessions(), "actual completion also works after a completed cleanup claim")
}

func TestBackgroundWorkStopEvidenceMatchesDurableOwner(t *testing.T) {
	for _, tc := range []struct {
		name, stopped, tracked string
		wantSettled            bool
	}{
		{name: "same stopped owner", stopped: "retained", wantSettled: true},
		{name: "old stopped owner", stopped: "old"},
		{name: "different tracked owner", tracked: "old"},
		{name: "same tracked owner", tracked: "retained", wantSettled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retained", TaskID: "t1", SessionID: "s1", AgentExecutionID: "retained"}))
			agent := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return tc.tracked, nil }}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
			observer := &settlingBackgroundWorkObserver{}
			svc.SetBackgroundWorkObserver(observer)
			svc.settleBackgroundWork(ctx, "s1", tc.stopped)
			require.Equal(t, tc.wantSettled, len(observer.settledSessions()) == 1)
		})
	}
}

func TestDelayedCompletionKeepsRetainedSuccessorBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "successor", TaskID: "t1", SessionID: "s1", AgentExecutionID: "successor"}))
	agent := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "", lifecycle.ErrNoExecutionForSession }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	svc.cleanupAgentExecution("old", "t1", "s1")
	require.Empty(t, observer.settledSessions())
	svc.cleanupAgentExecution("", "t1", "s1")
	require.Empty(t, observer.settledSessions())
}

func TestVerifiedBackgroundWorkStopStillRequiresSuccessfulLivenessProbe(t *testing.T) {
	for _, failProbe := range []bool{false, true} {
		t.Run(fmt.Sprint(failProbe), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retained", TaskID: "t1", SessionID: "s1", AgentExecutionID: "retained"}))
			base := &mockAgentManager{isAgentRunning: true}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), base)
			if failProbe {
				svc = createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &failingAgentLivenessProbe{mockAgentManager: base})
			}
			observer := &settlingBackgroundWorkObserver{}
			svc.SetBackgroundWorkObserver(observer)
			svc.settleBackgroundWork(ctx, "s1", "retained")
			require.Empty(t, observer.settledSessions())
		})
	}
}
