package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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

func queueSnapshotContents(t *testing.T, svc *Service, sessionID string) []string {
	t.Helper()
	entries, _, err := svc.messageQueue.SnapshotSession(context.Background(), sessionID)
	require.NoError(t, err)
	contents := make([]string, 0, len(entries))
	for _, entry := range entries {
		contents = append(contents, entry.Content)
	}
	return contents
}

func setActionRequiredRoute(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "dynamic-logical"
	session.ExecutionProfileID = "candidate-1"
	session.RouteGeneration = 1
	session.RouteState = dynamicRouteStatusActionRequired
	session.RouteReason = dynamicRouteOrphanedStartingReason
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
}

// A queued message for a WAITING_FOR_INPUT session whose dynamic route awaits a
// recovery action and which has no executor record cannot run until the user
// applies that action. Automatic drains must leave the entry queued instead of
// reserving it, failing in ensureSessionRunning, and requeueing it forever.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.15
func TestQueueDrainSkipsSessionWhoseDynamicRouteAwaitsAction(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	setActionRequiredRoute(t, repo, "s1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	workerDone := make(chan struct{}, 4)
	svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }

	_, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "hello", "", "user", false, nil)
	require.NoError(t, err)

	for attempt := 0; attempt < 2; attempt++ {
		if svc.drainQueuedMessageForPromptableSession(ctx, "s1") {
			select {
			case <-workerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the dispatched queued message to settle")
			}
			t.Fatalf("drain %d dispatched a queued message to a session that cannot run until its route action is applied", attempt+1)
		}
	}
	require.Equal(t, []string{"hello"}, queueSnapshotContents(t, svc, "s1"))
}

// A dispatch that reaches a WAITING_FOR_INPUT session whose dynamic route is
// still "starting" with no executor record has no launch owner. The failure
// releases the route to action_required so the session shows its route
// recovery actions, keeps the message queued, and stops further drains.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.16
func TestQueuedDispatchToStrandedStartingRouteMarksActionRequired(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-stranded-queue"
		sessionID   = "session-stranded-queue"
		executionID = "execution-stranded-queue"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	seedEngine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, seedEngine, true))
	seedClaimedDynamicRoute(t, ctx, repo, seedEngine, sessionID, executionID)
	recoveryEngine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, recoveryEngine, true))
	workerDone := make(chan struct{}, 4)
	svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, "hello", "", "user", false, nil)
	require.NoError(t, err)
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, sessionID),
		"the first drain must attempt the dispatch while the route still reads starting")
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the queued dispatch to settle")
	}

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, session.RouteState,
		"a dispatch with no launch owner must release the stranded route for user recovery")
	require.Equal(t, dynamicRouteOrphanedStartingReason, session.RouteReason)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, []string{"hello"}, queueSnapshotContents(t, svc, sessionID))
	require.False(t, svc.drainQueuedMessageForPromptableSession(ctx, sessionID),
		"later drains must wait for the route recovery action")
}

func newStrandedQueueFixture(
	t *testing.T,
	taskID, sessionID, executionID string,
) (*sqliterepo.Repository, *Service, chan struct{}, *dynamicruntime.Engine) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	seedEngine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, seedEngine, true))
	seedClaimedDynamicRoute(t, ctx, repo, seedEngine, sessionID, executionID)
	recoveryEngine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, recoveryEngine, true))
	workerDone := make(chan struct{}, 4)
	svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }
	return repo, svc, workerDone, recoveryEngine
}

func TestStrandedQueuedDispatchDoesNotChangeReplacementRoute(t *testing.T) {
	for _, tc := range []struct {
		name    string
		durable bool
	}{{name: "ordinary"}, {name: "durable", durable: true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			const taskID, sessionID = "task-replaced-route", "session-replaced-route"
			repo, svc, _, _ := newStrandedQueueFixture(t, taskID, sessionID, "execution-replaced-route")
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
			require.NoError(t, err)
			_, err = svc.messageQueue.QueueMessageWithMetadata(
				ctx, sessionID, taskID, "old queued prompt", "", "user", false, nil,
				map[string]interface{}{messagequeue.MetadataLifecycleDurable: tc.durable},
			)
			require.NoError(t, err)
			queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, ok)
			require.NotNil(t, queued)
			guard := svc.lockCancelInFlightGuard(sessionID)
			reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(ctx, identity, queued.ID, queued)
			guard.release()
			require.NotNil(t, reservation)
			t.Cleanup(func() { svc.clearQueuedDispatchInFlightIfCurrent(sessionID, reservation) })

			_, err = repo.DB().ExecContext(ctx,
				`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`,
				"replacement-incarnation", sessionID,
			)
			require.NoError(t, err)
			dispatchErr := fmt.Errorf("%w: %w", ErrSessionRuntimeUnavailable, errSessionAwaitingRuntimeLaunch)
			svc.finishQueuedMessageExecution(
				ctx, sessionID, sessionID, queued, reservation, false, false, false, dispatchErr,
			)

			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, "replacement-incarnation", session.QueueIncarnationID)
			require.Equal(t, "starting", session.RouteState)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			route, err := repo.LoadRouteState(ctx, sessionID)
			require.NoError(t, err)
			require.NotNil(t, route)
			require.Equal(t, "starting", route.Status)
			replacement := identity
			replacement.SessionIncarnationID = session.QueueIncarnationID
			entries, _, err := svc.messageQueue.SnapshotSessionForIdentity(ctx, replacement)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

// A retained durable delivery keeps its queued_messages row while it is
// reserved. When the stranded starting route is released for user recovery,
// the row must be released for the same session incarnation, not inserted a
// second time under the same id.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.16
func TestQueuedDispatchToStrandedStartingRouteReleasesDurableReservation(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-stranded-durable-queue"
		sessionID   = "session-stranded-durable-queue"
		executionID = "execution-stranded-durable-queue"
	)
	repo, svc, workerDone, _ := newStrandedQueueFixture(t, taskID, sessionID, executionID)

	_, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "hello", "", "user", false, nil,
		map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	)
	require.NoError(t, err)
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, sessionID))
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the queued dispatch to settle")
	}

	status := svc.messageQueue.GetStatus(ctx, sessionID)
	require.Equal(t, 1, status.Count,
		"the retained durable entry must become visible for the route recovery action")
	require.Equal(t, "hello", status.Entries[0].Content)
	require.False(t, status.Entries[0].IsReservedInFlight(),
		"the entry must be released from the retained delivery reservation")

	snapshot, _, err := svc.messageQueue.SnapshotSession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1, "release must not duplicate the queued row")
	require.Equal(t, status.Entries[0].ID, snapshot[0].ID)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, dynamicRouteStatusActionRequired, session.RouteState)
}

