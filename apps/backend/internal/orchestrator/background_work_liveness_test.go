package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
