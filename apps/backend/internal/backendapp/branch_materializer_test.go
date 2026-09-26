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

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// canonicalTempDir resolves symlinks so macOS /var temp roots pass owned-root guards.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return d
}

// stubRescanner records calls so the materializer test can assert that the
// agentctl rescan was triggered with the expected work_dir (task root) and
// that the frontend-visible "worktree materialized" event also fired.
type stubRescanner struct {
	calls       []stubRescanCall
	notifyCalls []lifecycle.MaterializedWorktree
}

type stubRescanCall struct {
	sessionID string
	workDir   string
}

func (s *stubRescanner) RescanWorkspaceForSession(_ context.Context, sessionID, workDir string, _ ...[]string) error {
	s.calls = append(s.calls, stubRescanCall{sessionID: sessionID, workDir: workDir})
	return nil
}

func (s *stubRescanner) NotifyWorktreeMaterialized(_ context.Context, wt lifecycle.MaterializedWorktree) {
	s.notifyCalls = append(s.notifyCalls, wt)
}

// TestBranchMaterializer_PromotesWorkspacePathAndTriggersRescan is the
// end-to-end happy path: a single-branch task gains a second branch via the
// materializer, which must (1) create the sibling worktree on disk,
// (2) promote task_environments.workspace_path from primary to task root,
// and (3) ping agentctl rescan with the new task root so the trackers
// rebuild without a session restart.
func TestBranchMaterializer_PromotesWorkspacePathAndTriggersRescan(t *testing.T) {
	ctx := context.Background()

	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	t.Logf("taskRoot=%s primary=%s", taskRoot, primaryPath)

	repoSqlite := newMaterializerRepo(t)
	worktreeMgr := newMaterializerWorktreeMgr(t, taskRoot)
	stub := &stubRescanner{}
	mat := &branchMaterializer{
		repo:        repoSqlite,
		worktreeMgr: worktreeMgr,
		rescanner:   stub,
		logger:      newTestLogger(),
	}

	seedMaterializerTask(t, ctx, repoSqlite, repoPath, taskRoot, primaryPath)

	tr := &models.TaskRepository{
		ID:             "tr-branch-2",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "branch-2",
		Position:       1,
		Metadata:       map[string]interface{}{},
	}
	if err := repoSqlite.CreateTaskRepository(ctx, tr); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}

	materialized, err := mat.MaterializeBranch(ctx, "task-1", tr.ID, taskservice.BranchMaterializationTarget{})
	if err != nil {
		t.Fatalf("MaterializeBranch: %v", err)
	}

	wantSiblingDir := filepath.Join(taskRoot, "kandev-branch-2")
	if materialized == nil || materialized.WorktreePath != wantSiblingDir || materialized.TaskWorkspacePath != taskRoot {
		t.Fatalf("materialization result = %#v, want sibling worktree and task root", materialized)
	}
	if _, err := os.Stat(wantSiblingDir); err != nil {
		t.Fatalf("expected sibling worktree at %s: %v", wantSiblingDir, err)
	}

	env, err := repoSqlite.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("re-fetch env: %v", err)
	}
	if env.WorkspacePath != taskRoot {
		t.Errorf("workspace_path = %q, want %q (task root)", env.WorkspacePath, taskRoot)
	}

	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 rescan call, got %d", len(stub.calls))
	}
	if stub.calls[0].workDir != taskRoot {
		t.Errorf("rescan work_dir = %q, want %q", stub.calls[0].workDir, taskRoot)
	}
	if stub.calls[0].sessionID != "session-1" {
		t.Errorf("rescan session_id = %q, want session-1", stub.calls[0].sessionID)
	}

	if len(stub.notifyCalls) != 1 {
		t.Fatalf("expected 1 NotifyWorktreeMaterialized call, got %d", len(stub.notifyCalls))
	}
	notify := stub.notifyCalls[0]
	if notify.TaskID != "task-1" || notify.SessionID != "session-1" {
		t.Errorf("notify identifiers = %+v, want task-1/session-1", notify)
	}
	if notify.WorktreePath != wantSiblingDir {
		t.Errorf("notify worktree path = %q, want %q", notify.WorktreePath, wantSiblingDir)
	}
	if notify.BranchSlug != "branch-2" {
		t.Errorf("notify branch_slug = %q, want branch-2", notify.BranchSlug)
	}
}

