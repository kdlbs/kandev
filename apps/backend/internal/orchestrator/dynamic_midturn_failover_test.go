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

const midTurnTaskDescription = "continue the mid-turn failover task"

var midTurnProfile = dynamicruntime.Profile{
	ID: "dynamic-midturn", Version: 1,
	Candidates: []dynamicruntime.Candidate{
		{ID: "candidate-1", Enabled: true},
		{ID: "candidate-2", Enabled: true},
	},
}

// recordingContinuations keeps every continuation the conductor persisted;
// a later action_required write rewrites the durable row, so the test reads
// the handoff package from the write itself.
type recordingContinuations struct {
	*sqliterepo.Repository
	mu      sync.Mutex
	records []dynamicruntime.ContinuationRecord
}

func (r *recordingContinuations) SaveRouteContinuation(ctx context.Context, record dynamicruntime.ContinuationRecord) error {
	r.mu.Lock()
	r.records = append(r.records, record)
	r.mu.Unlock()
	return r.Repository.SaveRouteContinuation(ctx, record)
}

type midTurnFixture struct {
	t       *testing.T
	ctx     context.Context
	repo    *sqliterepo.Repository
	saved   *recordingContinuations
	svc     *Service
	engine  *dynamicruntime.Engine
	taskID  string
	session string
	// execution is the current attempt's execution; the session row only
	// projects it from executors_running.
	execution string
	mu        sync.Mutex
	requests  []RouteActionRequest
	auto      []bool
	// launch, when set, runs the real successor launcher after selection.
	launch    bool
	launchErr error
}

func newMidTurnFixture(t *testing.T, name string, enabled bool) *midTurnFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	workspaceID, workflowID := "ws-"+name, "wf-"+name
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: name, DynamicMidTurnFailoverEnabled: enabled, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: workflowID, WorkspaceID: workspaceID, Name: name, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	f := &midTurnFixture{t: t, ctx: ctx, repo: repo, taskID: "task-" + name, session: "session-" + name}
	seedTaskAndSessionInWorkspace(t, repo, workspaceID, workflowID, f.taskID, f.session, "")
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[f.taskID] = &v1.Task{ID: f.taskID, State: v1.TaskStateInProgress, Description: midTurnTaskDescription}
	f.svc = createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	f.saved = &recordingContinuations{Repository: repo}
	f.engine = dynamicruntime.NewEngine(dynamicruntime.WithPersistence(f.saved))
	f.svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, f.engine, true))
	f.svc.SetRouteActionHandler(f.handleRouteAction)
	f.claimActiveGeneration("", 0, "execution-"+name+"-1")
	return f
}

// claimActiveGeneration selects the next candidate, activates it, and
// projects it onto a settled (WAITING_FOR_INPUT) session row.
func (f *midTurnFixture) claimActiveGeneration(exclude string, expected int64, executionID string) dynamicruntime.RouteDecision {
	f.t.Helper()
	decision, err := f.engine.Select(f.session, midTurnProfile, expected, exclude)
	if err != nil {
		f.t.Fatalf("Select: %v", err)
	}
	if err := f.engine.MarkActive(f.ctx, f.session, decision.Generation); err != nil {
		f.t.Fatalf("MarkActive: %v", err)
	}
	f.project(decision, executionID)
	return decision
}

func (f *midTurnFixture) project(decision dynamicruntime.RouteDecision, executionID string) {
	f.t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	if err != nil {
		f.t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = midTurnProfile.ID
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = "active"
	f.execution = executionID
	session.State = models.TaskSessionStateWaitingForInput
	if err := f.repo.UpdateTaskSession(f.ctx, session); err != nil {
		f.t.Fatalf("UpdateTaskSession: %v", err)
	}
}

// handleRouteAction stands in for the backend route-action transaction:
// the same generation-fenced Try next selection, persisted on the session.
func (f *midTurnFixture) handleRouteAction(ctx context.Context, request RouteActionRequest) (*RouteActionResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.auto = append(f.auto, automaticRouteActionFromContext(ctx))
	f.mu.Unlock()
	session, err := f.repo.GetTaskSession(ctx, request.SessionID)
	if err != nil {
		return nil, err
	}
	decision, err := f.engine.SelectContextWithReason(
		ctx, request.SessionID, midTurnProfile, request.ExpectedGeneration,
		session.ExecutionProfileID, agentruntime.RouteReasonAutoProviderFailover,
	)
	if err != nil {
		return nil, err
	}
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = decision.Status
	if err := f.repo.UpdateTaskSession(ctx, session); err != nil {
		return nil, err
	}
	if f.launch {
		f.launchErr = f.svc.LaunchDynamicRouteAction(ctx, request.SessionID)
	}
	return &RouteActionResult{SessionID: session.ID, RouteGeneration: decision.Generation, State: decision.Status}, nil
}

// fail delivers a failure for the session's current execution after the
// attempt observed output, then waits for any detached handoff.
func (f *midTurnFixture) fail(classified *routingerr.Error) {
	f.t.Helper()
	f.observeOutput()
	f.deliver(f.execution, classified)
}

func (f *midTurnFixture) observeOutput() {
	f.svc.beginDynamicAttempt(f.session)
	f.svc.bindDynamicAttemptExecution(f.session, f.execution)
	f.svc.observePromptAttempt(f.session, f.execution, 1, true, false)
}

func (f *midTurnFixture) deliver(executionID string, classified *routingerr.Error) {
	f.t.Helper()
	if f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: executionID, PromptGeneration: 1,
	}, classified) {
		f.t.Fatal("a mid-turn failure must keep the ordinary decline")
	}
	f.svc.dynamicSuccessorWorkers.Wait()
}

