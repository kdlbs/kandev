package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6
func TestMissingCheckoutRecoveryLaunchAndResume(t *testing.T) {
	for _, route := range []string{"prepare_existing_environment", "prepared_launch", "resume"} {
		t.Run(route, func(t *testing.T) {
			fixture := newExecutorMissingCheckoutRecoveryFixture(t, route)
			observed := make(chan error, 1)
			agentManager := &mockAgentManager{
				launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					err := fixture.observeRuntimeStart(ctx, req)
					observed <- err
					if err != nil {
						return nil, err
					}
					return &LaunchAgentResponse{
						AgentExecutionID: "execution-missing-checkout",
						ContainerID:      "container-missing-checkout",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
			}
			repo := newMockRepository()
			seedSelectedWorktreeRecoveryEnvironment(repo, fixture.taskID, fixture.sessionID, fixture.sessionState)
			configureExecutorMissingCheckoutEnvironment(repo, fixture)
			executor := newTestExecutor(t, agentManager, repo)
			executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)

			switch route {
			case "prepare_existing_environment":
				repo.tasks[fixture.taskID] = &models.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Missing checkout recovery"}
				preparedSessionID, err := executor.PrepareSessionForExistingEnvironment(
					context.Background(), &v1.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Missing checkout recovery"},
					"profile-recovery", models.ExecutorIDWorktree, "", "", fixture.environmentID,
				)
				if err != nil {
					t.Fatalf("PrepareSessionForExistingEnvironment: %v", err)
				}
				if preparedSessionID == "" {
					t.Fatal("PrepareSessionForExistingEnvironment returned an empty session ID")
				}
				if err := fixture.assertRestoredCheckout(); err != nil {
					t.Fatalf("prepared environment checkout: %v", err)
				}
			case "prepared_launch":
				_, err := executor.LaunchPreparedSession(context.Background(), &v1.Task{
					ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Missing checkout recovery",
				}, fixture.sessionID, LaunchOptions{
					AgentProfileID: "profile-recovery", ExecutorID: models.ExecutorIDWorktree,
					Prompt: "Verify the restored workspace", StartAgent: true,
				})
				if err != nil {
					t.Fatalf("LaunchPreparedSession: %v", err)
				}
			case "resume":
				repo.tasks[fixture.taskID] = &models.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Missing checkout recovery"}
				if _, err := executor.ResumeSession(context.Background(), repo.sessions[fixture.sessionID], true); err != nil {
					t.Fatalf("ResumeSession: %v", err)
				}
			}

			if route != "prepare_existing_environment" {
				select {
				case err := <-observed:
					if err != nil {
						t.Fatalf("runtime start observation: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for the runtime startup boundary")
				}
			}
			claim, err := fixture.store.GetTaskEnvironmentRecoveryClaim(context.Background(), fixture.environmentID)
			if err != nil {
				t.Fatalf("read recovery claim after runtime start: %v", err)
			}
			if claim != nil {
				t.Fatalf("recovery claim remained after launch returned: %+v", claim)
			}
		})
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
func TestMissingCheckoutRecoveryPreflightCarriesExplicitBranchReplacement(t *testing.T) {
	fixture := newExecutorMissingCheckoutRecoveryFixture(t, "resume")
	originPath := filepath.Join(t.TempDir(), "origin.git")
	runExecutorRecoveryGit(t, t.TempDir(), "init", "--bare", "--initial-branch=main", originPath)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "remote", "add", "origin", originPath)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "push", "-u", "origin", "main")
	runExecutorRecoveryGit(t, fixture.repositoryPath, "push", "origin", fixture.branch)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "push", "origin", "--delete", fixture.branch)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "fetch", "--prune", "origin")
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, fixture.taskID, fixture.sessionID, fixture.sessionState)
	configureExecutorMissingCheckoutEnvironment(repo, fixture)
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)
	session := repo.sessions[fixture.sessionID]
	if session == nil {
		t.Fatal("selected recovery session is missing from the repository fixture")
	}
	admission, err := executor.PreflightSessionWorktreeRecovery(context.Background(), fixture.taskID, session, true)
	if err != nil {
		t.Fatalf("preflight explicit new-branch recovery: %v", err)
	}
	if admission == nil || admission.Claim() == nil {
		t.Fatalf("explicit new-branch preflight returned no durable admission: %+v", admission)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release explicit branch-replacement admission: %v", err)
		}
	}()
	if _, err := os.Lstat(fixture.worktreePath); !os.IsNotExist(err) {
		t.Fatalf("branch-replacement preflight created a checkout before authorization: %v", err)
	}
	created, err := fixture.manager.Create(worktree.WithRecoveryAdmission(context.Background(), admission), worktree.CreateRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath, BaseBranch: "main",
		WorktreeID: fixture.worktreeID, TaskDirName: fixture.taskDirName, RepoName: "repository",
		BranchSlug: fixture.branchSlug, AllowBranchReplacement: true,
	})
	if err != nil {
		t.Fatalf("explicit branch replacement after real selected-environment preflight: %v", err)
	}
	if created == nil || created.Branch == fixture.branch || created.Branch == "" {
		t.Fatalf("explicit branch replacement result = %+v, want a new branch", created)
	}
	if _, err := os.Stat(created.Path); err != nil {
		t.Fatalf("replacement checkout is unavailable at %q: %v", created.Path, err)
	}
}