type pausedActionRequiredRouteRepo struct {
	*sqliterepo.Repository
	sessionID string
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (r *pausedActionRequiredRouteRepo) GetTaskSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	session, err := r.Repository.GetTaskSession(ctx, sessionID)
	if err == nil && session != nil && session.ID == r.sessionID &&
		session.RouteState == dynamicRouteStatusActionRequired {
		r.once.Do(func() {
			close(r.entered)
			<-r.release
		})
	}
	return session, err
}

// A route action may start a successor as soon as action_required is exposed.
// The release that makes the retained entry visible must therefore complete
// before the successor can drain it. The route action waits on the existing
// per-session operation lock until that release is done.
func TestStrandedRouteReleaseHoldsRouteActionUntilEntryVisible(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-stranded-route-barrier"
		sessionID   = "session-stranded-route-barrier"
		executionID = "execution-stranded-route-barrier"
	)
	baseRepo, svc, workerDone, routeEngine := newStrandedQueueFixture(t, taskID, sessionID, executionID)
	paused := &pausedActionRequiredRouteRepo{
		Repository: baseRepo,
		sessionID:  sessionID,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	svc.repo = paused

	_, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "hello", "", "user", false, nil,
		map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	)
	require.NoError(t, err)

	handlerEntered := make(chan struct{})
	handlerStatus := make(chan int, 1)
	svc.SetRouteActionHandler(func(ctx context.Context, request RouteActionRequest) (*RouteActionResult, error) {
		close(handlerEntered)
		handlerStatus <- svc.messageQueue.GetStatus(ctx, request.SessionID).Count
		return &RouteActionResult{SessionID: request.SessionID}, nil
	})

	go svc.drainQueuedMessageForPromptableSession(ctx, sessionID)
	select {
	case <-paused.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the queued failure did not reach the post-mark session reload")
	}

	routeDone := make(chan error, 1)
	go func() {
		_, actionErr := svc.ApplyRouteAction(ctx, RouteActionRequest{
			SessionID: sessionID,
			Action:    RouteActionRetry,
		})
		routeDone <- actionErr
	}()

	require.Eventually(t, func() bool {
		return svc.isRouteActionInFlight(sessionID)
	}, 2*time.Second, 10*time.Millisecond,
		"the route action must reach the per-session operation lock")

	select {
	case <-handlerEntered:
		t.Fatal("a route action ran before the retained entry was released")
	default:
	}

	close(paused.release)
	select {
	case <-handlerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("the route action did not run after the entry was released")
	}
	require.Equal(t, 1, <-handlerStatus,
		"the route action must observe the released entry")

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the queued dispatch did not settle after the release")
	}
	require.NoError(t, <-routeDone)

	status := svc.messageQueue.GetStatus(ctx, sessionID)
	require.Equal(t, 1, status.Count)
	snapshot, _, err := svc.messageQueue.SnapshotSession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1, "the released entry must remain a single row")

	// Model the successor becoming ready after the route action returns. The
	// normal readiness drain must deliver the restored entry to the provider.
	agentMgr, ok := svc.agentManager.(*mockAgentManager)
	require.True(t, ok)
	retried, err := routeEngine.ResumePendingNow(ctx, sessionID, 1)
	require.NoError(t, err, "Retry must claim the route before its successor starts")
	require.Equal(t, dynamicRouteStatusRetrying, retried.Status)
	seedExecutorRunning(t, baseRepo, sessionID, taskID, "execution-route-successor")
	require.NoError(t, svc.markDynamicRouteActive(ctx, sessionID, 1),
		"the ready successor must activate the claimed route")
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, sessionID),
		"the successor readiness event must admit the restored entry")
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the successor did not finish dispatching the restored entry")
	}
	agentMgr.mu.Lock()
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	require.Equal(t, []string{"hello"}, prompts)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, sessionID).Count)
}