func (f *midTurnFixture) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *midTurnFixture) routeState() *dynamicruntime.RouteState {
	f.t.Helper()
	state, err := f.repo.LoadRouteState(f.ctx, f.session)
	if err != nil || state == nil {
		f.t.Fatalf("LoadRouteState: %v %#v", err, state)
	}
	return state
}

func rateLimited() *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
		FallbackAllowed: true, AutoRetryable: true,
	}
}

func TestMidTurnFailover_DisabledWorkspaceKeepsManualRecovery(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-off", false)
	f.fail(rateLimited())
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 while the workspace setting is off", got)
	}
	state := f.routeState()
	if state.Generation != 1 || state.ExecutionProfileID != "candidate-1" {
		t.Fatalf("route = gen %d %s, want unchanged gen 1 candidate-1", state.Generation, state.ExecutionProfileID)
	}
}

func TestMidTurnFailover_EnabledHandsOffOnceWithContinuation(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-on", true)
	f.launch = true
	f.fail(rateLimited())

	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want exactly 1", got)
	}
	request := f.requests[0]
	if request.Action != RouteActionTryNext || !request.Automatic || request.ExpectedGeneration != 1 || !f.auto[0] {
		t.Fatalf("request = %#v automatic ctx=%v, want automatic try_next for generation 1", request, f.auto[0])
	}
	state := f.routeState()
	if state.Generation != 2 || state.ExecutionProfileID != "candidate-2" {
		t.Fatalf("route = gen %d %s, want gen 2 candidate-2", state.Generation, state.ExecutionProfileID)
	}
	f.saved.mu.Lock()
	defer f.saved.mu.Unlock()
	if len(f.saved.records) != 1 {
		t.Fatalf("persisted continuations = %d (launch err %v), want 1", len(f.saved.records), f.launchErr)
	}
	continuation := f.saved.records[0]
	if continuation.Generation != 2 || continuation.Continuation.TaskDescription != midTurnTaskDescription ||
		continuation.Continuation.Conversation != "" || continuation.Continuation.ToolSummary != "" {
		t.Fatalf("continuation = %#v, want the bounded gen-2 package without the failed attempt's output", continuation)
	}
}

func TestMidTurnFailover_DuplicateDeliveryHandsOffOnce(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-dup", true)
	f.fail(rateLimited())
	// The same failure delivered again names the predecessor execution.
	f.observeOutput()
	f.deliver(f.execution, rateLimited())
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want 1 for a twice-delivered failure", got)
	}
}

func TestMidTurnFailover_RepeatedSameFailureStops(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-repeat", true)
	f.fail(rateLimited()) // candidate-1 -> candidate-2 (gen 2)
	f.project(dynamicruntime.RouteDecision{ExecutionProfileID: "candidate-2", Generation: 2}, "execution-repeat-2")
	if err := f.engine.MarkActive(f.ctx, f.session, 2); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	f.fail(rateLimited()) // candidate-2 -> candidate-1 (gen 3)
	f.project(dynamicruntime.RouteDecision{ExecutionProfileID: "candidate-1", Generation: 3}, "execution-repeat-3")
	if err := f.engine.MarkActive(f.ctx, f.session, 3); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	f.fail(rateLimited()) // candidate-1 again with the same code: stop

	if got := f.requestCount(); got != 2 {
		t.Fatalf("route actions = %d, want 2 before the repeated failure stops the chain", got)
	}
	state := f.routeState()
	if state.Generation != 3 || state.ExecutionProfileID != "candidate-1" {
		t.Fatalf("route = gen %d %s, want gen 3 candidate-1 left for manual recovery", state.Generation, state.ExecutionProfileID)
	}
}

func TestMidTurnFailover_CompletedTurnStartsNewChain(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-reset", true)
	f.fail(rateLimited())
	f.project(dynamicruntime.RouteDecision{ExecutionProfileID: "candidate-2", Generation: 2}, "execution-reset-2")
	if err := f.engine.MarkActive(f.ctx, f.session, 2); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	f.svc.resetMidTurnFailoverChain(f.session)
	f.fail(rateLimited())
	f.project(dynamicruntime.RouteDecision{ExecutionProfileID: "candidate-1", Generation: 3}, "execution-reset-3")
	if err := f.engine.MarkActive(f.ctx, f.session, 3); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	f.svc.resetMidTurnFailoverChain(f.session)
	f.fail(rateLimited())
	if got := f.requestCount(); got != 3 {
		t.Fatalf("route actions = %d, want 3 when a completed turn separates the failures", got)
	}
}

