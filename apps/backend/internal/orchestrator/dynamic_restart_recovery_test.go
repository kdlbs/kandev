package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A resumed or prepared runtime settles to WAITING_FOR_INPUT before the
// process-start callback runs. The process did start for this execution, so
// the re-claimed route must become active instead of staying "starting".
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestHandleAgentProcessStartedActivatesRouteWaitingForInput(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-resumed-waiting"
		sessionID   = "session-dynamic-resumed-waiting"
		executionID = "execution-dynamic-resumed-waiting"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
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
		t.Fatalf("route state after resumed process start = %#v, want active generation %d", state, decision.Generation)
	}
}

// The resume-ready path completes the same transition when the runtime is
// already prepared and no process-start callback follows.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestActivateResumedDynamicRouteMarksClaimedRouteActive(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-resume-ready"
		sessionID   = "session-dynamic-resume-ready"
		executionID = "execution-dynamic-resume-ready"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	if err := svc.activateResumedDynamicRoute(ctx, session); err != nil {
		t.Fatalf("activateResumedDynamicRoute: %v", err)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActive {
		t.Fatalf("route state after resume ready = %#v, want active generation %d", state, decision.Generation)
	}
}

var errInjectedRouteActivation = errors.New("injected route activation failure")

// failingRouteActivationRepo fails the same-generation route write that
// completes "starting" -> "active", so a test can observe how a caller reacts
// when the durable activation cannot be written.
type failingRouteActivationRepo struct {
	*sqliterepo.Repository
	failActivation bool
}

func (r *failingRouteActivationRepo) ClaimRouteStateFrom(
	ctx context.Context, expectedGeneration int64, expectedStatus string, state dynamicruntime.RouteState,
) (bool, error) {
	if r.failActivation {
		return false, errInjectedRouteActivation
	}
	return r.Repository.ClaimRouteStateFrom(ctx, expectedGeneration, expectedStatus, state)
}

// A resume-ready activation that cannot complete the durable "starting" ->
// "active" write must report the failure. Reporting success while the route is
// still "starting" lets the resume caller acknowledge delivery or commit a
// continuation against a route a later explicit recovery still treats as
// pending.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestActivateResumedDynamicRouteReportsFailedActivation(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-activation-failure"
		sessionID   = "session-dynamic-activation-failure"
		executionID = "execution-dynamic-activation-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	persistence := &failingRouteActivationRepo{Repository: repo}
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(persistence))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	persistence.failActivation = true
	if err := svc.activateResumedDynamicRoute(ctx, session); err == nil {
		t.Fatal("resume-ready activation reported success while the durable route write failed")
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusStarting {
		t.Fatalf("route state after failed activation = %#v, want unchanged starting generation %d", state, decision.Generation)
	}
}

// A stale generation leaves the route unchanged and is not an activation
// failure: another worker already owns the claimed generation.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestActivateResumedDynamicRouteStaleGenerationIsNotAFailure(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-activation-stale"
		sessionID   = "session-dynamic-activation-stale"
		executionID = "execution-dynamic-activation-stale"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.RouteGeneration = decision.Generation + 1

	if err := svc.activateResumedDynamicRoute(ctx, session); err != nil {
		t.Fatalf("stale generation reported an activation failure: %v", err)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusStarting {
		t.Fatalf("route state after stale activation = %#v, want unchanged starting generation %d", state, decision.Generation)
	}
}

