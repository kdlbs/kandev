package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentruntimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
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

			seedEngine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, seedEngine, true))
			seedClaimedDynamicRoute(t, ctx, repo, seedEngine, sessionID, executionID)
			// Reconcile through a fresh engine. This proves the startup path loads
			// the durable route row instead of relying on the old process cache.
			recoveryEngine := dynamicruntime.NewEngine(
				dynamicruntime.WithPersistence(repo),
				dynamicruntime.WithStateLoader(repo),
			)
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, recoveryEngine, true))

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

// TestReconcileOrphanedDynamicStartingRoutes_PrecedesGeneralStartupReconciliation
// is the regression test for the CodeRabbit finding on Create PR review
// (service.go#L2580): Service.Start runs reconcileExecutorSessionsOnStartup
// before reconcileOrphanedDynamicStartingRoutes, and the former flips every
// STARTING/RUNNING session with an executors_running row to
// WAITING_FOR_INPUT — a state the orphan sweep treats as an already-explained
// terminal outcome and skips. Almost every claimed dynamic route has an
// executors_running row by the time a crash strands it (PrepareSession's
// background workspace launch creates the row early), so running the two in
// Start's original order would make the sweep a no-op for the exact scenario
// it exists to recover. This test drives the two reconcilers in Start's
// actual order and requires the route to still reach action_required.
func TestReconcileOrphanedDynamicStartingRoutes_PrecedesGeneralStartupReconciliation(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-orphan-ordering"
		sessionID   = "session-dynamic-orphan-ordering"
		executionID = "execution-dynamic-orphan-ordering"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        sessionID,
		SessionID: sessionID,
		TaskID:    taskID,
		Runtime:   agentruntimekind.RuntimeStandalone,
		Status:    models.ExecutorRunningStatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
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

	// Mirror Service.Start's exact ordering: the orphan sweep must run before
	// the general startup reconciler.
	svc.reconcileOrphanedDynamicStartingRoutes(ctx)
	svc.reconcileExecutorSessionsOnStartup(ctx)

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
	// The general reconciler still ran (and ran second): the session itself
	// settles to WAITING_FOR_INPUT the ordinary way.
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("task session state = %q, want WAITING_FOR_INPUT", session.State)
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

func TestDynamicTaskDownstreamLaunchMarksRouteActiveAfterProcessStart(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-downstream-active"
		sessionID   = "session-dynamic-downstream-active"
		executionID = "execution-dynamic-downstream-active"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error { return nil },
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)
	started := make(chan struct{})
	svc.executor.SetOnAgentProcessStarted(func(callbackCtx context.Context, callbackTaskID, callbackSessionID, callbackExecutionID string) {
		svc.handleAgentProcessStarted(callbackCtx, callbackTaskID, callbackSessionID, callbackExecutionID)
		close(started)
	})

	downstream := &dynamicTaskDownstream{
		service:   svc,
		task:      &v1.Task{ID: taskID, WorkspaceID: "ws1", Description: "test"},
		sessionID: sessionID,
		options: executor.LaunchOptions{
			AgentProfileID: "candidate-1",
			StartAgent:     true,
		},
	}
	if _, err := downstream.Launch(ctx, dynamicruntime.DownstreamLaunch{
		ExecutionProfileID: "candidate-1",
		Decision:           decision,
	}); err != nil {
		t.Fatalf("dynamic downstream launch: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process-start settlement")
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Status != dynamicRouteStatusActive || state.Generation != decision.Generation {
		t.Fatalf("route state after downstream launch = %#v, want active generation %d", state, decision.Generation)
	}
}

func TestMarkDynamicRouteActionRequiredPublishesRouteProjection(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-action-event"
		sessionID   = "session-dynamic-action-event"
		executionID = "execution-dynamic-action-event"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	svc.markDynamicRouteActionRequired(ctx, sessionID, decision.Generation, "launch_failed")

	if len(eventBus.events) != 1 || eventBus.events[0].subject != events.TaskSessionStateChanged {
		t.Fatalf("published events = %#v, want one session state event", eventBus.events)
	}
	data, ok := eventBus.events[0].event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map[string]interface{}", eventBus.events[0].event.Data)
	}
	if data["route_state"] != dynamicRouteStatusActionRequired {
		t.Fatalf("event route_state = %#v, want action_required", data["route_state"])
	}
	if data["route_reason"] != "launch_failed" {
		t.Fatalf("event route_reason = %#v, want launch_failed", data["route_reason"])
	}
}

