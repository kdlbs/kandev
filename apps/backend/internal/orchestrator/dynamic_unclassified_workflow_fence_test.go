package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestUnclassifiedWorkflowContextFencedAtRouteClaim(t *testing.T) {
	ctx := context.Background()
	fixture := newUnclassifiedWorkflowFenceFixture(t, true, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()

	if handled := fixture.svc.routeDynamicAgentFailure(ctx, fixture.event("execution-one", 1), fixture.failure()); handled {
		t.Fatal("first matching failure selected a successor below threshold")
	}
	fixture.resume(t, "execution-two")
	data := fixture.event("execution-two", 2)
	fixture.barrier.decisionEntered = make(chan struct{})
	fixture.barrier.releaseDecision = make(chan struct{})

	done := make(chan bool, 1)
	go func() {
		done <- fixture.svc.routeDynamicAgentFailure(ctx, data, fixture.failure())
	}()
	select {
	case <-fixture.barrier.decisionEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("route claim did not reach the deterministic workflow-context barrier")
	}
	fixture.moveToVetoedStep(t)
	close(fixture.barrier.releaseDecision)
	if handled := <-done; handled {
		t.Fatal("changed workflow context claimed and launched a successor")
	}

	routeState, err := fixture.repo.LoadRouteState(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState.Generation != 1 || routeState.ExecutionProfileID != "candidate-a" || routeState.Status != "action_required" {
		t.Fatalf("route state after stale context = %+v, want candidate-a generation 1 action_required", routeState)
	}
	var policyState dynamicruntime.PolicyState
	if routeState.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(routeState.PolicyStateJSON), &policyState); err != nil {
			t.Fatalf("decode policy state: %v", err)
		}
	}
	if policyState.Unclassified != nil {
		t.Fatalf("stale workflow context retained failure streak: %+v", policyState.Unclassified)
	}
	if len(fixture.agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launches after route-claim veto = %d, want none", len(fixture.agentManager.startAgentProcessCalls))
	}
}