func TestMidTurnFailover_IneligibleFailuresKeepManualRecovery(t *testing.T) {
	cases := map[string]*routingerr.Error{
		"unclassified": {Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified, FallbackAllowed: true},
		"user":         {Code: routingerr.CodePermissionDeniedByUser, Class: routingerr.ClassUnclassified},
		"runtime":      {Code: routingerr.CodeAgentRuntime, Class: routingerr.ClassUnclassified},
		"transport":    {Code: routingerr.CodeAgentTransportLost, Class: routingerr.ClassTransient, AutoRetryable: true},
		"auth":         {Code: routingerr.CodeAuthRequired, Class: routingerr.ClassHard, FallbackAllowed: true, UserAction: true},
		"account":      {Code: routingerr.CodeAccountBlocked, Class: routingerr.ClassHard, FallbackAllowed: true},
	}
	for name, classified := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMidTurnFixture(t, "midturn-inelig-"+name, true)
			f.fail(classified)
			if got := f.requestCount(); got != 0 {
				t.Fatalf("route actions = %d, want 0 for %s", got, classified.Code)
			}
		})
	}
}

func TestMidTurnFailover_PreResultUnknownEvidenceDoesNotHandOff(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-unknown", true)
	// No attempt record: evidence is unknown and must fail closed.
	f.deliver(f.execution, rateLimited())
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 without known output evidence", got)
	}
}

func TestMidTurnFailover_StaleGenerationDoesNothing(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-stale", true)
	// Hold the route-action lock so the detached handoff waits, then move
	// the route as a concurrent manual action would.
	release := f.svc.acquireRouteActionOperationLock(f.session)
	f.observeOutput()
	if f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: f.execution, PromptGeneration: 1,
	}, rateLimited()) {
		t.Fatal("a mid-turn failure must keep the ordinary decline")
	}
	f.claimActiveGeneration("candidate-1", 1, "execution-stale-2")
	release()
	f.svc.dynamicSuccessorWorkers.Wait()

	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 after the route moved past the failed generation", got)
	}
	if state := f.routeState(); state.Generation != 2 {
		t.Fatalf("route generation = %d, want 2 (the concurrent selection)", state.Generation)
	}
}

func TestMidTurnFailover_FailureFromSupersededExecutionIsIgnored(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-superseded", true)
	f.deliver("execution-from-an-older-generation", rateLimited())
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 for a superseded execution", got)
	}
}

func TestMidTurnProviderCapacityFailureCodes(t *testing.T) {
	included := []routingerr.Code{
		routingerr.CodeRateLimited, routingerr.CodeQuotaLimited, routingerr.CodeProviderUnavailable,
		routingerr.CodeProviderOverloaded, routingerr.CodeModelCapacity,
	}
	for _, code := range included {
		if !midTurnProviderCapacityFailure(&routingerr.Error{Code: code, FallbackAllowed: true}) {
			t.Fatalf("%s should qualify", code)
		}
		if midTurnProviderCapacityFailure(&routingerr.Error{Code: code}) {
			t.Fatalf("%s without fallback permission should not qualify", code)
		}
	}
	excluded := []routingerr.Code{
		routingerr.CodeNetworkUnavailable, routingerr.CodeAgentTransportLost, routingerr.CodeProviderResourceExhausted,
		routingerr.CodeAccountBlocked, routingerr.CodeAuthRequired, routingerr.CodeUnknownProvider, routingerr.CodeTask,
	}
	for _, code := range excluded {
		if midTurnProviderCapacityFailure(&routingerr.Error{Code: code, FallbackAllowed: true}) {
			t.Fatalf("%s should not qualify", code)
		}
	}
}

func TestApplyAutomaticRouteActionRejectsInadmissibleTrigger(t *testing.T) {
	svc := &Service{}
	called := false
	svc.SetRouteActionHandler(func(context.Context, RouteActionRequest) (*RouteActionResult, error) {
		called = true
		return &RouteActionResult{}, nil
	})
	_, err := svc.applyAutomaticRouteAction(context.Background(), RouteActionRequest{SessionID: "s"},
		func(context.Context) bool { return false })
	if !errors.Is(err, errMidTurnFailoverNotAdmissible) || called {
		t.Fatalf("err = %v called = %v, want inadmissible without calling the handler", err, called)
	}
}

func TestMidTurnFailover_FailureOfAnotherPromptAttemptDoesNothing(t *testing.T) {
	f := newMidTurnFixture(t, "midturn-other-prompt", true)
	f.observeOutput()
	if f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: f.execution, PromptGeneration: 2,
	}, rateLimited()) {
		t.Fatal("a mid-turn failure must keep the ordinary decline")
	}
	f.svc.dynamicSuccessorWorkers.Wait()
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 for a failure of a different prompt generation", got)
	}
}
