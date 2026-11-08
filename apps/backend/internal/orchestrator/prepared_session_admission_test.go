package orchestrator

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Regression coverage for kdlbs/kandev#5181: a CREATED session whose
// workspace was prepared (executors_running row present) but whose agent was
// never started is started through attemptColdResume ->
// startAgentOnPreparedWorkspace -> LaunchPreparedSession. That start must run
// as an authorized actor. With no caller identity and no usable durable
// executor actor, admission used to leave the context identity-free, which the
// secrets store treats as unscoped, so the runtime could reveal every user's
// global secrets.
//
// Admission is the real task-service PrepareExecutorAdmission wired the way
// backendapp wires it, and reveals use a real secrets store with synthetic
// values. The identity resolver and the agent runtime are stand-ins.

const (
	preparedOwner        = "user-prepared-owner"
	preparedCollaborator = "user-prepared-collaborator"
	preparedStranger     = "user-prepared-stranger"
	preparedDeleted      = "user-prepared-deleted"
	preparedOwnerSecret  = "secret-prepared-owner"
	preparedCollabSecret = "secret-prepared-collaborator"
	preparedSessionID    = "session-prepared"
	preparedRelaunchExec = "ae-prepared-start"
)

type preparedAdmissionOptions struct {
	// sessionActor is the durable ExecutorActorUserID ("" = unattributed).
	sessionActor string
	// profileOwner owns the executor profile ("" = shared).
	profileOwner string
	// authDisabled resolves every actor to the synthetic identity.
	authDisabled bool
}

type preparedAdmissionCall struct {
	identity string
	err      error
}

type preparedRuntimeCall struct {
	name string
	ctx  context.Context
}

type preparedAdmissionScenario struct {
	ctx  context.Context
	svc  *Service
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	}
	agentMgr *mockAgentManager
	reveal   func(context.Context, string) (string, error)

	mu         sync.Mutex
	admissions []preparedAdmissionCall
	runtime    []preparedRuntimeCall
}

func preparedIdentity(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return "<none>"
	}
	if identity.Synthetic {
		return "<synthetic>"
	}
	return identity.UserID
}

func preparedCallerContext(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func buildPreparedAdmissionScenario(t *testing.T, opts preparedAdmissionOptions) *preparedAdmissionScenario {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	log := testLogger()

	mustNoErr := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	mustNoErr("create workspace", repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", OwnerID: preparedOwner, CreatedAt: now, UpdatedAt: now}))
	mustNoErr("add collaborator", repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: "ws1", UserID: preparedCollaborator, Role: "collaborator", AddedBy: preparedOwner, CreatedAt: now,
	}))
	mustNoErr("create workflow", repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Lane", CreatedAt: now, UpdatedAt: now}))
	mustNoErr("create executor profile", repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ep1", ExecutorID: models.ExecutorIDLocal, Name: "Local", OwnerUserID: opts.profileOwner,
		CreatedAt: now, UpdatedAt: now,
	}))
	mustNoErr("create task", repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
		Title: "Test", Description: "Prepared workspace", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	mustNoErr("create task environment", repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocal),
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", Status: models.TaskEnvironmentStatusReady,
	}))
	sessionCtx := ctx
	if opts.sessionActor != "" {
		sessionCtx = preparedCallerContext(opts.sessionActor)
	}
	mustNoErr("create prepared session", repo.CreateTaskSession(sessionCtx, &models.TaskSession{
		ID: preparedSessionID, TaskID: "task-1", AgentProfileID: profileImpl,
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateCreated, IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
	mustNoErr("upsert executors_running", repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: preparedSessionID, SessionID: preparedSessionID, TaskID: "task-1",
		AgentExecutionID: "ae-prepared", CreatedAt: now, UpdatedAt: now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "Work", Position: 0, IsStartStep: true,
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Prepared workspace", State: v1.TaskStateInProgress,
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
	wirePreparedBootReady(svc, agentMgr)

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
		case preparedOwner, preparedCollaborator, preparedStranger:
			return authn.Identity{UserID: userID, Role: authn.RoleMember}, true
		default:
			// Empty, deleted, and disabled users do not resolve.
			return authn.Identity{}, false
		}
	})

	sc := &preparedAdmissionScenario{
		ctx: ctx, svc: svc, repo: repo, agentMgr: agentMgr,
		reveal: preparedSecretStore(t, repo.DB()),
	}
	svc.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		out, err := taskSvc.PrepareExecutorAdmission(ctx, sessionID, executorID, profileID)
		if sessionID == preparedSessionID {
			sc.mu.Lock()
			sc.admissions = append(sc.admissions, preparedAdmissionCall{identity: preparedIdentity(out), err: err})
			sc.mu.Unlock()
		}
		return out, err
	})
	sc.recordRuntimeContexts()
	return sc
}

// wirePreparedBootReady models the agent start on a prepared workspace:
// StartAgentProcess starts ACP on the existing execution and emits
// agent.boot_ready for the prepared session. A fresh LaunchAgent, which this
// path must not need, reports a new execution.
func wirePreparedBootReady(svc *Service, agentMgr *mockAgentManager) {
	promptReady := make(chan struct{})
	var readyOnce sync.Once
	started := func(_ context.Context, _ string) bool {
		select {
		case <-promptReady:
			return true
		default:
			return false
		}
	}
	agentMgr.isAgentReadyFn = started
	agentMgr.isAgentRunningFn = started
	agentMgr.launchAgentFunc = func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return &executor.LaunchAgentResponse{AgentExecutionID: preparedRelaunchExec, Status: v1.AgentStatusReady}, nil
	}
	agentMgr.startAgentProcessFunc = func(startCtx context.Context, executionID string) error {
		readyOnce.Do(func() { close(promptReady) })
		svc.handleAgentBootReady(context.Background(), watcher.AgentEventData{
			TaskID:           "task-1",
			SessionID:        preparedSessionID,
			AgentExecutionID: executionID,
			AttemptID:        executor.ResumeAttemptIDFromContext(startCtx),
			AgentProfileID:   profileImpl,
		})
		return nil
	}
}