func TestUnclassifiedWorkflowStepRevisionFencedAtRouteClaim(t *testing.T) {
	ctx := context.Background()
	fixture := newUnclassifiedWorkflowFenceFixture(t, true, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()

	if handled := fixture.svc.routeDynamicAgentFailure(ctx, fixture.event("execution-one", 1), fixture.failure()); handled {
		t.Fatal("first matching failure selected a successor below threshold")
	}
	fixture.resume(t, "execution-two")
	data := fixture.event("execution-two", 2)
	done := make(chan bool, 1)
	go func() {
		done <- fixture.svc.routeDynamicAgentFailure(ctx, data, fixture.failure())
	}()
	select {
	case <-fixture.barrier.decisionEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("route claim did not reach the deterministic workflow-revision barrier")
	}
	step, err := fixture.workflow.GetStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("GetStep before revision change: %v", err)
	}
	step.Name = "Edited while fallback was being evaluated"
	if err := fixture.workflow.UpdateStep(ctx, step); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	close(fixture.barrier.releaseDecision)
	if handled := <-done; handled {
		t.Fatal("changed workflow-step revision claimed and launched a successor")
	}
	routeState, err := fixture.repo.LoadRouteState(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState.Generation != 1 || routeState.ExecutionProfileID != "candidate-a" || routeState.Status != "action_required" {
		t.Fatalf("route state after step revision change = %+v, want candidate-a generation 1 action_required", routeState)
	}
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(routeState.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode policy state: %v", err)
	}
	if policyState.Unclassified != nil {
		t.Fatalf("changed workflow-step revision retained failure streak: %+v", policyState.Unclassified)
	}
	if len(fixture.agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launches after step revision change = %d, want none", len(fixture.agentManager.startAgentProcessCalls))
	}
}

func TestUnclassifiedWorkflowContextFencedBeforeDetachedLaunch(t *testing.T) {
	ctx := context.Background()
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, true)
	defer fixture.svc.stopDynamicSuccessorWorkers()

	if handled := fixture.svc.routeDynamicAgentFailure(ctx, fixture.event("execution-one", 1), fixture.failure()); handled {
		t.Fatal("first matching failure selected a successor below threshold")
	}
	fixture.resume(t, "execution-two")
	fixture.barrier.launchEntered = make(chan struct{})
	fixture.barrier.releaseLaunch = make(chan struct{})
	if handled := fixture.svc.routeDynamicAgentFailure(ctx, fixture.event("execution-two", 2), fixture.failure()); !handled {
		t.Fatal("threshold failure did not claim the detached successor route")
	}
	select {
	case <-fixture.barrier.launchEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("detached launch did not reach the deterministic workflow-context barrier")
	}
	fixture.setWorkflowVeto(t, true)
	close(fixture.barrier.releaseLaunch)
	waitForDynamicSuccessorWorkers(t, fixture.svc)

	routeState, err := fixture.repo.LoadRouteState(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState.Generation != 2 || routeState.ExecutionProfileID != "candidate-b" || routeState.Status != "action_required" {
		t.Fatalf("route state after launch-time veto = %+v, want candidate-b generation 2 action_required", routeState)
	}
	var policyState dynamicruntime.PolicyState
	if routeState.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(routeState.PolicyStateJSON), &policyState); err != nil {
			t.Fatalf("decode policy state: %v", err)
		}
	}
	if policyState.Unclassified != nil {
		t.Fatalf("launch-time veto retained failure streak: %+v", policyState.Unclassified)
	}
	session := mustTaskSession(t, fixture.repo, ctx, fixture.sessionID)
	if session.State != models.TaskSessionStateWaitingForInput || session.RouteState != "action_required" {
		t.Fatalf("session after launch-time veto = state %q route %q, want waiting_for_input/action_required", session.State, session.RouteState)
	}
	if len(fixture.agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launches after launch-time veto = %d, want none", len(fixture.agentManager.startAgentProcessCalls))
	}
}

type unclassifiedWorkflowFenceFixture struct {
	ctx          context.Context
	repo         *sqliterepo.Repository
	workflow     *workflowrepo.Repository
	barrier      *barrierUnclassifiedRoutePersistence
	svc          *Service
	resolver     *agentruntime.ProfileExecutionResolver
	agentManager *mockAgentManager
	taskID       string
	sessionID    string
	profile      dynamicruntime.Profile
}

type barrierUnclassifiedRoutePersistence struct {
	*sqliterepo.Repository
	decisionEntered chan struct{}
	releaseDecision chan struct{}
	launchEntered   chan struct{}
	releaseLaunch   chan struct{}
	decisionOnce    sync.Once
	launchOnce      sync.Once
}

func (p *barrierUnclassifiedRoutePersistence) RecordUnclassifiedRouteDecision(
	ctx context.Context,
	decision dynamicruntime.RouteDecision,
	state dynamicruntime.RouteState,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	if p.decisionEntered != nil {
		p.decisionOnce.Do(func() { close(p.decisionEntered) })
		<-p.releaseDecision
	}
	return p.Repository.RecordUnclassifiedRouteDecision(ctx, decision, state, evidence)
}

func (p *barrierUnclassifiedRoutePersistence) ClaimUnclassifiedFallbackLaunch(
	ctx context.Context,
	decision dynamicruntime.RouteDecision,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	if p.launchEntered != nil {
		p.launchOnce.Do(func() { close(p.launchEntered) })
		<-p.releaseLaunch
	}
	return p.Repository.ClaimUnclassifiedFallbackLaunch(ctx, decision, evidence)
}

