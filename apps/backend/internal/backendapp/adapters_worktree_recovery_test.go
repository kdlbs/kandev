package backendapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type capturingWorktreeRuntime struct {
	req   *lifecycle.ExecutorCreateRequest
	claim *models.TaskEnvironmentRecoveryClaim
	calls int
}

var errWorktreeRuntimeCaptured = errors.New("worktree runtime captured")

func (r *capturingWorktreeRuntime) Name() agentexecutor.Name          { return agentexecutor.NameStandalone }
func (r *capturingWorktreeRuntime) HealthCheck(context.Context) error { return nil }
func (r *capturingWorktreeRuntime) CreateInstance(ctx context.Context, req *lifecycle.ExecutorCreateRequest) (*lifecycle.ExecutorInstance, error) {
	r.calls++
	r.req = req
	r.claim = worktree.RecoveryClaimFromContext(ctx)
	return nil, errWorktreeRuntimeCaptured
}
func (r *capturingWorktreeRuntime) StopInstance(context.Context, *lifecycle.ExecutorInstance, bool) error {
	return nil
}
func (r *capturingWorktreeRuntime) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}
func (r *capturingWorktreeRuntime) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (r *capturingWorktreeRuntime) RequiresCloneURL() bool                           { return false }
func (r *capturingWorktreeRuntime) ShouldApplyPreferredShell() bool                  { return false }
func (r *capturingWorktreeRuntime) IsAlwaysResumable() bool                          { return true }

