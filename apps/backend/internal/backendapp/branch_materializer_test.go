package backendapp

import (
	"context"
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
	"github.com/kandev/kandev/internal/worktree"
)

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

	if err := mat.MaterializeBranch(ctx, "task-1", tr.ID); err != nil {
		t.Fatalf("MaterializeBranch: %v", err)
	}

	wantSiblingDir := filepath.Join(taskRoot, "kandev-branch-2")
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
	if err := mat.MaterializeBranch(ctx, "task-1", tr2.ID); err != nil {
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
	if err := mat.MaterializeBranch(ctx, "task-1", tr3.ID); err != nil {
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

// setupMaterializerScenario creates a bare origin repo + a clone that
// serves as the "primary" worktree at <task-root>/kandev/. Returns the
// repository path, the task root, and the primary worktree path.
func setupMaterializerScenario(t *testing.T) (repoPath, taskRoot, primaryPath string) {
	t.Helper()

	tmp := t.TempDir()
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
	primaryPath = filepath.Join(taskRoot, "kandev")
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
		ID: "repo-1", WorkspaceID: "ws-1", Name: "kandev",
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
		ID: "session-1", TaskID: "task-1",
		State:     models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1",
		TaskDirName:   filepath.Base(taskRoot),
		WorkspacePath: primaryPath,
		WorktreePath:  primaryPath,
		ExecutorType:  "worktree", Status: "ready",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
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