type executorMissingCheckoutRecoveryFixture struct {
	taskID         string
	sessionID      string
	workspaceID    string
	environmentID  string
	worktreeID     string
	taskDirName    string
	repositoryID   string
	branchSlug     string
	branch         string
	repositoryPath string
	worktreePath   string
	branchHead     string
	sessionState   models.TaskSessionState
	store          *worktree.SQLiteStore
	manager        *worktree.Manager
	config         worktree.Config
}

func newExecutorMissingCheckoutRecoveryFixture(t *testing.T, route string) *executorMissingCheckoutRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &executorMissingCheckoutRecoveryFixture{
		taskID: "task-missing-checkout-" + route, sessionID: "session-missing-checkout-" + route,
		workspaceID: "workspace-missing-checkout", environmentID: "environment-recovery",
		worktreeID: "worktree-recovery", taskDirName: "task-missing-checkout-root-" + route,
		repositoryID: "repo-recovery", branchSlug: "main", branch: "feature/recovery",
		sessionState: models.TaskSessionStateCreated,
	}
	if route == "resume" || route == "prepare_existing_environment" {
		fixture.sessionState = models.TaskSessionStateCancelled
	}

	dbConn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: fixture.workspaceID, Name: fixture.workspaceID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	fixture.repositoryPath = initExecutorRecoveryGitRepository(t)
	if err := taskRepo.CreateRepository(ctx, &models.Repository{
		ID: fixture.repositoryID, WorkspaceID: fixture.workspaceID, Name: "repository",
		SourceType: "local", LocalPath: fixture.repositoryPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &models.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Missing checkout recovery"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskRepo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-recovery-" + route, TaskID: fixture.taskID,
		RepositoryID: fixture.repositoryID, BaseBranch: "main", Position: 0,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}

	basePath := filepath.Join(t.TempDir(), "tasks")
	cfg := worktree.Config{TasksBasePath: basePath}
	fixture.config = cfg
	fixture.worktreePath, err = cfg.TaskWorktreePath(fixture.taskDirName, "repository", fixture.branchSlug)
	if err != nil {
		t.Fatalf("build canonical worktree path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.worktreePath), 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := workspaces.WriteOwnershipMarker(filepath.Dir(fixture.worktreePath), workspaces.OwnershipMarker{
		TaskID: fixture.taskID, TaskDirName: fixture.taskDirName, LayoutVersion: workspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write task-root ownership marker: %v", err)
	}
	runExecutorRecoveryGit(t, fixture.repositoryPath, "branch", fixture.branch, "main")
	runExecutorRecoveryGit(t, fixture.repositoryPath, "worktree", "add", fixture.worktreePath, fixture.branch)
	fixture.branchHead = strings.TrimSpace(runExecutorRecoveryGit(t, fixture.repositoryPath, "rev-parse", "refs/heads/"+fixture.branch))
	runExecutorRecoveryGit(t, fixture.repositoryPath, "worktree", "remove", "--force", fixture.worktreePath)
	runExecutorRecoveryGit(t, fixture.repositoryPath, "worktree", "prune")

	if err := taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: fixture.environmentID, TaskID: fixture.taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: fixture.worktreePath, TaskDirName: fixture.taskDirName,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-recovery-" + route, TaskEnvironmentID: fixture.environmentID,
			RepositoryID: fixture.repositoryID, BranchSlug: fixture.branchSlug, WorktreeID: fixture.worktreeID,
			WorktreePath: fixture.worktreePath, WorktreeBranch: fixture.branch, Status: "active", Position: 0,
		}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: fixture.sessionID, TaskID: fixture.taskID, TaskEnvironmentID: fixture.environmentID,
		State: models.TaskSessionStateCancelled,
	}); err != nil {
		t.Fatalf("create durable session: %v", err)
	}
	fixture.store, err = worktree.NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("create SQLite worktree store: %v", err)
	}
	if err := fixture.store.CreateWorktree(ctx, &worktree.Worktree{
		ID: fixture.worktreeID, SessionID: fixture.sessionID, TaskID: fixture.taskID, TaskDirName: fixture.taskDirName,
		TaskEnvironmentID: fixture.environmentID, RepositoryID: fixture.repositoryID, BranchSlug: fixture.branchSlug,
		RepositoryPath: fixture.repositoryPath, Path: fixture.worktreePath, Branch: fixture.branch,
		BaseBranch: "main", Status: worktree.StatusActive,
	}); err != nil {
		t.Fatalf("create canonical worktree row: %v", err)
	}
	fixture.manager, err = worktree.NewManager(cfg, fixture.store, logger.Default())
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	return fixture
}