// TestBranchMaterializer_SecondBranchKeepsTaskRootPromoted exercises the
// idempotent promotion path: a task that's already been promoted to task
// root must not be flipped back to a primary path on subsequent adds.
func TestBranchMaterializer_SecondBranchKeepsTaskRootPromoted(t *testing.T) {
	ctx := context.Background()

	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repoSqlite := newMaterializerRepo(t)
	worktreeMgr := newMaterializerWorktreeMgr(t, taskRoot)
	stub := &stubRescanner{}
	mat := &branchMaterializer{
		repo:        repoSqlite,
		worktreeMgr: worktreeMgr,
		rescanner:   stub,
		logger:      newTestLogger(),
	}

	seedMaterializerTask(t, ctx, repoSqlite, repoPath, taskRoot, primaryPath)

	tr2 := &models.TaskRepository{
		ID: "tr-branch-2", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "branch-2", Position: 1,
		Metadata: map[string]interface{}{},
	}
	if err := repoSqlite.CreateTaskRepository(ctx, tr2); err != nil {
		t.Fatalf("CreateTaskRepository branch-2: %v", err)
	}
	if _, err := mat.MaterializeBranch(ctx, "task-1", tr2.ID, taskservice.BranchMaterializationTarget{}); err != nil {
		t.Fatalf("MaterializeBranch branch-2: %v", err)
	}

	tr3 := &models.TaskRepository{
		ID: "tr-branch-3", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "branch-3", Position: 2,
		Metadata: map[string]interface{}{},
	}
	if err := repoSqlite.CreateTaskRepository(ctx, tr3); err != nil {
		t.Fatalf("CreateTaskRepository branch-3: %v", err)
	}
	if _, err := mat.MaterializeBranch(ctx, "task-1", tr3.ID, taskservice.BranchMaterializationTarget{}); err != nil {
		t.Fatalf("MaterializeBranch branch-3: %v", err)
	}

	env, err := repoSqlite.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("re-fetch env: %v", err)
	}
	if env.WorkspacePath != taskRoot {
		t.Errorf("workspace_path = %q, want %q (must stay at task root)", env.WorkspacePath, taskRoot)
	}
	for _, dir := range []string{"kandev-branch-2", "kandev-branch-3"} {
		p := filepath.Join(taskRoot, dir)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing sibling worktree dir %s: %v", p, err)
		}
	}
	// Second call should also trigger rescan — the trackers may need to
	// add the new sibling.
	if len(stub.calls) != 2 {
		t.Fatalf("expected 2 rescan calls (one per add_branch), got %d", len(stub.calls))
	}
	for i, call := range stub.calls {
		if call.workDir != taskRoot {
			t.Errorf("call %d workDir = %q, want %q", i, call.workDir, taskRoot)
		}
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
func TestBranchMaterializer_InheritedSessionPersistsCanonicalInventory(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repoSqlite := newMaterializerRepo(t)
	worktreeMgr := newPersistingMaterializerWorktreeMgr(t, taskRoot, repoSqlite)
	stub := &stubRescanner{}
	mat := &branchMaterializer{
		repo:        repoSqlite,
		worktreeMgr: worktreeMgr,
		rescanner:   stub,
		logger:      newTestLogger(),
	}

	seedMaterializerTask(t, ctx, repoSqlite, repoPath, taskRoot, primaryPath)
	branch := seedInheritedMaterializerChild(t, ctx, repoSqlite)

	materialized, err := mat.MaterializeBranch(ctx, "task-child", branch.ID, taskservice.BranchMaterializationTarget{
		SessionID: "session-child", TaskEnvironmentID: "env-1",
	})
	if err != nil {
		t.Fatalf("MaterializeBranch inherited: %v", err)
	}
	wantPath := filepath.Join(taskRoot, "kandev-branch-inherited")
	if materialized == nil || materialized.WorktreePath != wantPath || materialized.TaskWorkspacePath != taskRoot {
		t.Fatalf("materialization result = %#v, want inherited sibling and task root", materialized)
	}

	env, err := repoSqlite.GetTaskEnvironment(ctx, "env-1")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	var persisted *models.TaskEnvironmentRepo
	for _, envRepo := range env.Repos {
		if envRepo.RepositoryID == "repo-1" && envRepo.BranchSlug == "branch-inherited" {
			persisted = envRepo
			break
		}
	}
	if persisted == nil || persisted.WorktreePath != wantPath {
		t.Fatalf("canonical inherited inventory = %#v, want branch at %q", persisted, wantPath)
	}
	if len(stub.calls) != 1 || stub.calls[0].sessionID != "session-child" || stub.calls[0].workDir != taskRoot {
		t.Fatalf("rescan calls = %#v, want inherited child session and task root", stub.calls)
	}
	if len(stub.notifyCalls) != 1 || stub.notifyCalls[0].TaskID != "task-child" || stub.notifyCalls[0].SessionID != "session-child" {
		t.Fatalf("materialized notifications = %#v, want inherited child identity", stub.notifyCalls)
	}
}

func TestBranchMaterializer_InheritedEnvironmentFailuresFailClosed(t *testing.T) {
	now := time.Now().UTC()
	archivedAt := now
	ready := func() *models.TaskEnvironment {
		return &models.TaskEnvironment{
			ID: "env-parent", TaskID: "task-parent", ExecutorType: string(models.ExecutorTypeWorktree),
			Status: models.TaskEnvironmentStatusReady, TaskDirName: "task-root",
		}
	}
	tests := []struct {
		name         string
		environment  *models.TaskEnvironment
		environmentE error
		owner        *models.Task
		ownerE       error
		wantCause    error
	}{
		{name: "missing environment", environmentE: taskrepo.ErrTaskEnvironmentNotFound},
		{name: "unverifiable owner", environment: ready(), ownerE: context.Canceled, wantCause: context.Canceled},
		{name: "archived owner", environment: ready(), owner: &models.Task{ID: "task-parent", ArchivedAt: &archivedAt}},
		{name: "unprovisioned environment", environment: func() *models.TaskEnvironment {
			env := ready()
			env.Status = models.TaskEnvironmentStatusCreating
			env.TaskDirName = ""
			return env
		}(), owner: &models.Task{ID: "task-parent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &branchMaterializerRepoOverrides{
				getTaskEnvironmentByTaskID: func(context.Context, string) (*models.TaskEnvironment, error) { return nil, nil },
				getTaskEnvironment:         func(context.Context, string) (*models.TaskEnvironment, error) { return tt.environment, tt.environmentE },
				getTask:                    func(context.Context, string) (*models.Task, error) { return tt.owner, tt.ownerE },
			}
			materializer := &branchMaterializer{repo: repo, logger: newTestLogger()}
			session := &models.TaskSession{ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-parent"}
			if _, err := materializer.resolveMaterializationEnvironment(context.Background(), "task-child", session); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
				t.Fatalf("resolveMaterializationEnvironment() error = %v, want ErrWorkspaceReuseUnsafe", err)
			} else if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Fatalf("resolveMaterializationEnvironment() error = %v, want preserved cause %v", err, tt.wantCause)
			}
		})
	}
}

