package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedClaimedDynamicRoute claims generation 1 for a fixed single-candidate
// dynamic profile directly through the engine (bypassing the profile store,
// which these tests do not need) and projects the claim onto the
// task_sessions row the way persistDynamicLaunchDecision would.
func seedClaimedDynamicRoute(
	t *testing.T,
	ctx context.Context,
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
		UpdateTaskSession(context.Context, *models.TaskSession) error
	},
	engine *dynamicruntime.Engine,
	sessionID, executionID string,
) dynamicruntime.RouteDecision {
	t.Helper()
	profile := dynamicruntime.Profile{
		ID: "dynamic-logical", Version: 1,
		Candidates: []dynamicruntime.Candidate{{ID: "candidate-1", Enabled: true}},
	}
	decision, err := engine.Select(sessionID, profile, 0, "")
	if err != nil {
		t.Fatalf("claim initial route generation: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = profile.ID
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = decision.Status
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	return decision
}

func TestRouteDynamicAgentFailure_MarksActionRequiredWhenPreResultUnsafe(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-pre-result-unsafe"
		sessionID   = "session-dynamic-pre-result-unsafe"
		executionID = "execution-dynamic-pre-result-unsafe"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	// Reproduce the observed production failure verbatim: the agent already
	// streamed output for this attempt (OutputObserved=true), so
	// dynamicPreResultSafe must refuse to auto-replace the turn.
	svc.beginDynamicAttempt(sessionID)
	svc.bindDynamicAttemptExecution(sessionID, executionID)
	svc.observeDynamicAttempt(sessionID, executionID, true, false)

	classified := &routingerr.Error{
		Code: routingerr.CodeProviderOverloaded, Class: routingerr.ClassTransient,
		FallbackAllowed: true, AutoRetryable: true,
	}
	handled := svc.routeDynamicAgentFailure(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
	}, classified)
	if handled {
		t.Fatal("routeDynamicAgentFailure claimed to handle a pre-result-unsafe failure")
	}

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState == nil || routeState.Status != "action_required" {
		t.Fatalf("durable route state = %#v, want action_required", routeState)
	}
	if routeState.Generation != 1 {
		t.Fatalf("route generation = %d, want 1 (no successor was claimed)", routeState.Generation)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.RouteState != "action_required" {
		t.Fatalf("task session route_state = %q, want action_required", session.RouteState)
	}
}

func TestRouteDynamicAgentFailure_MarksActionRequiredWhenFallbackNotAllowed(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-fallback-not-allowed"
		sessionID   = "session-dynamic-fallback-not-allowed"
		executionID = "execution-dynamic-fallback-not-allowed"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	classified := &routingerr.Error{Code: routingerr.CodeTask, FallbackAllowed: false}
	handled := svc.routeDynamicAgentFailure(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
	}, classified)
	if handled {
		t.Fatal("routeDynamicAgentFailure claimed to handle a non-fallback failure")
	}

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState == nil || routeState.Status != "action_required" {
		t.Fatalf("durable route state = %#v, want action_required", routeState)
	}
}

func TestLaunchDynamicRouteAction_MarksRouteActionRequiredWhenRelaunchFails(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-route-action-failure"
		sessionID   = "session-dynamic-route-action-failure"
		executionID = "execution-dynamic-route-action-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	stopErr := errors.New("runtime teardown failed")
	agentManager := &mockAgentManager{stopAgentWithReasonErr: stopErr}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	// Simulates the backend route-action handler having already claimed the
	// successor generation as "starting" before calling LaunchDynamicRouteAction.
	if err := svc.LaunchDynamicRouteAction(ctx, sessionID); err == nil {
		t.Fatal("LaunchDynamicRouteAction succeeded despite predecessor stop failing")
	}

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState == nil || routeState.Status != "action_required" {
		t.Fatalf("durable route state = %#v, want action_required", routeState)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.RouteState != "action_required" {
		t.Fatalf("task session route_state = %q, want action_required", session.RouteState)
	}
}

func TestReconcileOrphanedDynamicStartingRoutes_SweepsInFlightLaunchStates(t *testing.T) {
	// STARTING means a launch was under way when the owning process stopped;
	// IDLE (office runs) means the executor was already torn down between
	// turns. Both have no possible owner left after a restart.
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateStarting,
		models.TaskSessionStateIdle,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			taskID := "task-dynamic-orphan-sweep-" + string(state)
			sessionID := "session-dynamic-orphan-sweep-" + string(state)
			executionID := "execution-dynamic-orphan-sweep-" + string(state)
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, state)
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

			engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
			seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

			svc.reconcileOrphanedDynamicStartingRoutes(ctx)

			routeState, err := repo.LoadRouteState(ctx, sessionID)
			if err != nil {
				t.Fatalf("LoadRouteState: %v", err)
			}
			if routeState == nil || routeState.Status != "action_required" {
				t.Fatalf("durable route state = %#v, want action_required", routeState)
			}
			session, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			if session.RouteState != "action_required" {
				t.Fatalf("task session route_state = %q, want action_required", session.RouteState)
			}
		})
	}
}

// TestReconcileOrphanedDynamicStartingRoutes_SkipsNonOrphanStates is the
// regression test for review finding F3: RUNNING already has a live owner;
// CREATED is PrepareSession's ordinary pre-first-prompt claim, which can sit
// unstarted for a long time by design; WAITING_FOR_INPUT/COMPLETED/FAILED/
// CANCELLED are terminal or parked outcomes the normal session UI already
// explains — including every dynamic session that predates MarkActive and is
// "starting" only because that transition did not exist yet, not because
// anything is stuck. None of these should grow a stale recovery banner.
func TestReconcileOrphanedDynamicStartingRoutes_SkipsNonOrphanStates(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateRunning,
		models.TaskSessionStateCreated,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			taskID := "task-dynamic-orphan-skip-" + string(state)
			sessionID := "session-dynamic-orphan-skip-" + string(state)
			executionID := "execution-dynamic-orphan-skip-" + string(state)
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, state)
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

			engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
			seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

			svc.reconcileOrphanedDynamicStartingRoutes(ctx)

			routeState, err := repo.LoadRouteState(ctx, sessionID)
			if err != nil {
				t.Fatalf("LoadRouteState: %v", err)
			}
			if routeState == nil || routeState.Status != "starting" {
				t.Fatalf("durable route state = %#v, want unchanged starting for a %s session", routeState, state)
			}
		})
	}
}

func TestMarkDynamicRouteActive_MirrorsBothStores(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-mark-active"
		sessionID   = "session-dynamic-mark-active"
		executionID = "execution-dynamic-mark-active"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	svc.markDynamicRouteActive(ctx, sessionID, decision.Generation)

	routeState, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState == nil || routeState.Status != dynamicRouteStatusActive {
		t.Fatalf("durable route state = %#v, want active", routeState)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.RouteState != dynamicRouteStatusActive {
		t.Fatalf("task session route_state = %q, want active", session.RouteState)
	}

	// A healthy active route must not be swept by startup reconciliation: it
	// is no longer "starting", so ListStartingRouteStates never returns it.
	svc.reconcileOrphanedDynamicStartingRoutes(ctx)
	routeState, err = repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after sweep: %v", err)
	}
	if routeState.Status != dynamicRouteStatusActive {
		t.Fatalf("route state after sweep = %#v, want unchanged active", routeState)
	}
}
