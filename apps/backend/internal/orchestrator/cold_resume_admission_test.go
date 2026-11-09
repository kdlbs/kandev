package orchestrator

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Regression coverage for kdlbs/kandev#5174: a reviewer's move_task_kandev
// back to a work step cold-resumes the card's own parked worker session. That
// relaunch must run executor admission and launch as the session's durable
// executor actor, exactly like every other resume entry, so profile secrets
// resolve for that user only. Before the fix the path called the plain
// executor.ResumeSession, skipped admission, and reached the runtime with no
// identity, which the secrets store treats as unscoped.
//
// The scenario drives the production turn-end handler (handleAgentReady) with
// the deferred move the MCP move_task_kandev handler persists. Executor
// admission is the real task-service PrepareExecutorAdmission, wired the way
// backendapp wires it, and reveals use a real secrets store. The identity
// resolver and the agent runtime are stand-ins.

const (
	coldResumeOwner         = "user-owner"
	coldResumeCollaborator  = "user-collaborator"
	coldResumeReviewerActor = "user-reviewer"
	coldResumeOwnerSecret   = "secret-owner"
	coldResumeCollabSecret  = "secret-collaborator"
	coldResumeFindings      = "Changes requested: F1 off-by-one in fib(0); F2 missing test for negatives."
)

type coldResumeOptions struct {
	// sessionActor launched the card's initial worker session ("" = no
	// durable attribution).
	sessionActor string
	// profileOwner owns the executor profile ("" = shared).
	profileOwner string
	// authDisabled resolves every actor to the synthetic identity, as
	// backendapp does when authentication is off.
	authDisabled bool
}

type coldResumeAdmissionCall struct {
	sessionID string
	identity  string
	err       error
}

type coldResumeRuntimeCall struct {
	name   string
	target string
	ctx    context.Context
}

type coldResumeScenario struct {
	*pendingMoveScenario
	taskSvc *taskservice.Service
	reveal  func(context.Context, string) (string, error)

	mu         sync.Mutex
	admissions []coldResumeAdmissionCall
	runtime    []coldResumeRuntimeCall
}

func coldResumeIdentity(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return "<none>"
	}
	if identity.Synthetic {
		return "<synthetic>"
	}
	return identity.UserID
}

func coldResumeSteps() map[string]*wfmodels.WorkflowStep {
	return map[string]*wfmodels.WorkflowStep{
		stepInProgressID: {
			ID: stepInProgressID, WorkflowID: "wf1", Name: "Work", Position: 0,
			Prompt:                    "Implement the task.",
			IsStartStep:               true,
			AutoAdvanceRequiresSignal: true,
			ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
			ProfileSessionEndPolicy:   models.WorkflowProfileSessionEndPolicyPark,
			SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepInReviewID}},
				},
			},
		},
		stepInReviewID: {
			ID: stepInReviewID, WorkflowID: "wf1", Name: "Review", Position: 1,
			AgentProfileID:            profileReview,
			ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
			ProfileSessionEndPolicy:   models.WorkflowProfileSessionEndPolicyComplete,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			},
		},
		stepReviewedID: {ID: stepReviewedID, WorkflowID: "wf1", Name: "QA Acceptance", Position: 2},
	}
}

