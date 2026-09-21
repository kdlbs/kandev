package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorktreeRecoveryResumeRealStoreFixture(t *testing.T) {
	ctx := context.Background()
	const (
		workspaceID   = "workspace-recovery-real-store"
		taskID        = "task-recovery-real-store"
		sessionID     = "session-recovery-real-store"
		environmentID = "environment-recovery-real-store"
		repositoryID  = "repository-recovery-real-store"
		worktreeID    = "worktree-recovery-real-store"
		executorID    = "executor-recovery-real-store"
	)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqliteDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqliteDB.Close() })
	repo, err := tasksqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("init task store: %v", err)
	}
	store, err := worktree.NewSQLiteStore(sqliteDB, sqliteDB)
	if err != nil {
		t.Fatalf("init worktree store: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	tasksPath := filepath.Join(t.TempDir(), "tasks")
	manager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasksPath, BranchPrefix: "kandev/"}, store, log)
	if err != nil {
		t.Fatalf("new worktree manager: %v", err)
	}
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if output, err := exec.Command("git", "init", "-b", "main", repositoryPath).CombinedOutput(); err != nil {
		t.Fatalf("init temporary git repository: %v: %s", err, output)
	}
	runRealStoreGit(t, repositoryPath, "config", "user.email", "recovery@example.test")
	runRealStoreGit(t, repositoryPath, "config", "user.name", "Recovery Test")
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write repository fixture: %v", err)
	}
	runRealStoreGit(t, repositoryPath, "add", "README.md")
	runRealStoreGit(t, repositoryPath, "commit", "-m", "initial")
	runRealStoreGit(t, repositoryPath, "branch", "feature/recovery")

	originalPath := filepath.Join(tasksPath, "recovery")
	runRealStoreGit(t, repositoryPath, "worktree", "add", originalPath, "feature/recovery")
	removeRealStoreRecoveryAdmin(t, originalPath)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Recovery real store"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Recover checkout"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: "recovery", LocalPath: repositoryPath,
		DefaultBranch: "main", WorktreeBranchPrefix: "feature/",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-recovery-real-store", TaskID: taskID, RepositoryID: repositoryID, BaseBranch: "main",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: executorID, Name: "worktree", Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive, Resumable: true,
	}); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 1, ExecutorID: executorID,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: originalPath, TaskDirName: "recovery", Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-recovery-real-store", RepositoryID: repositoryID, BranchSlug: "main",
			WorktreeID: worktreeID, WorktreePath: originalPath, WorktreeBranch: "feature/recovery", Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-recovery-real-store",
		TaskEnvironmentID: environmentID, ExecutorID: executorID, RepositoryID: repositoryID,
		AgentProfileID: "profile-recovery-real-store", BaseBranch: "main", State: models.TaskSessionStateCancelled,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	launches := 0
	agentManager := &mockAgentManager{launchAgentFunc: func(launchCtx context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		launches++
		claim := worktree.RecoveryClaimFromContext(launchCtx)
		if claim == nil || claim.TaskEnvironmentID != environmentID || claim.SessionID != sessionID || claim.SessionIncarnationID != "incarnation-recovery-real-store" {
			t.Fatalf("lifecycle handoff claim = %+v, want exact durable recovery authority", claim)
		}
		if request.WorktreeID == worktreeID || request.WorktreeID == "" {
			t.Fatalf("lifecycle request worktree ID = %q, want recovered canonical identity", request.WorktreeID)
		}
		return &LaunchAgentResponse{AgentExecutionID: "execution-recovery-real-store", Status: v1.AgentStatusStarting}, nil
	}}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(manager.AdmitRecovery)

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if _, err := exec.ResumeSession(ctx, session, true); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if launches != 1 {
		t.Fatalf("lifecycle launch calls = %d, want 1", launches)
	}
	recovered, err := store.GetWorktreeByID(ctx, worktreeID)
	if err != nil {
		t.Fatalf("GetWorktreeByID(original): %v", err)
	}
	if recovered != nil {
		t.Fatalf("original worktree ID still resolves after recovery: %+v", recovered)
	}
	worktrees, err := store.GetWorktreesByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorktreesByTaskID: %v", err)
	}
	if len(worktrees) != 1 || !strings.Contains(worktrees[0].Path, ".recovered-") || !manager.IsValid(worktrees[0].Path) {
		t.Fatalf("durable recovered worktree = %+v, want one valid rematerialized replacement", worktrees)
	}
	var claims int
	if err := sqliteDB.GetContext(ctx, &claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, environmentID); err != nil {
		t.Fatalf("count released recovery claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("durable recovery claims after lifecycle handoff = %d, want 0", claims)
	}
}

func runRealStoreGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func removeRealStoreRecoveryAdmin(t *testing.T, worktreePath string) {
	t.Helper()
	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree admin pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove worktree admin directory: %v", err)
	}
}