// preparedSecretStore is a real secrets store holding one synthetic global
// secret for the workspace owner and one for the collaborator.
func preparedSecretStore(t *testing.T, conn *sql.DB) func(context.Context, string) (string, error) {
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
	for id, owner := range map[string]string{preparedOwnerSecret: preparedOwner, preparedCollabSecret: preparedCollaborator} {
		if err := store.Create(preparedCallerContext(owner), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id, Scope: secrets.ScopeGlobal},
			Value:  "synthetic-value-of-" + id,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	return store.RevealGlobal
}

// recordRuntimeContexts keeps the context of every runtime launch and start.
// backendapp hands this context unchanged to lifecycle.Manager, whose profile
// env merge reveals global secrets with it.
func (sc *preparedAdmissionScenario) recordRuntimeContexts() {
	m := sc.agentMgr
	m.mu.Lock()
	defer m.mu.Unlock()
	launch, start := m.launchAgentFunc, m.startAgentProcessFunc
	record := func(name string, ctx context.Context) {
		sc.mu.Lock()
		sc.runtime = append(sc.runtime, preparedRuntimeCall{name: name, ctx: context.WithoutCancel(ctx)})
		sc.mu.Unlock()
	}
	m.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		record("LaunchAgent", ctx)
		return launch(ctx, req)
	}
	m.startAgentProcessFunc = func(ctx context.Context, executionID string) error {
		record("StartAgentProcess", ctx)
		return start(ctx, executionID)
	}
}

func (sc *preparedAdmissionScenario) start(ctx context.Context, t *testing.T) error {
	t.Helper()
	session, err := sc.repo.GetTaskSession(sc.ctx, preparedSessionID)
	if err != nil {
		t.Fatalf("load prepared session: %v", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return sc.svc.ensureSessionRunning(runCtx, preparedSessionID, session, launchOriginAutomatic)
}

func (sc *preparedAdmissionScenario) runtimeCalls() []preparedRuntimeCall {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return append([]preparedRuntimeCall(nil), sc.runtime...)
}

func (sc *preparedAdmissionScenario) admissionCalls() []preparedAdmissionCall {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return append([]preparedAdmissionCall(nil), sc.admissions...)
}

func (sc *preparedAdmissionScenario) session(t *testing.T) *models.TaskSession {
	t.Helper()
	session, err := sc.repo.GetTaskSession(sc.ctx, preparedSessionID)
	if err != nil {
		t.Fatalf("load prepared session: %v", err)
	}
	return session
}

// assertStartedAs checks that the start succeeded, that admission ran for the
// prepared session, and that every runtime call carried want.
func (sc *preparedAdmissionScenario) assertStartedAs(t *testing.T, err error, want string) []preparedRuntimeCall {
	t.Helper()
	if err != nil {
		t.Fatalf("prepared start failed: %v", err)
	}
	if len(sc.admissionCalls()) == 0 {
		t.Error("executor admission never ran for the prepared session")
	}
	calls := sc.runtimeCalls()
	if len(calls) == 0 {
		t.Fatal("prepared start never reached the runtime")
	}
	for _, call := range calls {
		if got := preparedIdentity(call.ctx); got != want {
			t.Errorf("%s carried identity %s, want %s", call.name, got, want)
		}
	}
	return calls
}

func (sc *preparedAdmissionScenario) assertReveals(t *testing.T, calls []preparedRuntimeCall, visible, hidden string) {
	t.Helper()
	for _, call := range calls {
		if v, err := sc.reveal(call.ctx, visible); err != nil || v != "synthetic-value-of-"+visible {
			t.Errorf("%s context cannot reveal %s: err=%v", call.name, visible, err)
		}
		if _, err := sc.reveal(call.ctx, hidden); err == nil {
			t.Errorf("%s context revealed another user's secret %s", call.name, hidden)
		}
	}
}

// assertFailedClosed checks that the start was refused before any runtime
// call, that no runtime context could reveal either synthetic secret, and that
// the refusal is recorded on the session without secret values.
func (sc *preparedAdmissionScenario) assertFailedClosed(t *testing.T, err error) *models.TaskSession {
	t.Helper()
	if err == nil {
		t.Error("prepared start succeeded, want it refused")
	}
	for _, call := range sc.runtimeCalls() {
		t.Errorf("refused start still reached the runtime: %s as %s", call.name, preparedIdentity(call.ctx))
		for _, id := range []string{preparedOwnerSecret, preparedCollabSecret} {
			if _, revealErr := sc.reveal(call.ctx, id); revealErr == nil {
				t.Errorf("%s context revealed %s", call.name, id)
			}
		}
	}
	session := sc.session(t)
	if session.State != models.TaskSessionStateFailed {
		t.Errorf("session state = %s, want FAILED", session.State)
	}
	if session.ErrorMessage == "" {
		t.Error("the refused start left no error on the session")
	}
	if strings.Contains(session.ErrorMessage, "synthetic-value-of-") {
		t.Errorf("session error discloses a secret value: %q", session.ErrorMessage)
	}
	return session
}