// TestLifecycleAdapterWorktreeRecoveryConsumesReplacement exercises the
// production ResumeSession path through the lifecycle adapter and real
// worktree preparer. It proves a broken linked checkout is replaced before
// lifecycle execution, then hands the replacement to the final runtime seam.
func TestLifecycleAdapterWorktreeRecoveryConsumesReplacement(t *testing.T) {
	ctx := context.Background()
	const (
		workspaceID          = "workspace-lifecycle-recovery"
		taskID               = "task-lifecycle-recovery"
		sessionID            = "session-lifecycle-recovery"
		environmentID        = "environment-lifecycle-recovery"
		frontendRepositoryID = "repository-lifecycle-recovery-frontend"
		backendRepositoryID  = "repository-lifecycle-recovery-backend"
		frontendWorktreeID   = "worktree-lifecycle-recovery-frontend"
		backendWorktreeID    = "worktree-lifecycle-recovery-backend"
		executorID           = "executor-lifecycle-recovery"
		incarnationID        = "incarnation-lifecycle-recovery"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqliteDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqliteDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("new task repository: %v", err)
	}
	worktreeStore, err := worktree.NewSQLiteStore(sqliteDB, sqliteDB)
	if err != nil {
		t.Fatalf("new worktree store: %v", err)
	}
	log := newTestLogger()
	tasksPath := filepath.Join(t.TempDir(), "tasks")
	worktreeManager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasksPath, BranchPrefix: "kandev/"}, worktreeStore, log)
	if err != nil {
		t.Fatalf("new worktree manager: %v", err)
	}
	frontendRepositoryPath := lifecycleRecoveryGitRepository(t, "frontend")
	backendRepositoryPath := lifecycleRecoveryGitRepository(t, "backend")
	taskRoot := filepath.Join(tasksPath, "recovery")
	frontendOriginalPath := filepath.Join(taskRoot, "frontend")
	backendOriginalPath := filepath.Join(taskRoot, "backend")
	runLifecycleRecoveryGit(t, frontendRepositoryPath, "worktree", "add", frontendOriginalPath, "feature/recovery")
	runLifecycleRecoveryGit(t, backendRepositoryPath, "worktree", "add", backendOriginalPath, "feature/recovery")
	removeLifecycleRecoveryWorktreeAdmin(t, frontendOriginalPath)
	removeLifecycleRecoveryWorktreeAdmin(t, backendOriginalPath)
	seedLifecycleRecoveryStore(t, ctx, taskRepo, lifecycleRecoverySeed{
		workspaceID: workspaceID, taskID: taskID, sessionID: sessionID, environmentID: environmentID,
		frontendRepositoryID: frontendRepositoryID, backendRepositoryID: backendRepositoryID,
		frontendWorktreeID: frontendWorktreeID, backendWorktreeID: backendWorktreeID,
		executorID: executorID, incarnationID: incarnationID, frontendRepositoryPath: frontendRepositoryPath,
		backendRepositoryPath: backendRepositoryPath, taskRoot: taskRoot, frontendOriginalPath: frontendOriginalPath,
		backendOriginalPath: backendOriginalPath,
	})
	agentsRegistry := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	agent.SetEnabled(true)
	if err := agentsRegistry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	runtime := &capturingWorktreeRuntime{}
	runtimes := lifecycle.NewExecutorRegistry(log)
	runtimes.Register(runtime)
	events := bus.NewMemoryEventBus(log)
	t.Cleanup(events.Close)
	manager := lifecycle.NewManager(agentsRegistry, events, runtimes, nil, nil, nil, lifecycle.ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = manager.Stop() })
	// SetWorktreeManager installs the real preparer only after a registry exists.
	preparers := lifecycle.NewPreparerRegistry(log)
	manager.SetPreparerRegistry(preparers)
	manager.SetWorktreeManager(worktreeManager)
	if _, ok := preparers.Get(models.ExecutorTypeWorktree).(*lifecycle.WorktreePreparer); !ok {
		t.Fatal("worktree executor did not receive the concrete worktree preparer")
	}
	executor := orchestratorexecutor.NewExecutor(newLifecycleAdapter(manager, agentsRegistry, log), taskRepo, log, orchestratorexecutor.ExecutorConfig{})
	executor.SetSelectedWorktreeRecoveryAdmission(worktreeManager.AdmitRecovery)
	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if _, err := executor.ResumeSession(ctx, session, true); !errors.Is(err, errWorktreeRuntimeCaptured) {
		t.Fatalf("ResumeSession error = %v, want final runtime capture", err)
	}
	if runtime.req == nil {
		t.Fatal("runtime was not reached through the registered worktree preparer")
	}
	if runtime.req.WorkspacePath != taskRoot {
		t.Fatalf("runtime workspace = %q, want recovered replacement", runtime.req.WorkspacePath)
	}
	if got, want := runtime.req.WorkspaceSourceRoots, []string{frontendRepositoryPath, backendRepositoryPath}; !sameLifecycleRecoveryStrings(got, want) {
		t.Fatalf("runtime workspace source roots = %q, want %q", got, want)
	}
	if runtime.req.TaskEnvironmentID != environmentID || runtime.req.SessionID != sessionID {
		t.Fatalf("runtime identity = environment %q session %q, want %q %q", runtime.req.TaskEnvironmentID, runtime.req.SessionID, environmentID, sessionID)
	}
	if runtime.claim == nil || runtime.claim.TaskEnvironmentID != environmentID || runtime.claim.SessionID != sessionID || runtime.claim.SessionIncarnationID != incarnationID {
		t.Fatalf("runtime recovery claim = %+v, want exact durable authority", runtime.claim)
	}
	for _, originalID := range []string{frontendWorktreeID, backendWorktreeID} {
		recovered, getErr := worktreeStore.GetWorktreeByID(ctx, originalID)
		if getErr != nil {
			t.Fatalf("get original worktree %q: %v", originalID, getErr)
		}
		if recovered != nil {
			t.Fatalf("original worktree %q remains durable: %+v", originalID, recovered)
		}
	}
	worktrees, err := worktreeStore.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get recovered worktree: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("durable replacement = %+v, runtime workspace = %q", worktrees, runtime.req.WorkspacePath)
	}
	for _, recovered := range worktrees {
		if !strings.Contains(recovered.Path, ".recovered-") || !worktreeManager.IsValid(recovered.Path) {
			t.Fatalf("durable replacement = %+v, want valid rematerialized worktrees", worktrees)
		}
	}
	var claims int
	if err := sqliteDB.GetContext(ctx, &claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, environmentID); err != nil {
		t.Fatalf("count recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable recovery claims = %d, want released exactly once", claims)
	}
}

