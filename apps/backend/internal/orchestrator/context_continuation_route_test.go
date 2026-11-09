package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedRetryingContextRoute(t *testing.T, ctx context.Context, svc *Service, sessionID string) {
	t.Helper()
	repo := svc.repo.(interface {
		SaveRouteState(context.Context, dynamicruntime.RouteState) error
	})
	require.NoError(t, repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: sessionID, LogicalProfileID: "dynamic-logical", ExecutionProfileID: "candidate-1",
		Generation: 1, ProfileVersion: 1, Status: dynamicRouteStatusRetrying, UpdatedAt: time.Now().UTC(),
	}))
	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID, session.ExecutionProfileID = "dynamic-logical", "candidate-1"
	session.RouteGeneration, session.RouteState = 1, dynamicRouteStatusRetrying
	require.NoError(t, svc.repo.UpdateTaskSession(ctx, session))
}

func newContextRouteTestService(t *testing.T, agentManager *mockAgentManager) (*Service, string) {
	t.Helper()
	const taskID, sessionID = "task-context-route", "session-context-route"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	return svc, sessionID
}

// A failed continuation returns its claim to action_required in both the engine
// and the session projection, so a later continuation can claim it again.
func TestReleaseAbandonedContextContinuationRouteRestoresSessionProjection(t *testing.T) {
	ctx := context.Background()
	svc, sessionID := newContextRouteTestService(t, &mockAgentManager{})
	seedRetryingContextRoute(t, ctx, svc, sessionID)

	succeeded := false
	svc.releaseAbandonedContextContinuationRoute(ctx, sessionID, 1, &succeeded)

	loader := svc.repo.(dynamicRouteStateLoader)
	state, err := loader.LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, state.Status)
	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, session.RouteState)
	require.Equal(t, int64(1), session.RouteGeneration)
}

func TestReleaseAbandonedContextContinuationRouteKeepsNewerGeneration(t *testing.T) {
	ctx := context.Background()
	svc, sessionID := newContextRouteTestService(t, &mockAgentManager{})
	seedRetryingContextRoute(t, ctx, svc, sessionID)

	succeeded := false
	svc.releaseAbandonedContextContinuationRoute(ctx, sessionID, 0, &succeeded)

	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusRetrying, session.RouteState, "a stale generation must not rewrite the projection")
}

// A manual retry accepted behind the recovery fence keeps a "retrying" claim
// for explicit recovery; that recovery owns its release when it fails.
func TestRetainedContextContinuationRouteRequiresNoLiveAgent(t *testing.T) {
	ctx := context.Background()
	agentManager := &mockAgentManager{}
	svc, sessionID := newContextRouteTestService(t, agentManager)
	seedRetryingContextRoute(t, ctx, svc, sessionID)
	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	require.True(t, svc.retainedContextContinuationRoute(ctx, session))
	agentManager.isAgentRunning = true
	require.False(t, svc.retainedContextContinuationRoute(ctx, session), "a live launch owner keeps its claim")
	agentManager.isAgentRunning = false
	session.RouteState = dynamicRouteStatusActionRequired
	require.False(t, svc.retainedContextContinuationRoute(ctx, session))
}

func TestClaimContextContinuationRouteRejectsStaleGenerationAndRunningAgent(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name         string
		stale        bool
		agentRunning bool
	}{
		{name: "stale generation", stale: true},
		{name: "agent still running", agentRunning: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			agentManager := &mockAgentManager{isAgentRunning: test.agentRunning}
			svc, sessionID := newContextRouteTestService(t, agentManager)
			seedRetryingContextRoute(t, ctx, svc, sessionID)
			svc.markDynamicRouteActionRequired(ctx, sessionID, 1, "test recovery")
			session, err := svc.repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			if test.stale {
				session.RouteGeneration++
			}

			claimed, err := svc.claimContextContinuationRoute(ctx, session)
			require.False(t, claimed)
			if test.stale {
				require.ErrorIs(t, err, dynamicruntime.ErrStaleGeneration)
			} else {
				require.Error(t, err)
			}
			state, err := svc.repo.(dynamicRouteStateLoader).LoadRouteState(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, dynamicRouteStatusActionRequired, state.Status)
		})
	}
}

type rejectContextContinuationRouteProjection struct {
	*sqliterepo.Repository
}

func (r *rejectContextContinuationRouteProjection) UpdateTaskSessionIfCurrentSnapshot(
	context.Context,
	*models.TaskSession,
	models.TaskSessionState,
	time.Time,
	map[string]interface{},
) (bool, error) {
	return false, nil
}

func TestClaimContextContinuationRouteRestoresActionRequiredWhenProjectionLoses(t *testing.T) {
	ctx := context.Background()
	svc, sessionID := newContextRouteTestService(t, &mockAgentManager{})
	repo := svc.repo.(*sqliterepo.Repository)
	seedRetryingContextRoute(t, ctx, svc, sessionID)
	svc.markDynamicRouteActionRequired(ctx, sessionID, 1, "test recovery")
	svc.repo = &rejectContextContinuationRouteProjection{Repository: repo}
	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	claimed, err := svc.claimContextContinuationRoute(ctx, session)
	require.False(t, claimed)
	require.ErrorIs(t, err, dynamicruntime.ErrStaleGeneration)
	state, err := svc.repo.(dynamicRouteStateLoader).LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, state.Status)
}