func newUnclassifiedWorkflowFenceFixture(
	t *testing.T,
	barrierDecision bool,
	barrierLaunch bool,
) unclassifiedWorkflowFenceFixture {
	t.Helper()
	ctx := context.Background()
	const (
		taskID    = "task-unclassified-workflow-fence"
		sessionID = "session-unclassified-workflow-fence"
		profileID = "dynamic-unclassified"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "workflow-fence.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("provide task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	workflow, err := workflowrepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}
	seedSession(t, repo, taskID, sessionID, "step-1")
	if err := workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Work"}); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if err := workflow.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf1", Name: "Vetoed", DisableUnclassifiedFallback: true,
	}); err != nil {
		t.Fatalf("CreateStep for vetoed destination: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET state = ? WHERE id = ?`, models.TaskSessionStateRunning, sessionID); err != nil {
		t.Fatalf("set running session: %v", err)
	}
	barrier := &barrierUnclassifiedRoutePersistence{Repository: repo}
	if barrierDecision {
		barrier.decisionEntered = make(chan struct{})
		barrier.releaseDecision = make(chan struct{})
	}
	if barrierLaunch {
		barrier.launchEntered = make(chan struct{})
		barrier.releaseLaunch = make(chan struct{})
	}
	profile := unclassifiedAdmissionProfile(2)
	policyJSON, err := json.Marshal(profile.Candidates[0].Policies)
	if err != nil {
		t.Fatalf("marshal candidate policy: %v", err)
	}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, profileID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-a", enabled: true, rulesJSON: string(policyJSON)},
		{executionProfileID: "candidate-b", enabled: true},
	}, dynamicruntime.WithPersistence(barrier), dynamicruntime.WithStateLoader(barrier))
	profile.ID = profileID
	_, initial := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, "execution-one")
	if initial.Generation != 1 {
		t.Fatalf("initial route generation = %d, want 1", initial.Generation)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{}
	stepGetter := newMockStepGetter()
	initialStep, err := workflow.GetStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("GetStep for evidence: %v", err)
	}
	stepGetter.steps["step-1"] = initialStep
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	svc.SetProfileExecutionResolver(resolver)
	return unclassifiedWorkflowFenceFixture{
		ctx: ctx, repo: repo, workflow: workflow, barrier: barrier,
		svc: svc, resolver: resolver, agentManager: agentManager,
		taskID: taskID, sessionID: sessionID, profile: profile,
	}
}

func (f unclassifiedWorkflowFenceFixture) failure() *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified,
		Phase: routingerr.PhasePromptSend,
	}
}

func (f unclassifiedWorkflowFenceFixture) event(executionID string, generation uint64) watcher.AgentEventData {
	f.failPrompt(executionID, generation)
	return watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.sessionID, OwnerKind: "task", AgentExecutionID: executionID,
		PromptGeneration: generation, DynamicRouteAttempt: true, EvidenceKnown: true,
		ProviderError: &streams.ProviderError{
			Source: streams.ProviderErrorSourceACPPrompt, ProviderID: "provider-x",
			Message: "provider returned an unsupported terminal response", DiagnosticIdentityComplete: true,
			OccurredAt: time.Now().UTC(),
		},
	}
}

func (f unclassifiedWorkflowFenceFixture) failPrompt(executionID string, generation uint64) {
	f.svc.beginPromptAttempt(f.sessionID, executionID, generation, true)
}

func (f unclassifiedWorkflowFenceFixture) resume(t *testing.T, executionID string) {
	t.Helper()
	// The first failure is manually action-required. A retry reclaims the
	// same route generation as retrying while preserving the durable streak.
	state, err := f.repo.LoadRouteState(f.ctx, f.sessionID)
	if err != nil || state == nil {
		t.Fatalf("LoadRouteState before retry: %+v, %v", state, err)
	}
	resumed, err := f.resolver.ResolveRouteAction(
		f.ctx, f.sessionID, f.profile.ID, "candidate-a", state.Generation, "retry",
	)
	if err != nil {
		t.Fatalf("ResolveRouteAction: %v", err)
	}
	session := mustTaskSession(t, f.repo, f.ctx, f.sessionID)
	session.AgentExecutionID = executionID
	session.ExecutionProfileID = resumed.Decision.ExecutionProfileID
	session.RouteGeneration = resumed.Generation
	session.RouteState = resumed.Decision.Status
	session.State = models.TaskSessionStateRunning
	if err := f.repo.UpdateTaskSession(f.ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession retry: %v", err)
	}
}