// buildColdResumeScenario seeds a card at Review whose initial worker session
// is parked (WAITING_FOR_INPUT with a resumable executors_running row) and
// whose review session is running and has just called move_task_kandev back
// to Work with findings.
func buildColdResumeScenario(t *testing.T, opts coldResumeOptions) *coldResumeScenario {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	log := testLogger()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", OwnerID: coldResumeOwner, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, member := range []string{coldResumeCollaborator, coldResumeReviewerActor} {
		if err := repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
			WorkspaceID: "ws1", UserID: member, Role: "collaborator", AddedBy: coldResumeOwner, CreatedAt: now,
		}); err != nil {
			t.Fatalf("add workspace member %s: %v", member, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Lane", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	// The built-in, unowned exec-local executor is seeded with the schema.
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ep1", ExecutorID: models.ExecutorIDLocal, Name: "Local", OwnerUserID: opts.profileOwner,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create executor profile: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepInReviewID,
		Title: "Test", Description: "Implement a python fibonacci",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.MetaKeyWorkflowInitialSession: models.WorkflowInitialSessionSnapshot{
				SessionID: "session-impl", AgentProfileID: profileImpl,
			},
		},
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocal),
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}

	// Sessions are attributed the way production attributes them: by the
	// identity on the creating request.
	launchCtx := func(actor string) context.Context {
		if actor == "" {
			return ctx
		}
		return authn.WithIdentity(ctx, authn.Identity{UserID: actor, Role: authn.RoleMember})
	}
	if err := repo.CreateTaskSession(launchCtx(opts.sessionActor), &models.TaskSession{
		ID: "session-impl", TaskID: "task-1", AgentProfileID: profileImpl,
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		AgentExecutionID: "ae-impl-original", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("create initial session: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-impl", SessionID: "session-impl", TaskID: "task-1",
		ResumeToken: "resume-token-impl", AgentExecutionID: "ae-impl-original",
		CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert executors_running for initial session: %v", err)
	}
	if err := repo.CreateTaskSession(launchCtx(coldResumeReviewerActor), &models.TaskSession{
		ID: "session-review", TaskID: "task-1", AgentProfileID: profileReview,
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		AgentExecutionID: "ae-review", State: models.TaskSessionStateRunning, IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create review session: %v", err)
	}

	stepGetter := newMockStepGetter()
	for id, step := range coldResumeSteps() {
		stepGetter.steps[id] = step
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Implement a python fibonacci", State: v1.TaskStateInProgress,
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       newSQLiteQueueForTaskRepo(t, repo),
		executor:           exec,
		scheduler:          scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{}),
	}
	svc.SetWorkflowStepGetter(stepGetter)
	const implRelaunchExec = "ae-impl-relaunch"
	wireBootReadySimulator(svc, agentMgr, implRelaunchExec)

	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo,
		Workflows: repo, Messages: repo, Turns: repo, Sessions: repo,
		GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	taskSvc.SetExecutorIdentityResolver(func(_ context.Context, userID string) (authn.Identity, bool) {
		if opts.authDisabled {
			return authn.Identity{UserID: "synthetic", Role: authn.RoleAdmin, Synthetic: true}, true
		}
		switch userID {
		case coldResumeOwner, coldResumeCollaborator, coldResumeReviewerActor:
			return authn.Identity{UserID: userID, Role: authn.RoleMember}, true
		default:
			return authn.Identity{}, false
		}
	})

	sc := &coldResumeScenario{
		pendingMoveScenario: &pendingMoveScenario{
			ctx: ctx, svc: svc, repo: repo, agentMgr: agentMgr, stepGetter: stepGetter,
			implSessionID: "session-impl", reviewSessionID: "session-review", implRelaunchExec: implRelaunchExec,
		},
		taskSvc: taskSvc,
		reveal:  coldResumeSecretStore(t, repo.DB()),
	}
	svc.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		out, err := taskSvc.PrepareExecutorAdmission(ctx, sessionID, executorID, profileID)
		sc.mu.Lock()
		sc.admissions = append(sc.admissions, coldResumeAdmissionCall{sessionID: sessionID, identity: coldResumeIdentity(out), err: err})
		sc.mu.Unlock()
		return out, err
	})
	sc.recordRuntimeContexts()
	sc.armReviewerMove(t)
	return sc
}