func TestMarkDynamicRouteActiveDoesNotOverwriteActionRequiredRoute(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-dynamic-active-noop"
		sessionID = "session-dynamic-active-noop"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = "dynamic-logical"
	session.ExecutionProfileID = "candidate-1"
	session.RouteGeneration = 1
	session.RouteState = dynamicRouteStatusActionRequired
	session.RouteReason = "launch_failed"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: sessionID, LogicalProfileID: session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID, Generation: 1,
		Status: dynamicRouteStatusActionRequired, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	engine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))

	svc.markDynamicRouteActive(ctx, sessionID, 1)

	loaded, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded == nil || loaded.Status != dynamicRouteStatusActionRequired {
		t.Fatalf("durable route state = %#v, want action_required", loaded)
	}
	updated, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession after MarkActive: %v", err)
	}
	if updated.RouteState != dynamicRouteStatusActionRequired || updated.RouteReason != "launch_failed" {
		t.Fatalf("session route projection = (%q, %q), want action_required/launch_failed", updated.RouteState, updated.RouteReason)
	}
}

func TestHandleAgentProcessStartedMarksDynamicRouteActive(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-process-started"
		sessionID   = "session-dynamic-process-started"
		executionID = "execution-dynamic-process-started"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	svc.handleAgentProcessStarted(ctx, taskID, sessionID, executionID)

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActive {
		t.Fatalf("route state after process start = %#v, want active generation %d", state, decision.Generation)
	}
}

func TestHandleAgentProcessStartedSkipsCancelledSession(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-process-cancelled"
		sessionID   = "session-dynamic-process-cancelled"
		executionID = "execution-dynamic-process-cancelled"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCancelled)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	svc.handleAgentProcessStarted(ctx, taskID, sessionID, executionID)

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != "starting" {
		t.Fatalf("route state after cancelled process start = %#v, want unchanged starting generation %d", state, decision.Generation)
	}
}

func TestHandleAgentProcessStartFailedMarksDynamicRouteActionRequired(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-process-failed"
		sessionID   = "session-dynamic-process-failed"
		executionID = "execution-dynamic-process-failed"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	svc.handleAgentProcessStartFailed(ctx, taskID, sessionID, executionID, errors.New("provider process failed"))

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActionRequired {
		t.Fatalf("route state after process failure = %#v, want action_required generation %d", state, decision.Generation)
	}
}

func TestRouteDynamicAgentFailureMarksSuccessorGenerationActionRequiredWhenLaunchFails(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-successor-failure"
		sessionID   = "session-dynamic-successor-failure"
		executionID = "execution-dynamic-successor-failure"
		dynamicID   = "dynamic-successor-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{stopAgentWithReasonErr: errors.New("predecessor stop failed")}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	engineOptions := []dynamicruntime.EngineOption{dynamicruntime.WithPersistence(repo)}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{
			executionProfileID: "candidate-1",
			enabled:            true,
			rulesJSON:          `{"on_provider_error":"try_next"}`,
		},
		{executionProfileID: "candidate-2", enabled: true},
	}, engineOptions...)
	svc.SetProfileExecutionResolver(resolver)
	selected, err := resolver.Resolve(ctx, sessionID, dynamicID, 0, "")
	if err != nil {
		t.Fatalf("initial dynamic resolve: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = dynamicID
	session.ExecutionProfileID = selected.ExecutionProfileID
	session.RouteGeneration = selected.Generation
	session.RouteState = selected.Decision.Status
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("project initial route: %v", err)
	}
	svc.beginDynamicAttempt(sessionID)
	svc.bindDynamicAttemptExecution(sessionID, executionID)

	handled := svc.routeDynamicAgentFailure(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		PromptGeneration: 1,
	}, &routingerr.Error{
		Code: routingerr.CodeProviderOverloaded, Class: routingerr.ClassTransient,
		FallbackAllowed: true,
	})
	// The successor launch can complete in this call or on a detached worker.
	// Both paths must settle the claimed generation before this recovery can
	// be presented to the user.
	deadline := time.Now().Add(2 * time.Second)
	var state *dynamicruntime.RouteState
	for {
		state, err = repo.LoadRouteState(ctx, sessionID)
		if err != nil {
			t.Fatalf("LoadRouteState while waiting for successor failure: %v", err)
		}
		if state != nil && state.Generation == 2 && state.Status == dynamicRouteStatusActionRequired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("successor launch result not settled (handled=%v): %#v", handled, state)
		}
		time.Sleep(10 * time.Millisecond)
	}

	updated, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession after successor failure: %v", err)
	}
	if updated.RouteGeneration != 2 || updated.RouteState != dynamicRouteStatusActionRequired {
		t.Fatalf("session route projection = generation %d state %q, want generation 2/action_required", updated.RouteGeneration, updated.RouteState)
	}
}