func configureExecutorMissingCheckoutEnvironment(repo *mockRepository, fixture *executorMissingCheckoutRecoveryFixture) {
	env := repo.taskEnvironments[fixture.environmentID]
	env.TaskID = fixture.taskID
	env.TaskDirName = fixture.taskDirName
	env.WorkspacePath = fixture.worktreePath
	env.OwnershipGeneration = 1
	env.ExecutorType = string(models.ExecutorTypeWorktree)
	env.Status = models.TaskEnvironmentStatusReady
	for _, row := range env.Repos {
		row.WorktreeID = fixture.worktreeID
		row.WorktreePath = fixture.worktreePath
		row.WorktreeBranch = fixture.branch
		row.RepositoryID = fixture.repositoryID
		row.BranchSlug = fixture.branchSlug
	}
	for _, rows := range repo.taskEnvironmentRepos {
		for _, row := range rows {
			if row.TaskEnvironmentID == fixture.environmentID {
				row.WorktreeID = fixture.worktreeID
				row.WorktreePath = fixture.worktreePath
				row.WorktreeBranch = fixture.branch
				row.RepositoryID = fixture.repositoryID
				row.BranchSlug = fixture.branchSlug
			}
		}
	}
	repo.repositories[fixture.repositoryID].LocalPath = fixture.repositoryPath
	repo.taskRepositories["task-repo-recovery"].TaskID = fixture.taskID
	repo.taskRepositories["task-repo-recovery"].RepositoryID = fixture.repositoryID
	repo.taskRepositories["task-repo-recovery"].BaseBranch = "main"
	if session := repo.sessions[fixture.sessionID]; session != nil {
		session.State = fixture.sessionState
		session.TaskEnvironmentID = fixture.environmentID
		session.TaskID = fixture.taskID
		session.ExecutorID = models.ExecutorIDWorktree
		session.RepositoryID = fixture.repositoryID
		session.BaseBranch = "main"
	}
}

func (f *executorMissingCheckoutRecoveryFixture) observeRuntimeStart(ctx context.Context, req *LaunchAgentRequest) error {
	if req.TaskID != f.taskID || req.SessionID != f.sessionID || req.TaskEnvironmentID != f.environmentID ||
		req.WorktreeID != f.worktreeID || req.RepositoryPath != f.repositoryPath {
		return fmt.Errorf("runtime request identity = task %q session %q env %q worktree %q repository %q", req.TaskID,
			req.SessionID, req.TaskEnvironmentID, req.WorktreeID, req.RepositoryPath)
	}
	claim, err := f.store.GetTaskEnvironmentRecoveryClaim(ctx, f.environmentID)
	if err != nil || claim == nil || claim.SessionID != f.sessionID {
		return fmt.Errorf("runtime startup did not retain its selected-environment claim: claim=%+v error=%v", claim, err)
	}
	attached, err := f.manager.Create(ctx, worktree.CreateRequest{
		TaskID: f.taskID, SessionID: f.sessionID, TaskEnvironmentID: f.environmentID,
		RepositoryID: f.repositoryID, RepositoryPath: f.repositoryPath, BaseBranch: "main",
		WorktreeID: f.worktreeID, BranchIdentitySlug: f.branchSlug, ReuseRequired: true,
	})
	if err != nil {
		return fmt.Errorf("attach canonical worktree before runtime start: %w", err)
	}
	if attached == nil || attached.ID != f.worktreeID || filepath.Clean(attached.Path) != filepath.Clean(f.worktreePath) || attached.Branch != f.branch {
		return fmt.Errorf("runtime attached worktree = %+v, want ID %q path %q branch %q", attached, f.worktreeID, f.worktreePath, f.branch)
	}
	return f.assertRestoredCheckout()
}

func (f *executorMissingCheckoutRecoveryFixture) assertRestoredCheckout() error {
	head, err := executorRecoveryGit(f.worktreePath, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != f.branchHead {
		return fmt.Errorf("runtime checkout head = %q, want %q (error %v)", strings.TrimSpace(head), f.branchHead, err)
	}
	branch, err := executorRecoveryGit(f.worktreePath, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) != f.branch {
		return fmt.Errorf("runtime checkout branch = %q, want %q (error %v)", strings.TrimSpace(branch), f.branch, err)
	}
	return nil
}

func initExecutorRecoveryGitRepository(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create repository path: %v", err)
	}
	runExecutorRecoveryGit(t, path, "init", "--initial-branch=main")
	runExecutorRecoveryGit(t, path, "config", "user.email", "kandev@example.test")
	runExecutorRecoveryGit(t, path, "config", "user.name", "Kandev Test")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("recovery integration\n"), 0o644); err != nil {
		t.Fatalf("write repository seed: %v", err)
	}
	runExecutorRecoveryGit(t, path, "add", "README.md")
	runExecutorRecoveryGit(t, path, "commit", "-m", "seed recovery integration repository")
	return path
}

func runExecutorRecoveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := executorRecoveryGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, output)
	}
	return output
}

func executorRecoveryGit(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}