func (f unclassifiedWorkflowFenceFixture) setWorkflowVeto(t *testing.T, disabled bool) {
	t.Helper()
	step, err := f.workflow.GetStep(f.ctx, "step-1")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	step.DisableUnclassifiedFallback = disabled
	if err := f.workflow.UpdateStep(f.ctx, step); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
}

func (f unclassifiedWorkflowFenceFixture) moveToVetoedStep(t *testing.T) {
	t.Helper()
	if err := f.repo.AddTaskToWorkflow(f.ctx, f.taskID, "wf1", "step-2", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow into vetoed step: %v", err)
	}
}

func waitForDynamicSuccessorWorkers(t *testing.T, svc *Service) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		svc.dynamicSuccessorWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for detached dynamic successor")
	}
}

func TestUnclassifiedStartupAdmission(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-unclassified-startup"
		sessionID   = "session-unclassified-startup"
		executionID = "execution-unclassified-startup"
	)
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	profile := unclassifiedAdmissionProfile(2)
	engine, decision := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, executionID)
	startupErr := routingerr.NewAgentStartupFailure(
		routingerr.PhaseSessionInit, "provider-x", errors.New("agent session initialization failed"),
	)
	var startup *routingerr.AgentStartupFailure
	if !errors.As(startupErr, &startup) {
		t.Fatalf("trusted startup producer returned %T, want AgentStartupFailure", startupErr)
	}
	attempt := dynamicStartupAttempt{
		ID: "route-attempt-1", SessionID: sessionID, LogicalProfileID: profile.ID,
		ExecutionProfileID: "candidate-a", Generation: decision.Generation,
	}
	data := watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, OwnerKind: "task", AgentExecutionID: executionID,
		DynamicRouteAttempt: true, EvidenceKnown: true,
	}
	evidence := svc.unclassifiedStartupEvidence(ctx, data, mustTaskSession(t, repo, ctx, sessionID), attempt, startup)
	if !evidence.TaskScope || !evidence.StepKnown || !evidence.CurrentAttempt || !evidence.DiagnosticComplete || evidence.Phase != routingerr.PhaseSessionInit {
		t.Fatalf("trusted startup evidence = %+v", evidence)
	}
	failure := classifyTrustedAgentStartupFailure(startup)
	if failure.Code != routingerr.CodeAgentRuntime || failure.Class != routingerr.ClassUnclassified {
		t.Fatalf("startup classification = %+v, want unclassified agent_runtime", failure)
	}
	if failure.FallbackAllowed || failure.AutoRetryable {
		t.Fatalf("startup classification widened global routing flags: %+v", failure)
	}
	if _, err := engine.ApplyUnclassifiedFailureContext(ctx, sessionID, profile, decision.Generation, "candidate-a", failure, evidence); !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("trusted startup failure error = %v, want manual recovery before threshold", err)
	}

	if _, ok := dynamicStartupAttemptFromContext(context.Background()); ok {
		t.Fatal("ordinary launch context unexpectedly contained a route attempt")
	}
	if wrapped := routingerr.NewAgentStartupFailure(routingerr.PhaseProcessStart, "provider-x", context.Canceled); wrapped != context.Canceled {
		t.Fatalf("cancelled startup result = %T, want unwrapped cancellation", wrapped)
	}
}