// TestLifecycleAdapterWorktreeRecoveryPreparerFailureKeepsRecoveredSlot proves
// a later repository preparation failure cannot start the runtime or undo an
// already-authorized replacement from the same selected environment.
func TestLifecycleAdapterWorktreeRecoveryPreparerFailureKeepsRecoveredSlot(t *testing.T) {
	ctx := context.Background()
	const (
		workspaceID        = "workspace-lifecycle-recovery-failure"
		taskID             = "task-lifecycle-recovery-failure"
		sessionID          = "session-lifecycle-recovery-failure"
		environmentID      = "environment-lifecycle-recovery-failure"
		frontendID         = "repository-lifecycle-recovery-failure-frontend"
		backendID          = "repository-lifecycle-recovery-failure-backend"
		frontendWorktreeID = "worktree-lifecycle-recovery-failure-frontend"
		backendWorktreeID  = "worktree-lifecycle-recovery-failure-backend"
		executorID         = "executor-lifecycle-recovery-failure"
		incarnationID      = "incarnation-lifecycle-recovery-failure"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqliteDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqliteDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("new task repository: %v", err)
	}
	store, err := worktree.NewSQLiteStore(sqliteDB, sqliteDB)
	if err != nil {
		t.Fatalf("new worktree store: %v", err)
	}
	log := newTestLogger()
	tasksPath := filepath.Join(t.TempDir(), "tasks")
	worktreeManager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasksPath, BranchPrefix: "kandev/"}, store, log)
	if err != nil {
		t.Fatalf("new worktree manager: %v", err)
	}
	frontendPath := lifecycleRecoveryGitRepository(t, "frontend")
	backendPath := lifecycleRecoveryGitRepository(t, "backend")
	taskRoot := filepath.Join(tasksPath, "recovery")
	frontendOriginalPath := filepath.Join(taskRoot, "frontend")
	backendOriginalPath := filepath.Join(taskRoot, "backend")
	runLifecycleRecoveryGit(t, frontendPath, "worktree", "add", frontendOriginalPath, "feature/recovery")
	runLifecycleRecoveryGit(t, backendPath, "worktree", "add", backendOriginalPath, "feature/recovery")
	removeLifecycleRecoveryWorktreeAdmin(t, frontendOriginalPath)
	seedLifecycleRecoveryStore(t, ctx, taskRepo, lifecycleRecoverySeed{
		workspaceID: workspaceID, taskID: taskID, sessionID: sessionID, environmentID: environmentID,
		frontendRepositoryID: frontendID, backendRepositoryID: backendID,
		frontendWorktreeID: frontendWorktreeID, backendWorktreeID: backendWorktreeID, executorID: executorID, incarnationID: incarnationID,
		frontendRepositoryPath: frontendPath, backendRepositoryPath: backendPath, taskRoot: taskRoot,
		frontendOriginalPath: frontendOriginalPath, backendOriginalPath: backendOriginalPath,
	})
	agentsRegistry := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	agent.SetEnabled(true)
	if err := agentsRegistry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	runtime := &capturingWorktreeRuntime{}
	runtimes := lifecycle.NewExecutorRegistry(log)
	runtimes.Register(runtime)
	events := bus.NewMemoryEventBus(log)
	t.Cleanup(events.Close)
	manager := lifecycle.NewManager(agentsRegistry, events, runtimes, nil, nil, nil, lifecycle.ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = manager.Stop() })
	preparers := lifecycle.NewPreparerRegistry(log)
	manager.SetPreparerRegistry(preparers)
	manager.SetWorktreeManager(worktreeManager)
	executor := orchestratorexecutor.NewExecutor(newLifecycleAdapter(manager, agentsRegistry, log), taskRepo, log, orchestratorexecutor.ExecutorConfig{})
	executor.SetSelectedWorktreeRecoveryAdmission(func(admissionCtx context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admission, admissionErr := worktreeManager.AdmitRecovery(admissionCtx, req)
		if admissionErr == nil && admission != nil {
			if removeErr := os.RemoveAll(filepath.Join(backendPath, ".git")); removeErr != nil {
				t.Fatalf("break backend source after selected recovery: %v", removeErr)
			}
		}
		return admission, admissionErr
	})
	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	_, resumeErr := executor.ResumeSession(ctx, session, true)
	var preparationErr *lifecycle.RepositoryPreparationError
	if !errors.As(resumeErr, &preparationErr) || preparationErr.RepositoryID != backendID {
		t.Fatalf("ResumeSession error = %v, want backend repository preparation failure", resumeErr)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runtime.calls)
	}
	recovered, err := store.GetWorktreeByID(ctx, frontendWorktreeID)
	if err != nil {
		t.Fatalf("get original frontend worktree: %v", err)
	}
	worktrees, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get recovered worktrees: %v", err)
	}
	if recovered != nil {
		t.Fatalf("original frontend worktree remains durable: %+v", recovered)
	}
	var frontendReplacement *worktree.Worktree
	for _, candidate := range worktrees {
		if candidate.RepositoryID == frontendID && candidate.ID != frontendWorktreeID {
			frontendReplacement = candidate
		}
	}
	if frontendReplacement == nil || !worktreeManager.IsValid(frontendReplacement.Path) {
		t.Fatalf("recovered worktrees = %+v, want a valid frontend replacement", worktrees)
	}
	var claims int
	if err := sqliteDB.GetContext(ctx, &claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, environmentID); err != nil {
		t.Fatalf("count recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable recovery claims = %d, want 0", claims)
	}
}