// coldResumeSecretStore is a real secrets store holding one global secret for
// the workspace owner and one for the collaborator.
func coldResumeSecretStore(t *testing.T, conn *sql.DB) func(context.Context, string) (string, error) {
	t.Helper()
	key, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("disposable master key: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	store, closeStore, err := secrets.Provide(database, database, key)
	if err != nil {
		t.Fatalf("secrets store: %v", err)
	}
	t.Cleanup(func() { _ = closeStore() })
	for id, owner := range map[string]string{coldResumeOwnerSecret: coldResumeOwner, coldResumeCollabSecret: coldResumeCollaborator} {
		ownerCtx := authn.WithIdentity(context.Background(), authn.Identity{UserID: owner, Role: authn.RoleMember})
		if err := store.Create(ownerCtx, &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id, Scope: secrets.ScopeGlobal},
			Value:  "value-of-" + id,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	return store.RevealGlobal
}

// recordRuntimeContexts keeps the context of every runtime launch and start.
// backendapp hands this context unchanged to lifecycle.Manager, whose profile
// env merge reveals global secrets with it.
func (sc *coldResumeScenario) recordRuntimeContexts() {
	m := sc.agentMgr
	m.mu.Lock()
	defer m.mu.Unlock()
	launch, start := m.launchAgentFunc, m.startAgentProcessFunc
	record := func(name, target string, ctx context.Context) {
		sc.mu.Lock()
		sc.runtime = append(sc.runtime, coldResumeRuntimeCall{name: name, target: target, ctx: context.WithoutCancel(ctx)})
		sc.mu.Unlock()
	}
	m.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		record("LaunchAgent", req.SessionID, ctx)
		return launch(ctx, req)
	}
	m.startAgentProcessFunc = func(ctx context.Context, executionID string) error {
		record("StartAgentProcess", executionID, ctx)
		return start(ctx, executionID)
	}
}

// armReviewerMove persists the deferred move exactly as the MCP
// move_task_kandev handler does for a running review session.
func (sc *coldResumeScenario) armReviewerMove(t *testing.T) {
	t.Helper()
	review, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	if err := sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, &messagequeue.PendingMove{
		MoveID:               uuid.NewString(),
		SessionIncarnationID: review.QueueIncarnationID,
		TaskID:               "task-1",
		WorkflowID:           "wf1",
		WorkflowStepID:       stepInProgressID,
		FromStepID:           stepInReviewID,
		Actor:                string(wfmodels.StepTransitionActorAgent),
		EntryOptions:         &workflowmove.EntryOptions{Instructions: coldResumeFindings},
	}); err != nil {
		t.Fatalf("set pending move: %v", err)
	}
}

// reviewTurnEnds fires the review session's agent.ready, the production
// turn-end entry that applies the deferred move.
func (sc *coldResumeScenario) reviewTurnEnds(ctx context.Context) {
	sc.svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "task-1", SessionID: sc.reviewSessionID, AgentExecutionID: "ae-review", AgentProfileID: profileReview,
	})
}

func (sc *coldResumeScenario) runtimeCalls() []coldResumeRuntimeCall {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return append([]coldResumeRuntimeCall(nil), sc.runtime...)
}

func (sc *coldResumeScenario) launchCount() int {
	n := 0
	for _, call := range sc.runtimeCalls() {
		if call.name == "LaunchAgent" {
			n++
		}
	}
	return n
}

// initialSessionAdmissions returns the admissions that named the card's
// initial worker session.
func (sc *coldResumeScenario) initialSessionAdmissions() []coldResumeAdmissionCall {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	var out []coldResumeAdmissionCall
	for _, call := range sc.admissions {
		if call.sessionID == sc.implSessionID {
			out = append(out, call)
		}
	}
	return out
}

func (sc *coldResumeScenario) findingsDeliveries() int {
	n := 0
	for _, exec := range []string{sc.implRelaunchExec, "ae-impl-original"} {
		for _, p := range capturedPromptsForExecution(sc.agentMgr, exec) {
			n += strings.Count(p, coldResumeFindings)
		}
	}
	for _, d := range capturedExecutionDescriptionsForExecution(sc.agentMgr, sc.implRelaunchExec) {
		n += strings.Count(d, coldResumeFindings)
	}
	for _, entry := range sc.svc.messageQueue.GetStatus(sc.ctx, sc.implSessionID).Entries {
		n += strings.Count(entry.Content, coldResumeFindings)
	}
	return n
}

func (sc *coldResumeScenario) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (sc *coldResumeScenario) initialSession(t *testing.T) *models.TaskSession {
	t.Helper()
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.implSessionID)
	if err != nil {
		t.Fatalf("load initial session: %v", err)
	}
	return session
}

