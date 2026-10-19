package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	failedRouteProfile = "dynamic-failed-route"
	failedRouteA       = "candidate-a"
	failedRouteB       = "candidate-b"
)

// failedRouteFixture runs a dynamic task session whose candidates try the
// next candidate on a rate limit, against durable SQLite route state and an
// injectable engine and circuit clock.
type failedRouteFixture struct {
	t         *testing.T
	ctx       context.Context
	repo      *sqliterepo.Repository
	svc       *Service
	resolver  *agentruntime.ProfileExecutionResolver
	circuits  *dynamicruntime.CircuitRegistry
	clockMu   sync.Mutex
	now       time.Time
	workspace string
	taskID    string
	session   string
	mu        sync.Mutex
	requests  []RouteActionRequest
}

type failedRouteSettings struct {
	autoRetry bool
	max       int
	midTurn   bool
}

func newFailedRouteFixture(t *testing.T, name string, settings failedRouteSettings) *failedRouteFixture {
	t.Helper()
	f := &failedRouteFixture{
		t: t, ctx: context.Background(), repo: setupTestRepo(t), now: time.Now().UTC(),
		workspace: "ws-" + name, taskID: "task-" + name, session: "session-" + name,
	}
	now := f.now
	if err := f.repo.CreateWorkspace(f.ctx, &models.Workspace{
		ID: f.workspace, Name: name, CreatedAt: now, UpdatedAt: now,
		DynamicFailedRouteAutoRetryEnabled: settings.autoRetry, DynamicFailedRouteAutoRetryMax: settings.max,
		DynamicMidTurnFailoverEnabled: settings.midTurn,
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workflowID := "wf-" + name
	if err := f.repo.CreateWorkflow(f.ctx, &models.Workflow{
		ID: workflowID, WorkspaceID: f.workspace, Name: name, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	seedTaskAndSessionInWorkspace(t, f.repo, f.workspace, workflowID, f.taskID, f.session, "")
	f.svc = f.newService()
	f.claimInitialRoute()
	return f
}

// newService builds a fresh service and engine over the fixture's durable
// repository, as a backend restart would.
func (f *failedRouteFixture) newService() *Service {
	f.t.Helper()
	clock := func() time.Time {
		f.clockMu.Lock()
		defer f.clockMu.Unlock()
		return f.now
	}
	rules := `{"rate_limited":"try_next"}`
	f.circuits = dynamicruntime.NewCircuitRegistry()
	f.resolver = newWorkflowDynamicProfileResolverWithCandidates(f.t, failedRouteProfile,
		[]workflowDynamicCandidate{
			{executionProfileID: failedRouteA, enabled: true, rulesJSON: rules},
			{executionProfileID: failedRouteB, enabled: true, rulesJSON: rules},
		},
		dynamicruntime.WithCircuitRegistry(f.circuits),
		dynamicruntime.WithClock(clock),
		dynamicruntime.WithPersistence(f.repo),
		dynamicruntime.WithStateLoader(f.repo),
	)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[f.taskID] = &v1.Task{ID: f.taskID, State: v1.TaskStateInProgress, Description: "failed route task"}
	svc := createTestServiceWithScheduler(f.repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.SetProfileExecutionResolver(f.resolver)
	svc.SetRouteActionHandler(f.handleRouteAction)
	svc.startDynamicPolicyRecovery(f.ctx)
	f.t.Cleanup(svc.stopDynamicPolicyRecovery)
	return svc
}

func (f *failedRouteFixture) claimInitialRoute() {
	f.t.Helper()
	execution, err := f.resolver.Resolve(f.ctx, f.session, failedRouteProfile, 0, "")
	if err != nil {
		f.t.Fatalf("Resolve: %v", err)
	}
	f.activate(execution.ExecutionProfileID, execution.Generation)
}

// activate marks the claimed generation active and settles the session.
func (f *failedRouteFixture) activate(executionProfileID string, generation int64) {
	f.t.Helper()
	if err := f.resolver.MarkRouteActive(f.ctx, f.session, generation); err != nil {
		f.t.Fatalf("MarkRouteActive: %v", err)
	}
	f.updateSession(func(session *models.TaskSession) {
		session.AgentProfileID = failedRouteProfile
		session.ExecutionProfileID = executionProfileID
		session.RouteGeneration = generation
		session.RouteState = "active"
		session.State = models.TaskSessionStateWaitingForInput
	})
}

func (f *failedRouteFixture) updateSession(mutate func(*models.TaskSession)) {
	f.t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	if err != nil {
		f.t.Fatalf("GetTaskSession: %v", err)
	}
	mutate(session)
	if err := f.repo.UpdateTaskSession(f.ctx, session); err != nil {
		f.t.Fatalf("UpdateTaskSession: %v", err)
	}
}

func (f *failedRouteFixture) advance(to time.Time) {
	f.clockMu.Lock()
	f.now = to
	f.clockMu.Unlock()
}

func (f *failedRouteFixture) openCircuit(candidate string, until time.Time) {
	f.t.Helper()
	execution, err := f.resolver.LoadDynamicProfile(f.ctx, failedRouteProfile)
	if err != nil {
		f.t.Fatalf("LoadDynamicProfile: %v", err)
	}
	for _, c := range execution.Candidates {
		if c.ID == candidate {
			f.circuits.Open(c.BindingKey, until, routingerr.CodeRateLimited)
			return
		}
	}
	f.t.Fatalf("candidate %s not found", candidate)
}

// failPreResult delivers a classified failure of the current candidate that
// produced no output or effects, then settles the turn as the ordinary
// recoverable-failure path does.
func (f *failedRouteFixture) failPreResult(classified *routingerr.Error) {
	f.t.Helper()
	session := f.currentSession()
	f.svc.beginDynamicAttempt(f.session)
	f.svc.bindDynamicAttemptExecution(f.session, "execution-"+session.ExecutionProfileID)
	f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: "execution-" + session.ExecutionProfileID,
		ExecutionProfileID: session.ExecutionProfileID, PromptGeneration: 1,
	}, classified)
	f.updateSession(func(session *models.TaskSession) { session.State = models.TaskSessionStateWaitingForInput })
}

func (f *failedRouteFixture) currentSession() *models.TaskSession {
	f.t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	if err != nil {
		f.t.Fatalf("GetTaskSession: %v", err)
	}
	return session
}

func (f *failedRouteFixture) routeState() *dynamicruntime.RouteState {
	f.t.Helper()
	state, err := f.repo.LoadRouteState(f.ctx, f.session)
	if err != nil || state == nil {
		f.t.Fatalf("LoadRouteState: %v %#v", err, state)
	}
	return state
}

func (f *failedRouteFixture) pending() (dynamicruntime.PendingFailedRouteRetry, bool) {
	return dynamicruntime.FailedRouteRetryPending(*f.routeState())
}

// fire runs the shared recovery timer's automatic retry for generation, as
// the timer would at the fixture clock's current time.
func (f *failedRouteFixture) fire(generation int64) {
	f.svc.runFailedRouteAutoRetry(f.svc.dynamicRecoveryCtx, f.session, generation)
}

func (f *failedRouteFixture) armed() bool {
	f.svc.dynamicRecoveryMu.Lock()
	defer f.svc.dynamicRecoveryMu.Unlock()
	return f.svc.dynamicRecoveryTimers[f.session] != nil
}

// armedGeneration reports the route generation of the session's recovery
// timer, or zero when no timer is armed.
func (f *failedRouteFixture) armedGeneration() int64 {
	f.svc.dynamicRecoveryMu.Lock()
	defer f.svc.dynamicRecoveryMu.Unlock()
	entry := f.svc.dynamicRecoveryTimers[f.session]
	if entry == nil {
		return 0
	}
	return entry.generation
}

func (f *failedRouteFixture) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// handleRouteAction stands in for the backend route-action transaction: the
// same resolver action the backend maps the request to, persisted on the
// session projection. The successor launch is out of scope here.
func (f *failedRouteFixture) handleRouteAction(ctx context.Context, request RouteActionRequest) (*RouteActionResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	session, err := f.repo.GetTaskSession(ctx, request.SessionID)
	if err != nil {
		return nil, err
	}
	action := string(request.Action)
	if request.Automatic && request.Action == RouteActionRetry {
		action = agentruntime.RouteActionAutoRetry
	} else if request.Automatic && request.Action == RouteActionTryNext {
		action = agentruntime.RouteActionAutoTryNext
	}
	execution, err := f.resolver.ResolveRouteAction(
		ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID, request.ExpectedGeneration, action,
	)
	var noCandidate *dynamicruntime.NoEligibleCandidateError
	switch {
	case errors.As(err, &noCandidate):
		session.RouteGeneration, session.RouteState, session.RouteReason = noCandidate.Generation, "waiting", "no_eligible_candidate"
	case err != nil:
		return nil, err
	default:
		session.ExecutionProfileID = execution.ExecutionProfileID
		session.RouteGeneration = execution.Generation
		session.RouteState = execution.Decision.Status
		session.RouteReason = execution.Decision.Reason
	}
	if err := f.repo.UpdateTaskSession(ctx, session); err != nil {
		return nil, err
	}
	return &RouteActionResult{
		SessionID: session.ID, RouteGeneration: session.RouteGeneration, State: session.RouteState, Reason: session.RouteReason,
	}, nil
}

func rateLimitedUntil(reset time.Time) *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
		FallbackAllowed: true, AutoRetryable: true, ResetHint: &reset,
	}
}