// An explicit recovery may resume a dynamic session whose route claim was
// left "starting" with no live runtime behind it. A live launch owner, a
// passive resume, and a route in any other state keep the guard.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.13
func TestExplicitRecoveryReclaimsStrandedStartingRoute(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		routeState string
		recovery   string
		running    bool
		wantGuard  bool
	}{
		{name: "explicit recovery without a live runtime", routeState: "starting", recovery: recoveryActionResume},
		{name: "passive resume", routeState: "starting", wantGuard: true},
		{name: "live launch owner", routeState: "starting", recovery: recoveryActionResume, running: true, wantGuard: true},
		{name: "manual recovery state", routeState: dynamicRouteStatusActionRequired, recovery: recoveryActionResume, wantGuard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
			session, err := repo.GetTaskSession(ctx, "session1")
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			session.AgentProfileID = "dynamic-logical"
			session.ExecutionProfileID = "candidate-1"
			session.RouteGeneration = 1
			session.RouteState = tt.routeState
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("UpdateTaskSession: %v", err)
			}
			seedExecutorRunning(t, repo, "session1", "task1", "claimed-execution")
			running := tt.running
			agentMgr := &mockAgentManager{isAgentRunningFn: func(context.Context, string) bool { return running }}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

			candidate, err := svc.loadResumeSessionCandidate(ctx, "task1", "session1",
				executor.ResumeOptions{RecoveryAction: tt.recovery}, nil)

			var blocked *sessionOpenRecoveryBlockedError
			if guarded := errors.As(err, &blocked) && blocked.reason == autoResumeBlockedDynamicRoute; guarded != tt.wantGuard {
				t.Fatalf("loadResumeSessionCandidate error = %v, want dynamic-route guard %v", err, tt.wantGuard)
			}
			if !tt.wantGuard && (err != nil || candidate == nil) {
				t.Fatalf("explicit stranded-route admission = %#v, %v; want a candidate without an error", candidate, err)
			}
		})
	}
}

// An explicit recovery action that cannot resume must fail. Reporting success
// with no runtime lets the caller acknowledge delivery or commit a
// continuation against an execution that does not exist. A passive session
// open keeps its waiting response.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.13
func TestLaunchResumeRecoveryActionReportsBlockedRouteAsError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = "dynamic-logical"
	session.RouteState = dynamicRouteStatusActionRequired
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	agentMgr := &mockAgentManager{isAgentRunningFn: func(context.Context, string) bool { return false }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	response, err := svc.launchResume(ctx, &LaunchSessionRequest{
		TaskID: "task1", SessionID: "session1", RecoveryAction: recoveryActionResume,
	})
	var blocked *sessionOpenRecoveryBlockedError
	if !errors.As(err, &blocked) || response != nil {
		t.Fatalf("explicit recovery = %#v, %v; want the dynamic-route block as an error", response, err)
	}

	passive, err := svc.launchResume(ctx, &LaunchSessionRequest{TaskID: "task1", SessionID: "session1"})
	if err != nil || passive == nil || !passive.Success || passive.ActivationReason != autoResumeBlockedDynamicRoute {
		t.Fatalf("passive resume = %#v, %v; want the existing waiting response", passive, err)
	}
}

// A transient activation-write failure leaves the claimed route "starting"
// behind a healthy, prompt-ready runtime. After the store recovers, the
// ordinary resume admission retries the same-generation activation instead of
// leaving the route blocked as pending, and it neither stops nor replaces the
// live runtime.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestResumeAdmissionCompletesRecoverableStartingRoute(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-activation-recovery"
		sessionID   = "session-dynamic-activation-recovery"
		executionID = "execution-dynamic-activation-recovery"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		ExecutionProfileID: "candidate-1", Status: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	persistence := &failingRouteActivationRepo{Repository: repo}
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(persistence))
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunningFn: func(context.Context, string) bool { return true }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)

	// The first activation write fails while the runtime stays healthy.
	persistence.failActivation = true
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if err := svc.activateResumedDynamicRoute(ctx, session); err == nil {
		t.Fatal("activation reported success while the durable route write failed")
	}

	// The store recovers; the ordinary resume admission must settle the same
	// generation instead of blocking it as pending.
	persistence.failActivation = false
	admission, err := svc.loadResumeSessionCandidate(ctx, taskID, sessionID, executor.ResumeOptions{}, nil)
	if err != nil {
		t.Fatalf("loadResumeSessionCandidate after store recovery: %v", err)
	}
	if admission == nil || admission.session.RouteState != dynamicRouteStatusActive {
		t.Fatalf("admitted session route = %#v, want active", admission)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActive {
		t.Fatalf("route state after recovery = %#v, want active generation %d", state, decision.Generation)
	}
}