// assertRelaunchedAs checks that the Iterate relaunch ran admission for the
// initial session and that every runtime call carried want, and returns the
// runtime calls.
func (sc *coldResumeScenario) assertRelaunchedAs(t *testing.T, want string) []coldResumeRuntimeCall {
	t.Helper()
	sc.waitFor(t, "the Iterate relaunch", func() bool { return sc.launchCount() > 0 })
	sc.waitFor(t, "the findings on the initial session", func() bool { return sc.findingsDeliveries() > 0 })
	admissions := sc.initialSessionAdmissions()
	if len(admissions) == 0 {
		t.Error("executor admission never ran for the cold-resumed initial session")
	}
	for _, a := range admissions {
		if a.err != nil || a.identity != want {
			t.Errorf("initial session admission: identity=%s err=%v, want %s admitted", a.identity, a.err, want)
		}
	}
	calls := sc.runtimeCalls()
	for _, call := range calls {
		if got := coldResumeIdentity(call.ctx); got != want {
			t.Errorf("%s carried identity %s, want %s", call.name, got, want)
		}
	}
	return calls
}

func (sc *coldResumeScenario) assertReveals(t *testing.T, calls []coldResumeRuntimeCall, visible, hidden string) {
	t.Helper()
	for _, call := range calls {
		if v, err := sc.reveal(call.ctx, visible); err != nil || v != "value-of-"+visible {
			t.Errorf("%s context cannot reveal %s: value=%q err=%v", call.name, visible, v, err)
		}
		if v, err := sc.reveal(call.ctx, hidden); err == nil {
			t.Errorf("%s context revealed another user's secret %s (%q)", call.name, hidden, v)
		}
	}
}

// assertFailedClosed checks that the relaunch was refused before any runtime
// call and that the refusal is visible on the initial session.
func (sc *coldResumeScenario) assertFailedClosed(t *testing.T) *models.TaskSession {
	t.Helper()
	sc.waitFor(t, "the initial session to record the refused relaunch", func() bool {
		return sc.initialSession(t).State == models.TaskSessionStateFailed
	})
	time.Sleep(200 * time.Millisecond)
	if calls := sc.runtimeCalls(); len(calls) != 0 {
		for _, call := range calls {
			t.Errorf("refused relaunch still reached the runtime: %s as %s", call.name, coldResumeIdentity(call.ctx))
		}
	}
	session := sc.initialSession(t)
	if session.ErrorMessage == "" {
		t.Error("the refused relaunch left no error on the initial session")
	}
	return session
}

// The card was launched by a collaborator. The Iterate relaunch runs as the
// collaborator: admission runs for the initial session and the launch context
// reveals the collaborator's secrets, never the workspace owner's.
func TestColdResumeAdmission_CollaboratorCardRelaunchesAsCollaborator(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeCollaborator})
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	calls := sc.assertRelaunchedAs(t, coldResumeCollaborator)
	sc.assertReveals(t, calls, coldResumeCollabSecret, coldResumeOwnerSecret)
	if got := sc.findingsDeliveries(); got != 1 {
		t.Errorf("findings deliveries = %d, want 1", got)
	}
}

// An owner-launched card relaunches as the owner and cannot reveal the
// collaborator's secrets. A successful owner retry therefore says nothing
// about other users' cards.
func TestColdResumeAdmission_OwnerCardRelaunchesAsOwner(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeOwner})
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	calls := sc.assertRelaunchedAs(t, coldResumeOwner)
	sc.assertReveals(t, calls, coldResumeOwnerSecret, coldResumeCollabSecret)
}

// With authentication on, a session with no durable executor actor cannot be
// relaunched by an internal event: there is nobody to launch as. The resume
// fails closed and surfaces on the session; it does not fall back to an
// unscoped launch, the reviewer, or the workspace owner.
func TestColdResumeAdmission_UnattributedSessionFailsClosed(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: ""})
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	session := sc.assertFailedClosed(t)
	if !strings.Contains(session.ErrorMessage, "executor actor") {
		t.Errorf("session error = %q, want the missing executor actor named", session.ErrorMessage)
	}
}

// With authentication off every actor resolves to the synthetic identity, so
// single-user installs relaunch exactly as before.
func TestColdResumeAdmission_AuthDisabledRelaunchesWithSyntheticIdentity(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: "", authDisabled: true})
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	sc.assertRelaunchedAs(t, "<synthetic>")
	if got := sc.findingsDeliveries(); got != 1 {
		t.Errorf("findings deliveries = %d, want 1", got)
	}
}