func seedSelectedWorktreeRecoveryEnvironment(
	repo *mockRepository,
	taskID, sessionID string,
	sessionState models.TaskSessionState,
) {
	repo.repositories["repo-recovery"] = &models.Repository{
		ID:                   "repo-recovery",
		Name:                 "recovery",
		LocalPath:            "/repos/recovery",
		WorktreeBranchPrefix: "feature/",
	}
	repo.taskRepositories["task-repo-recovery"] = &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: taskID, RepositoryID: "repo-recovery", Position: 0, BaseBranch: "main",
	}
	repo.executors[models.ExecutorIDWorktree] = &models.Executor{
		ID: models.ExecutorIDWorktree, Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive,
	}
	environmentRepos := []*models.TaskEnvironmentRepo{{
		ID:                "environment-repo-recovery",
		TaskEnvironmentID: "environment-recovery",
		RepositoryID:      "repo-recovery",
		BranchSlug:        "main",
		WorktreeID:        "worktree-recovery",
		WorktreePath:      "/tasks/recovery/recovery",
		WorktreeBranch:    "feature/recovery",
		Status:            "active",
		Position:          0,
	}}
	repo.taskEnvironments["environment-recovery"] = &models.TaskEnvironment{
		ID:                  "environment-recovery",
		TaskID:              taskID,
		OwnershipGeneration: 1,
		ExecutorType:        string(models.ExecutorTypeWorktree),
		Status:              models.TaskEnvironmentStatusReady,
		WorkspacePath:       "/tasks/recovery/recovery",
		TaskDirName:         "recovery_abc",
		Repos:               environmentRepos,
	}
	repo.taskEnvironmentRepos["environment-recovery"] = environmentRepos
	repo.sessions[sessionID] = &models.TaskSession{
		ID:                 sessionID,
		TaskID:             taskID,
		TaskEnvironmentID:  "environment-recovery",
		QueueIncarnationID: "incarnation-recovery",
		AgentProfileID:     "profile-recovery",
		ExecutorID:         models.ExecutorIDWorktree,
		RepositoryID:       "repo-recovery",
		BaseBranch:         "main",
		State:              sessionState,
		StartedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
}

func TestWorktreeRecoveryLaunchIntegration(t *testing.T) {
	const taskID = "task-recovery-launch"
	const sessionID = "session-recovery-launch"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCreated)

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{
		ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery launch",
	}, sessionID, LaunchOptions{
		AgentProfileID: "profile-recovery",
		ExecutorID:     models.ExecutorIDWorktree,
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if admissionCalls != 1 {
		t.Fatalf("selected recovery admission calls = %d, want 1", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func TestWorktreeRecoveryResumeIntegration(t *testing.T) {
	const taskID = "task-recovery-resume"
	const sessionID = "session-recovery-resume"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCancelled)
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery resume"}

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.ResumeSession(context.Background(), repo.sessions[sessionID], false)
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if admissionCalls != 1 {
		t.Fatalf("selected recovery admission calls = %d, want 1", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func TestWorktreeRecoveryResumeAdmitsBeforeStartingState(t *testing.T) {
	const taskID = "task-recovery-resume-start"
	const sessionID = "session-recovery-resume-start"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCancelled)
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery resume"}

	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{AgentExecutionID: "execution-recovery", Status: v1.AgentStatusStarting}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, _ worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		if got := repo.sessions[sessionID].State; got != models.TaskSessionStateCancelled {
			t.Fatalf("durable session state at recovery admission = %q, want prelaunch cancelled", got)
		}
		return nil, nil
	})

	if _, err := exec.ResumeSession(context.Background(), repo.sessions[sessionID], true); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
}

func assertSelectedWorktreeRecoveryRequest(
	t *testing.T,
	req worktree.RecoveryAdmissionRequest,
	taskID, sessionID string,
) {
	t.Helper()
	if req.TaskID != taskID || req.SessionID != sessionID {
		t.Fatalf("admission identity = task %q/session %q, want %q/%q", req.TaskID, req.SessionID, taskID, sessionID)
	}
	if req.SessionIncarnationID != "incarnation-recovery" {
		t.Fatalf("admission session incarnation = %q, want incarnation-recovery", req.SessionIncarnationID)
	}
	if req.TaskEnvironmentID != "environment-recovery" || req.OwnerTaskID != taskID {
		t.Fatalf("admission environment = %q, owner %q, want environment-recovery/%q", req.TaskEnvironmentID, req.OwnerTaskID, taskID)
	}
	if req.OwnershipGeneration != 1 || req.ExecutorType != string(models.ExecutorTypeWorktree) {
		t.Fatalf("admission authority = generation %d/executor %q, want 1/worktree", req.OwnershipGeneration, req.ExecutorType)
	}
	if len(req.Slots) != 1 {
		t.Fatalf("admission slots = %d, want 1", len(req.Slots))
	}
	slot := req.Slots[0]
	if slot.WorktreeID != "worktree-recovery" || slot.RepositoryID != "repo-recovery" || slot.BranchSlug != "main" {
		t.Fatalf("admission slot = %+v, want selected canonical worktree", slot)
	}
}