func TestLifecycleAdapterWorktreeRecoveryRejectsLiveConsumer(t *testing.T) {
	ctx := context.Background()
	const (
		workspaceID        = "workspace-lifecycle-live-consumer"
		taskID             = "task-lifecycle-live-consumer"
		sessionID          = "session-lifecycle-live-consumer"
		consumerID         = "session-lifecycle-live-consumer-other"
		environmentID      = "environment-lifecycle-live-consumer"
		frontendID         = "repository-lifecycle-live-consumer-frontend"
		backendID          = "repository-lifecycle-live-consumer-backend"
		frontendWorktreeID = "worktree-lifecycle-live-consumer-frontend"
		backendWorktreeID  = "worktree-lifecycle-live-consumer-backend"
		executorID         = "executor-lifecycle-live-consumer"
		incarnationID      = "incarnation-lifecycle-live-consumer"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqliteDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqliteDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("new task repository: %v", err)
	}
	store, err := worktree.NewSQLiteStore(sqliteDB, sqliteDB)
	if err != nil {
		t.Fatalf("new worktree store: %v", err)
	}
	log := newTestLogger()
	tasksPath := filepath.Join(t.TempDir(), "tasks")
	worktreeManager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasksPath, BranchPrefix: "kandev/"}, store, log)
	if err != nil {
		t.Fatalf("new worktree manager: %v", err)
	}
	frontendPath := lifecycleRecoveryGitRepository(t, "frontend")
	backendPath := lifecycleRecoveryGitRepository(t, "backend")
	taskRoot := filepath.Join(tasksPath, "recovery")
	frontendOriginalPath := filepath.Join(taskRoot, "frontend")
	backendOriginalPath := filepath.Join(taskRoot, "backend")
	runLifecycleRecoveryGit(t, frontendPath, "worktree", "add", frontendOriginalPath, "feature/recovery")
	runLifecycleRecoveryGit(t, backendPath, "worktree", "add", backendOriginalPath, "feature/recovery")
	seedLifecycleRecoveryStore(t, ctx, taskRepo, lifecycleRecoverySeed{
		workspaceID: workspaceID, taskID: taskID, sessionID: sessionID, environmentID: environmentID,
		frontendRepositoryID: frontendID, backendRepositoryID: backendID, frontendWorktreeID: frontendWorktreeID,
		backendWorktreeID: backendWorktreeID, executorID: executorID, incarnationID: incarnationID,
		frontendRepositoryPath: frontendPath, backendRepositoryPath: backendPath, taskRoot: taskRoot,
		frontendOriginalPath: frontendOriginalPath, backendOriginalPath: backendOriginalPath,
	})
	now := time.Now().UTC()
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: consumerID, TaskID: taskID, QueueIncarnationID: "incarnation-live-consumer", TaskEnvironmentID: environmentID, ExecutorID: executorID, RepositoryID: frontendID, AgentProfileID: "mock-agent", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create live consumer session: %v", err)
	}
	if err := taskRepo.CreateTurn(ctx, &models.Turn{ID: "turn-lifecycle-live-consumer", TaskID: taskID, TaskSessionID: consumerID, StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create live consumer turn: %v", err)
	}
	removeLifecycleRecoveryWorktreeAdmin(t, frontendOriginalPath)
	original, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get original worktrees: %v", err)
	}
	agentsRegistry := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	agent.SetEnabled(true)
	if err := agentsRegistry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	runtime := &capturingWorktreeRuntime{}
	runtimes := lifecycle.NewExecutorRegistry(log)
	runtimes.Register(runtime)
	events := bus.NewMemoryEventBus(log)
	t.Cleanup(events.Close)
	manager := lifecycle.NewManager(agentsRegistry, events, runtimes, nil, nil, nil, lifecycle.ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = manager.Stop() })
	preparers := lifecycle.NewPreparerRegistry(log)
	manager.SetPreparerRegistry(preparers)
	manager.SetWorktreeManager(worktreeManager)
	executor := orchestratorexecutor.NewExecutor(newLifecycleAdapter(manager, agentsRegistry, log), taskRepo, log, orchestratorexecutor.ExecutorConfig{})
	executor.SetSelectedWorktreeRecoveryAdmission(worktreeManager.AdmitRecovery)
	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get requester session: %v", err)
	}
	if _, err := executor.ResumeSession(ctx, session, true); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("ResumeSession error = %v, want live-consumer busy refusal", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runtime.calls)
	}
	after, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get worktrees after refusal: %v", err)
	}
	if len(after) != len(original) {
		t.Fatalf("durable worktree count = %d, want %d", len(after), len(original))
	}
	afterByRepository := make(map[string]*worktree.Worktree, len(after))
	for _, candidate := range after {
		afterByRepository[candidate.RepositoryID] = candidate
	}
	for _, before := range original {
		candidate := afterByRepository[before.RepositoryID]
		if candidate == nil || before.ID != candidate.ID || before.Path != candidate.Path {
			t.Fatalf("durable worktree for repository %q changed: before=%+v after=%+v", before.RepositoryID, before, candidate)
		}
	}
	var claims int
	if err := sqliteDB.GetContext(ctx, &claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, environmentID); err != nil {
		t.Fatalf("count recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable recovery claims = %d, want 0", claims)
	}
}