// The executor profile is private to the workspace owner and the card was
// launched by the collaborator. Admission refuses the collaborator and the
// relaunch is not retried under anyone else's authority.
func TestColdResumeAdmission_PrivateProfileIsNotBorrowed(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeCollaborator, profileOwner: coldResumeOwner})
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	sc.assertFailedClosed(t)
	admissions := sc.initialSessionAdmissions()
	if len(admissions) == 0 {
		t.Fatal("executor admission never ran for the cold-resumed initial session")
	}
	for _, a := range admissions {
		if a.err == nil {
			t.Errorf("admission accepted the owner's private profile for %s", a.identity)
		}
	}
}

// Duplicate and concurrent turn ends for the same review turn apply the move
// once and relaunch the parked session once, as its own actor.
func TestColdResumeAdmission_DuplicateAndConcurrentTurnEndsRelaunchOnce(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeCollaborator})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc.reviewTurnEnds(sc.ctx)
		}()
	}
	wg.Wait()
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	sc.assertRelaunchedAs(t, coldResumeCollaborator)
	sc.reviewTurnEnds(sc.ctx)
	time.Sleep(300 * time.Millisecond)
	if got := sc.launchCount(); got != 1 {
		t.Errorf("runtime launches = %d, want 1", got)
	}
	if got := sc.findingsDeliveries(); got != 1 {
		t.Errorf("findings deliveries = %d, want 1", got)
	}
}

// A turn end delivered with an already-cancelled context (a shutting-down
// event handler) relaunches nothing under any identity and leaves the move
// pending. The next turn end then relaunches the parked session exactly once,
// as its own actor.
func TestColdResumeAdmission_CancelledTurnEndLaunchesNothing(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeCollaborator})
	ctx, cancel := context.WithCancel(sc.ctx)
	cancel()
	sc.reviewTurnEnds(ctx)
	time.Sleep(300 * time.Millisecond)
	if calls := sc.runtimeCalls(); len(calls) != 0 {
		for _, call := range calls {
			t.Errorf("cancelled turn end reached the runtime: %s as %s", call.name, coldResumeIdentity(call.ctx))
		}
	}
	if _, ok, err := sc.svc.messageQueue.GetPendingMoveWithError(sc.ctx, sc.reviewSessionID); err != nil || !ok {
		t.Fatalf("pending move after cancelled turn end: present=%v err=%v; want it retained", ok, err)
	}

	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	calls := sc.assertRelaunchedAs(t, coldResumeCollaborator)
	sc.assertReveals(t, calls, coldResumeCollabSecret, coldResumeOwnerSecret)
	if got := sc.launchCount(); got != 1 {
		t.Errorf("runtime launches = %d, want 1", got)
	}
}

// When the worker session already has a prompt-ready execution, the findings
// are delivered to it without a relaunch, so no launch can substitute another
// identity.
func TestColdResumeAdmission_ActiveSessionIsNotRelaunched(t *testing.T) {
	sc := buildColdResumeScenario(t, coldResumeOptions{sessionActor: coldResumeCollaborator})
	if err := sc.repo.UpdateTaskSessionState(sc.ctx, sc.implSessionID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("mark initial session running: %v", err)
	}
	sc.agentMgr.mu.Lock()
	active := func(_ context.Context, sessionID string) bool { return sessionID == "session-impl" }
	sc.agentMgr.isAgentRunningFn = active
	sc.agentMgr.isAgentReadyFn = active
	sc.agentMgr.mu.Unlock()
	sc.reviewTurnEnds(sc.ctx)
	sc.waitForTaskStep(t, stepInProgressID, 3*time.Second)
	sc.waitFor(t, "the findings on the active session", func() bool { return sc.findingsDeliveries() > 0 })
	time.Sleep(300 * time.Millisecond)
	for _, call := range sc.runtimeCalls() {
		t.Errorf("active session was relaunched: %s %s as %s", call.name, call.target, coldResumeIdentity(call.ctx))
	}
	if got := len(sc.initialSessionAdmissions()); got != 0 {
		t.Errorf("initial session admissions = %d, want none without a relaunch", got)
	}
	if got := sc.findingsDeliveries(); got != 1 {
		t.Errorf("findings deliveries = %d, want 1", got)
	}
}