func TestBranchMaterializer_LiveSessionBindingOverridesStaleOwnedEnvironment(t *testing.T) {
	bound := &models.TaskEnvironment{
		ID: "env-parent", TaskID: "task-parent", ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, TaskDirName: "task-root",
	}
	repo := &branchMaterializerRepoOverrides{
		getTaskEnvironmentByTaskID: func(context.Context, string) (*models.TaskEnvironment, error) {
			return &models.TaskEnvironment{
				ID: "env-child-stale", TaskID: "task-child", ExecutorType: string(models.ExecutorTypeWorktree),
				Status: models.TaskEnvironmentStatusReady, TaskDirName: "stale-root",
			}, nil
		},
		getTaskEnvironment: func(context.Context, string) (*models.TaskEnvironment, error) { return bound, nil },
		getTask:            func(context.Context, string) (*models.Task, error) { return &models.Task{ID: "task-parent"}, nil },
	}
	materializer := &branchMaterializer{repo: repo, logger: newTestLogger()}
	session := &models.TaskSession{ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-parent"}

	got, err := materializer.resolveMaterializationEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveMaterializationEnvironment: %v", err)
	}
	if got == nil || got.ID != "env-parent" {
		t.Fatalf("resolved environment = %+v, want live session binding env-parent", got)
	}
}