// TestLifecycleAdapterWorktreeRecoveryRejectsMismatchedRequester proves the
// real durable claim boundary rejects an ownership change before liveness can
// acquire a claim or mutate a damaged checkout.
func TestLifecycleAdapterWorktreeRecoveryRejectsMismatchedRequester(t *testing.T) {
	ctx := context.Background()
	const (
		workspaceID         = "workspace-lifecycle-requester-mismatch"
		attackerWorkspaceID = "workspace-lifecycle-requester-attacker"
		taskID              = "task-lifecycle-requester-mismatch"
		attackerTaskID      = "task-lifecycle-requester-attacker"
		sessionID           = "session-lifecycle-requester-mismatch"
		consumerID          = "session-lifecycle-requester-mismatch-consumer"
		environmentID       = "environment-lifecycle-requester-mismatch"
		frontendID          = "repository-lifecycle-requester-mismatch-frontend"
		backendID           = "repository-lifecycle-requester-mismatch-backend"
		frontendWorktreeID  = "worktree-lifecycle-requester-mismatch-frontend"
		backendWorktreeID   = "worktree-lifecycle-requester-mismatch-backend"
		executorID          = "executor-lifecycle-requester-mismatch"
		incarnationID       = "incarnation-lifecycle-requester-mismatch"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqliteDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqliteDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("new task repository: %v", err)
	}
	store, err := worktree.NewSQLiteStore(sqliteDB, sqliteDB)
	if err != nil {
		t.Fatalf("new worktree store: %v", err)
	}
	log := newTestLogger()
	tasksPath := filepath.Join(t.TempDir(), "tasks")
	worktreeManager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasksPath, BranchPrefix: "kandev/"}, store, log)
	if err != nil {
		t.Fatalf("new worktree manager: %v", err)
	}
	frontendPath := lifecycleRecoveryGitRepository(t, "frontend")
	backendPath := lifecycleRecoveryGitRepository(t, "backend")
	taskRoot := filepath.Join(tasksPath, "recovery")
	frontendOriginalPath := filepath.Join(taskRoot, "frontend")
	backendOriginalPath := filepath.Join(taskRoot, "backend")
	runLifecycleRecoveryGit(t, frontendPath, "worktree", "add", frontendOriginalPath, "feature/recovery")
	runLifecycleRecoveryGit(t, backendPath, "worktree", "add", backendOriginalPath, "feature/recovery")
	seedLifecycleRecoveryStore(t, ctx, taskRepo, lifecycleRecoverySeed{
		workspaceID: workspaceID, taskID: taskID, sessionID: sessionID, environmentID: environmentID,
		frontendRepositoryID: frontendID, backendRepositoryID: backendID, frontendWorktreeID: frontendWorktreeID,
		backendWorktreeID: backendWorktreeID, executorID: executorID, incarnationID: incarnationID,
		frontendRepositoryPath: frontendPath, backendRepositoryPath: backendPath, taskRoot: taskRoot,
		frontendOriginalPath: frontendOriginalPath, backendOriginalPath: backendOriginalPath,
	})
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: attackerWorkspaceID, Name: "Attacker workspace"}); err != nil {
		t.Fatalf("create attacker workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &models.Task{ID: attackerTaskID, WorkspaceID: attackerWorkspaceID, Title: "Attacker task"}); err != nil {
		t.Fatalf("create attacker task: %v", err)
	}
	now := time.Now().UTC()
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: consumerID, TaskID: taskID, QueueIncarnationID: "incarnation-lifecycle-requester-consumer", TaskEnvironmentID: environmentID, ExecutorID: executorID, RepositoryID: frontendID, AgentProfileID: "mock-agent", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create live consumer session: %v", err)
	}
	if err := taskRepo.CreateTurn(ctx, &models.Turn{ID: "turn-lifecycle-requester-mismatch", TaskID: taskID, TaskSessionID: consumerID, StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create live consumer turn: %v", err)
	}
	removeLifecycleRecoveryWorktreeAdmin(t, frontendOriginalPath)
	original, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get original worktrees: %v", err)
	}
	agentsRegistry := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	agent.SetEnabled(true)
	if err := agentsRegistry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	runtime := &capturingWorktreeRuntime{}
	runtimes := lifecycle.NewExecutorRegistry(log)
	runtimes.Register(runtime)
	events := bus.NewMemoryEventBus(log)
	t.Cleanup(events.Close)
	manager := lifecycle.NewManager(agentsRegistry, events, runtimes, nil, nil, nil, lifecycle.ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = manager.Stop() })
	preparers := lifecycle.NewPreparerRegistry(log)
	manager.SetPreparerRegistry(preparers)
	manager.SetWorktreeManager(worktreeManager)
	executor := orchestratorexecutor.NewExecutor(newLifecycleAdapter(manager, agentsRegistry, log), taskRepo, log, orchestratorexecutor.ExecutorConfig{})
	executor.SetSelectedWorktreeRecoveryAdmission(func(admissionCtx context.Context, request worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		if _, updateErr := sqliteDB.ExecContext(admissionCtx, `UPDATE task_sessions SET task_id = ? WHERE id = ?`, attackerTaskID, sessionID); updateErr != nil {
			t.Fatalf("change requester ownership at admission boundary: %v", updateErr)
		}
		return worktreeManager.AdmitRecovery(admissionCtx, request)
	})
	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get requester session: %v", err)
	}
	if _, err := executor.ResumeSession(ctx, session, true); !errors.Is(err, recoveryclaim.ErrClaimMismatch) {
		t.Fatalf("ResumeSession error = %v, want requester identity mismatch", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runtime.calls)
	}
	after, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get worktrees after refusal: %v", err)
	}
	if len(after) != len(original) {
		t.Fatalf("durable worktree count = %d, want %d", len(after), len(original))
	}
	for index, before := range original {
		if after[index].ID != before.ID || after[index].Path != before.Path {
			t.Fatalf("durable worktree changed: before=%+v after=%+v", before, after[index])
		}
	}
	if !lifecycleRecoveryWorktreeAdminMissing(t, frontendOriginalPath) {
		t.Fatal("damaged frontend checkout admin was restored during refusal")
	}
	var claims int
	if err := sqliteDB.GetContext(ctx, &claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, environmentID); err != nil {
		t.Fatalf("count recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable recovery claims = %d, want 0", claims)
	}
}