func TestUnclassifiedFallbackScope(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		ownerKind     string
		mismatchedID  bool
		isOffice      bool
		stepID        string
		stepReadError bool
		wantTaskScope bool
		wantStepKnown bool
	}{
		{name: "task with no workflow step", ownerKind: "task", wantTaskScope: true, wantStepKnown: true},
		{name: "office owner", ownerKind: "office"},
		{name: "utility owner", ownerKind: "utility"},
		{name: "missing owner", ownerKind: ""},
		{name: "mismatched task", ownerKind: "task", mismatchedID: true},
		{name: "office task", ownerKind: "task", isOffice: true},
		{name: "workflow step read failure", ownerKind: "task", stepID: "step-1", stepReadError: true, wantTaskScope: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			taskID, sessionID := "task-scope", "session-scope"
			repo := setupTestRepo(t)
			seedSession(t, repo, taskID, sessionID, testCase.stepID)
			if testCase.isOffice {
				task, err := repo.GetTask(ctx, taskID)
				if err != nil {
					t.Fatalf("GetTask: %v", err)
				}
				task.ProjectID = "office-project"
				if err := repo.UpdateTask(ctx, task); err != nil {
					t.Fatalf("UpdateTask: %v", err)
				}
			}
			stepGetter := newMockStepGetter()
			if testCase.stepReadError {
				stepGetter.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
					return nil, errors.New("step read failed")
				}
			} else if testCase.stepID != "" {
				stepGetter.steps[testCase.stepID] = &wfmodels.WorkflowStep{ID: testCase.stepID, WorkflowID: "wf1", Name: "Work"}
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})
			session := mustTaskSession(t, repo, ctx, sessionID)
			dataTaskID := taskID
			if testCase.mismatchedID {
				dataTaskID = "another-task"
			}
			evidence := svc.unclassifiedFailureEvidence(
				ctx,
				watcher.AgentEventData{TaskID: dataTaskID, SessionID: sessionID, OwnerKind: testCase.ownerKind},
				session, true, dynamicruntime.UnclassifiedOriginTerminalProvider,
				routingerr.PhasePromptSend, "attempt-1", "provider-x", "complete diagnostic", true, true, false, false,
			)
			if evidence.TaskScope != testCase.wantTaskScope {
				t.Fatalf("TaskScope = %v, want %v: %+v", evidence.TaskScope, testCase.wantTaskScope, evidence)
			}
			if evidence.StepKnown != testCase.wantStepKnown {
				t.Fatalf("StepKnown = %v, want %v: %+v", evidence.StepKnown, testCase.wantStepKnown, evidence)
			}
		})
	}
}

func TestStopSessionClearsUnclassifiedStreak(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-unclassified-stop"
		sessionID   = "session-unclassified-stop"
		executionID = "execution-unclassified-stop"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	profile := unclassifiedAdmissionProfile(3)
	engine, decision := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, executionID)
	evidence := dynamicruntime.UnclassifiedFailureEvidence{
		TaskScope: true, TaskID: taskID, SessionID: sessionID,
		LogicalProfileID: profile.ID, ExecutionProfileID: "candidate-a", RouteGeneration: decision.Generation,
		StepKnown: true, AttemptID: "prompt:execution-unclassified-stop:1",
		Origin: dynamicruntime.UnclassifiedOriginTerminalProvider, Phase: routingerr.PhasePromptSend,
		ProviderID: "provider-x", DiagnosticText: "unsupported terminal result",
		DiagnosticComplete: true, CurrentAttempt: true, EvidenceKnown: true,
	}
	failure := &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend}
	if _, err := engine.ApplyUnclassifiedFailureContext(ctx, sessionID, profile, decision.Generation, "candidate-a", failure, evidence); !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("seed failure: %v", err)
	}
	manager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	if err := svc.StopSession(ctx, sessionID, "operator stopped", false); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode route policy state: %v", err)
	}
	if policyState.Unclassified != nil {
		t.Fatalf("unclassified streak after stop = %+v, want nil", policyState.Unclassified)
	}
}
