package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// A concurrent lifecycle path that already finished tearing down this exact
// execution leaves nothing for the prompt-readiness reaper to stop, so the
// resume must continue instead of failing with "teardown is already owned".
func TestReapPromptUnreadyExecution_AcceptsCompletedTeardownOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-reap-completed-owner"
		sessionID   = "session-reap-completed-owner"
		executionID = "execution-reap-completed-owner"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)

	var stopCalls atomic.Int32
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls.Add(1)
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.executionTeardownClaims.Store(terminalExecutionKey(sessionID, executionID), executionTeardownClaim{
		intent:           executionTeardownIntentForce,
		expiresAt:        time.Now().Add(time.Minute),
		cleanupCompleted: true,
	})

	err := svc.reapPromptUnreadyExecution(ctx, sessionID, errors.New("agent never became prompt-ready"))

	require.NoError(t, err)
	require.Zero(t, stopCalls.Load(), "a completed teardown must not be repeated")
}

func newExecutionLessStopService(t *testing.T, state models.TaskSessionState) (*Service, *sqliterepo.Repository, *taskservice.Service, string, string) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID    = "task-execution-less-stop"
		sessionID = "session-execution-less-stop"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, state)
	turns := taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo,
		Turns: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	turn, err := turns.StartTurn(ctx, sessionID)
	require.NoError(t, err)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(_ context.Context, id string) (string, error) {
			return "", fmt.Errorf("%w: %s", lifecycle.ErrNoExecutionForSession, id)
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.SetTurnService(turns)
	return svc, repo, turns, sessionID, turn.ID
}

// Stopping a settled session whose runtime is already gone closes its open
// turn, so task-wide idle checks such as workspace-source attach no longer
// report an active turn.
func TestStopSession_ExecutionLessSettledSessionClosesOpenTurn(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, sessionID, _ := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)

	require.NoError(t, svc.StopSession(ctx, sessionID, "stopped via API", false))

	_, err := repo.GetActiveTurnBySessionID(ctx, sessionID)
	require.ErrorIs(t, err, sql.ErrNoRows, "the open turn must be closed")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}

// A session whose launch may still be registering its execution keeps the
// original not-found error and its turn.
func TestStopSession_ExecutionLessLaunchingSessionKeepsTurn(t *testing.T) {
	ctx := context.Background()
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
	} {
		t.Run(string(state), func(t *testing.T) {
			svc, _, turns, sessionID, turnID := newExecutionLessStopService(t, state)

			err := svc.StopSession(ctx, sessionID, "stopped via API", false)

			require.ErrorIs(t, err, lifecycle.ErrNoExecutionForSession)
			active, turnErr := turns.GetActiveTurn(ctx, sessionID)
			require.NoError(t, turnErr)
			require.Equal(t, turnID, active.ID)
		})
	}
}