type lifecycleRecoverySeed struct {
	workspaceID, taskID, sessionID, environmentID, executorID, incarnationID                           string
	frontendRepositoryID, backendRepositoryID, frontendWorktreeID, backendWorktreeID                   string
	frontendRepositoryPath, backendRepositoryPath, taskRoot, frontendOriginalPath, backendOriginalPath string
	backendSlotEmpty                                                                                   bool
}

func seedLifecycleRecoveryStore(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, seed lifecycleRecoverySeed) {
	t.Helper()
	for _, create := range []func() error{
		func() error {
			return repo.CreateWorkspace(ctx, &models.Workspace{ID: seed.workspaceID, Name: "Lifecycle recovery"})
		},
		func() error {
			return repo.CreateTask(ctx, &models.Task{ID: seed.taskID, WorkspaceID: seed.workspaceID, Title: "Recover checkout"})
		},
		func() error {
			return repo.CreateRepository(ctx, &models.Repository{ID: seed.frontendRepositoryID, WorkspaceID: seed.workspaceID, Name: "frontend", LocalPath: seed.frontendRepositoryPath, DefaultBranch: "main", WorktreeBranchPrefix: "feature/"})
		},
		func() error {
			return repo.CreateRepository(ctx, &models.Repository{ID: seed.backendRepositoryID, WorkspaceID: seed.workspaceID, Name: "backend", LocalPath: seed.backendRepositoryPath, DefaultBranch: "main", WorktreeBranchPrefix: "feature/"})
		},
		func() error {
			return repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repository-frontend-" + seed.taskID, TaskID: seed.taskID, RepositoryID: seed.frontendRepositoryID, Position: 0, BaseBranch: "main"})
		},
		func() error {
			return repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repository-backend-" + seed.taskID, TaskID: seed.taskID, RepositoryID: seed.backendRepositoryID, Position: 1, BaseBranch: "main"})
		},
		func() error {
			return repo.CreateExecutor(ctx, &models.Executor{ID: seed.executorID, Name: "worktree", Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive, Resumable: true})
		},
		func() error { return repo.CreateTaskEnvironment(ctx, lifecycleRecoveryEnvironment(seed)) },
		func() error {
			return repo.CreateTaskSession(ctx, &models.TaskSession{ID: seed.sessionID, TaskID: seed.taskID, QueueIncarnationID: seed.incarnationID, TaskEnvironmentID: seed.environmentID, ExecutorID: seed.executorID, RepositoryID: seed.frontendRepositoryID, AgentProfileID: "mock-agent", BaseBranch: "main", State: models.TaskSessionStateCancelled})
		},
	} {
		if err := create(); err != nil {
			t.Fatalf("seed lifecycle recovery store: %v", err)
		}
	}
}