func TestBranchMaterializer_RejectsSessionReboundBeforeCreate(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repoSqlite := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repoSqlite, repoPath, taskRoot, primaryPath)
	branch := seedInheritedMaterializerChild(t, ctx, repoSqlite)
	repo := &branchMaterializerRepoOverrides{branchMaterializerRepo: repoSqlite}
	repo.listTaskSessions = func(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
		sessions, err := repoSqlite.ListTaskSessions(ctx, taskID)
		if err != nil {
			return nil, err
		}
		repo.listCalls++
		if repo.listCalls > 1 {
			for i, session := range sessions {
				copy := *session
				copy.TaskEnvironmentID = "env-rebound"
				sessions[i] = &copy
			}
		}
		return sessions, nil
	}
	materializer := &branchMaterializer{
		repo: repo, worktreeMgr: newPersistingMaterializerWorktreeMgr(t, taskRoot, repoSqlite), logger: newTestLogger(),
	}
	_, err := materializer.MaterializeBranch(ctx, "task-child", branch.ID, taskservice.BranchMaterializationTarget{
		SessionID: "session-child", TaskEnvironmentID: "env-1",
	})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("MaterializeBranch rebound error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if repo.listCalls != 2 {
		t.Fatalf("session list calls = %d, want resolution plus immediate revalidation", repo.listCalls)
	}
	if _, statErr := os.Stat(filepath.Join(taskRoot, "kandev-branch-inherited")); !os.IsNotExist(statErr) {
		t.Fatalf("rebound materialization created a worktree: %v", statErr)
	}
}

type branchMaterializerRepoOverrides struct {
	branchMaterializerRepo
	getTask                    func(context.Context, string) (*models.Task, error)
	getTaskEnvironment         func(context.Context, string) (*models.TaskEnvironment, error)
	getTaskEnvironmentByTaskID func(context.Context, string) (*models.TaskEnvironment, error)
	listTaskSessions           func(context.Context, string) ([]*models.TaskSession, error)
	listCalls                  int
}

func (r *branchMaterializerRepoOverrides) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if r.getTask != nil {
		return r.getTask(ctx, id)
	}
	return r.branchMaterializerRepo.GetTask(ctx, id)
}

func (r *branchMaterializerRepoOverrides) GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error) {
	if r.getTaskEnvironment != nil {
		return r.getTaskEnvironment(ctx, id)
	}
	return r.branchMaterializerRepo.GetTaskEnvironment(ctx, id)
}

func (r *branchMaterializerRepoOverrides) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	if r.getTaskEnvironmentByTaskID != nil {
		return r.getTaskEnvironmentByTaskID(ctx, taskID)
	}
	return r.branchMaterializerRepo.GetTaskEnvironmentByTaskID(ctx, taskID)
}

func (r *branchMaterializerRepoOverrides) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if r.listTaskSessions != nil {
		return r.listTaskSessions(ctx, taskID)
	}
	return r.branchMaterializerRepo.ListTaskSessions(ctx, taskID)
}

func TestTaskRepositoryBranchTemplatePrefersPolicySnapshot(t *testing.T) {
	repo := &models.Repository{WorktreeBranchTemplate: "feature/{title}"}
	legacy := &models.TaskRepository{}
	if got := taskRepositoryBranchTemplate(repo, legacy); got != "feature/{title}" {
		t.Fatalf("legacy template = %q, want repository fallback", got)
	}
	policy := &models.TaskRepository{BranchPolicyBranchTemplate: "bugfix/{title}-{suffix}"}
	if got := taskRepositoryBranchTemplate(repo, policy); got != "bugfix/{title}-{suffix}" {
		t.Fatalf("policy template = %q, want snapshot template", got)
	}
}

