package orchestrator

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantInputNativeStopAuthorizationAndIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "session", "task", "execution")
	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)
	svc.sessionControlCheck = func(context.Context, string) error { return errors.New("control denied") }
	_, err := svc.StopTaskSessionForCoordinator(ctx, "task", "session")
	require.ErrorContains(t, err, "control denied")
	svc.sessionControlCheck = func(context.Context, string) error { return nil }
	_, err = svc.StopTaskSessionForCoordinator(ctx, "foreign", "session")
	require.ErrorContains(t, err, "belongs to task")
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	stopped, err := svc.StopTaskSessionForCoordinator(ctx, "task", "session")
	require.NoError(t, err)
	require.True(t, stopped)
	waitForStopCall(t, agentManager)
	stopped, err = svc.StopTaskSessionForCoordinator(ctx, "task", "session")
	require.NoError(t, err)
	require.False(t, stopped)
}
