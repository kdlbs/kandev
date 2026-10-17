package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestReconcileOrphanedDynamicStartingRoutes_SweepsStrandedWaitingRoute fixes
// the deadlock a ceiling-deferred automatic launch leaves behind: its session
// sits in WAITING_FOR_INPUT still owning a "starting" route, but the runtime it
// needs was torn down and nothing relaunches it. The routing orphan sweep is
// the only pass that can release the route, so it must treat a WAITING_FOR_INPUT
// session with no live executor row as stranded and mark it action_required.
func TestReconcileOrphanedDynamicStartingRoutes_SweepsStrandedWaitingRoute(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-stranded-waiting-route"
		sessionID   = "session-stranded-waiting-route"
		executionID = "execution-stranded-waiting-route"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	seedEngine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, seedEngine, true))
	seedClaimedDynamicRoute(t, ctx, repo, seedEngine, sessionID, executionID)
	// Reconcile through a fresh engine so the recovery reads the durable route
	// row, not the seeding process's cache.
	recoveryEngine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, recoveryEngine, true))

	svc.reconcileOrphanedDynamicStartingRoutes(ctx)

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, routeState)
	require.Equal(t, dynamicRouteStatusActionRequired, routeState.Status)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, session.RouteState)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State,
		"releasing the route must not change the session's own state")
}

// TestReconcileOrphanedDynamicStartingRoutes_KeepsWaitingRouteWithLiveExecutor
// is the negative gate: a WAITING_FOR_INPUT session with a live executors_running
// row still has an owner, so its "starting" route may legitimately be mid-launch
// and must not grow a recovery banner.
func TestReconcileOrphanedDynamicStartingRoutes_KeepsWaitingRouteWithLiveExecutor(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-waiting-route-live-executor"
		sessionID   = "session-waiting-route-live-executor"
		executionID = "execution-waiting-route-live-executor"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	seedEngine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, seedEngine, true))
	seedClaimedDynamicRoute(t, ctx, repo, seedEngine, sessionID, executionID)
	recoveryEngine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, recoveryEngine, true))

	svc.reconcileOrphanedDynamicStartingRoutes(ctx)

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, routeState)
	require.Equal(t, "starting", routeState.Status)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "starting", session.RouteState)
}