func lifecycleRecoveryEnvironment(seed lifecycleRecoverySeed) *models.TaskEnvironment {
	backend := &models.TaskEnvironmentRepo{ID: "environment-repository-backend-" + seed.taskID, RepositoryID: seed.backendRepositoryID, Position: 1, BranchSlug: "main", Status: "active"}
	if !seed.backendSlotEmpty {
		backend.WorktreeID = seed.backendWorktreeID
		backend.WorktreePath = seed.backendOriginalPath
		backend.WorktreeBranch = "feature/recovery"
	}
	return &models.TaskEnvironment{ID: seed.environmentID, TaskID: seed.taskID, OwnershipGeneration: 1, ExecutorID: seed.executorID, ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady, WorkspacePath: seed.taskRoot, TaskDirName: "recovery", Repos: []*models.TaskEnvironmentRepo{{ID: "environment-repository-frontend-" + seed.taskID, RepositoryID: seed.frontendRepositoryID, Position: 0, BranchSlug: "main", WorktreeID: seed.frontendWorktreeID, WorktreePath: seed.frontendOriginalPath, WorktreeBranch: "feature/recovery", Status: "active"}, backend}}
}

func lifecycleRecoveryGitRepository(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if output, err := exec.Command("git", "init", "-b", "main", path).CombinedOutput(); err != nil {
		t.Fatalf("init repository: %v: %s", err, output)
	}
	runLifecycleRecoveryGit(t, path, "config", "user.email", "recovery@example.test")
	runLifecycleRecoveryGit(t, path, "config", "user.name", "Recovery Test")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runLifecycleRecoveryGit(t, path, "add", "README.md")
	runLifecycleRecoveryGit(t, path, "commit", "-m", "initial")
	runLifecycleRecoveryGit(t, path, "branch", "feature/recovery")
	return path
}

func sameLifecycleRecoveryStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func runLifecycleRecoveryGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func removeLifecycleRecoveryWorktreeAdmin(t *testing.T, worktreePath string) {
	t.Helper()
	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree admin pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove worktree admin: %v", err)
	}
}

func lifecycleRecoveryWorktreeAdminMissing(t *testing.T, worktreePath string) bool {
	t.Helper()
	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree admin pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	_, err = os.Stat(adminPath)
	return errors.Is(err, os.ErrNotExist)
}