// setupMaterializerScenario creates a bare origin repo + a clone that
// serves as the "primary" worktree at <task-root>/kandev/. Returns the
// repository path, the task root, and the primary worktree path.
func setupMaterializerScenario(t *testing.T) (repoPath, taskRoot, primaryPath string) {
	t.Helper()

	tmp := canonicalTempDir(t)
	bareDir := filepath.Join(tmp, "origin.git")
	runGit(t, tmp, "init", "--bare", "-b", "main", bareDir)

	repoPath = filepath.Join(tmp, "repo")
	cmd := exec.Command("git", "clone", bareDir, repoPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("initial\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial")
	runGit(t, repoPath, "push", "origin", "main")

	taskRoot = filepath.Join(tmp, "tasks", "task-1_aaa")
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatalf("mkdir taskRoot: %v", err)
	}
	primaryPath = filepath.Join(taskRoot, kandevName)
	// Simulate the existing primary worktree by adding it as a git worktree
	// off the repo. The materializer doesn't depend on this being a real
	// worktree (it only looks at task_environments.workspace_path), but
	// the test asserts the on-disk layout afterwards.
	runGit(t, repoPath, "worktree", "add", "-b", "feature/initial", primaryPath, "main")
	return repoPath, taskRoot, primaryPath
}

// newMaterializerRepo opens an in-memory sqlite repo wired with the task
// schema used by the service + materializer.
func newMaterializerRepo(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "kandev.db")
	conn, err := db.OpenSQLite(dbFile)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Logf("close db: %v", err)
		}
	})
	repo, cleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repo provide: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("repo cleanup: %v", err)
		}
	})
	return repo
}

// newMaterializerWorktreeMgr builds a worktree.Manager rooted at taskRoot's
// parent so the manager's TasksBasePath aligns with our scenario.
func newMaterializerWorktreeMgr(t *testing.T, taskRoot string) *worktree.Manager {
	t.Helper()
	cfg := worktree.Config{Enabled: true, TasksBasePath: filepath.Dir(taskRoot), BranchPrefix: "feature/"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("worktree config: %v", err)
	}
	mgr, err := worktree.NewManager(cfg, nil, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func newPersistingMaterializerWorktreeMgr(
	t *testing.T,
	taskRoot string,
	repo *sqliterepo.Repository,
) *worktree.Manager {
	t.Helper()
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	store, err := worktree.NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	cfg := worktree.Config{Enabled: true, TasksBasePath: filepath.Dir(taskRoot), BranchPrefix: "feature/"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("worktree config: %v", err)
	}
	mgr, err := worktree.NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

// seedMaterializerTask inserts the task / workspace / repository /
// task_environment rows the materializer relies on.
func seedMaterializerTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, repoPath, taskRoot, primaryPath string) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws-1", Name: kandevName,
		LocalPath: repoPath, DefaultBranch: "main", WorktreeBranchPrefix: "feature/",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		Title: "Add Multi Branch", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr-primary", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "main", Position: 0, Metadata: map[string]interface{}{},
	}); err != nil {
		t.Fatalf("CreateTaskRepository primary: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", TaskEnvironmentID: "env-1",
		State:     models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1",
		TaskDirName:   filepath.Base(taskRoot),
		WorkspacePath: primaryPath,
		ExecutorType:  "worktree", Status: "ready",
		CreatedAt: now, UpdatedAt: now,
		Repos: []*models.TaskEnvironmentRepo{
			{ID: "env-1-repo-primary", RepositoryID: "repo-1", WorktreePath: primaryPath, CreatedAt: now},
		},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
}

func seedInheritedMaterializerChild(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
) *models.TaskRepository {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-1", ParentID: "task-1",
		Title: "Inherited Child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}); err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr-child-primary", TaskID: "task-child", RepositoryID: "repo-1",
		BaseBranch: "main", Position: 0, Metadata: map[string]interface{}{},
	}); err != nil {
		t.Fatalf("CreateTaskRepository child primary: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession child: %v", err)
	}
	branch := &models.TaskRepository{
		ID: "tr-child-branch", TaskID: "task-child", RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "branch-inherited", Position: 1,
		Metadata: map[string]interface{}{},
	}
	if err := repo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatalf("CreateTaskRepository child branch: %v", err)
	}
	return branch
}

// runGit runs `git` in dir, failing the test on error. Inlined here (rather
// than reused from the worktree package's helper) to keep this package
// independent of internal-test exports.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}
