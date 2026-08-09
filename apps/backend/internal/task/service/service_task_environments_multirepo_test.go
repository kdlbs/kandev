package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// realWorktreeDestroyer implements EnvironmentDestroyer over a real
// worktree.Manager so teardownEnvironmentResources is exercised against real
// SQLite persistence and real git worktrees on disk, not a mock.
type realWorktreeDestroyer struct {
	mgr *worktree.Manager
}

func (d *realWorktreeDestroyer) DestroyContainer(context.Context, string) error { return nil }
func (d *realWorktreeDestroyer) DestroySandbox(context.Context, string, string) error {
	return nil
}
func (d *realWorktreeDestroyer) DestroyWorktreeAt(
	ctx context.Context, worktreeID, worktreePath, repositoryID string, excludeSessionIDs []string,
) error {
	return d.mgr.RemoveAt(ctx, worktreeID, worktreePath, repositoryID, excludeSessionIDs)
}
func (d *realWorktreeDestroyer) PushEnvironmentBranch(context.Context, *models.TaskEnvironment) error {
	return nil
}
func (d *realWorktreeDestroyer) GetContainerLiveStatus(context.Context, string) (*ContainerLiveStatus, error) {
	return nil, nil
}

// newRealWorktreeTestService wires a real SQLite-backed task repository and a
// real worktree.Manager (real git worktrees on disk), mirroring
// internal/worktree/manager_cleanup_shared_reference_test.go's
// newReferenceCleanupTestManager. createTestService (service_test.go) does
// not wire a real worktree manager, so this is a dedicated helper.
func newRealWorktreeTestService(t *testing.T) (*Service, *worktree.Manager, *sqliterepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	store, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("create worktree store: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	cfg := worktree.Config{Enabled: true, TasksBasePath: t.TempDir(), BranchPrefix: "kandev/"}
	mgr, err := worktree.NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	mgr.SetRepositoryProvider(worktree.NewRepositoryAdapter(repo))

	svc := &Service{logger: log}
	svc.SetEnvironmentDestroyer(&realWorktreeDestroyer{mgr: mgr})

	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "workspace", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	return svc, mgr, repo
}

// initSimpleGitRepo creates a minimal local git repository (no remote) with
// one commit on main, suitable as a worktree source.
func initSimpleGitRepo(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	runGitTestCmd(t, repoPath, "init", "-b", "main")
	runGitTestCmd(t, repoPath, "config", "user.email", "test@example.com")
	runGitTestCmd(t, repoPath, "config", "user.name", "Test User")
	runGitTestCmd(t, repoPath, "config", "commit.gpgsign", "false")

	readme := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readme, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGitTestCmd(t, repoPath, "add", "README.md")
	runGitTestCmd(t, repoPath, "commit", "-m", "initial commit")
	return repoPath
}

func runGitTestCmd(t *testing.T, repoPath string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// TestTeardownEnvironmentResources_MultiRepoRemovesEveryWorktreeFromDisk is
// the regression test for the multi-repo worktree disk leak: once a task's
// last session is deleted (cascading away task_session_worktrees, the only
// table that stores a worktree's on-disk path), teardownEnvironmentResources
// must still remove every repo's worktree directory using the durable
// TaskEnvironmentRepo path/repository handles, not the session-scoped ID lookup.
//
// Persists real `repositories` rows so RemoveAt's
// path-only fallback can resolve each worktree's source repository. Without
// that, resolveRepositoryPath can't scope `git worktree remove`/`prune` to
// the correct repo, so os.Stat alone is not a sufficient assertion: the
// directory can be gone while the SOURCE repo is left with a stale
// `prunable` registration that blocks recreating a worktree at that path
// (Review Finding 3) — so this test also asserts each source repo's own
// `git worktree list` is clean afterward.
func TestTeardownEnvironmentResources_MultiRepoRemovesEveryWorktreeFromDisk(t *testing.T) {
	svc, mgr, repo := newRealWorktreeTestService(t)
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-multi", WorkspaceID: "workspace", Title: "Multi repo", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-multi", TaskID: "task-multi", State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	repoAPath := initSimpleGitRepo(t)
	repoBPath := initSimpleGitRepo(t)
	for id, path := range map[string]string{"repo-a": repoAPath, "repo-b": repoBPath} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "workspace", Name: id, SourceType: "local", LocalPath: path,
		}); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
	}

	primary, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: "task-multi", SessionID: "session-multi", TaskTitle: "Multi repo",
		RepositoryID: "repo-a", RepositoryPath: repoAPath,
		BaseBranch: "main", TaskDirName: "task-multi", RepoName: "repo-a",
	})
	if err != nil {
		t.Fatalf("create primary worktree: %v", err)
	}
	secondary, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: "task-multi", SessionID: "session-multi", TaskTitle: "Multi repo",
		RepositoryID: "repo-b", RepositoryPath: repoBPath,
		BaseBranch: "main", TaskDirName: "task-multi", RepoName: "repo-b",
	})
	if err != nil {
		t.Fatalf("create secondary worktree: %v", err)
	}

	env := &models.TaskEnvironment{
		ID:     "env-multi",
		TaskID: "task-multi",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", WorktreeID: primary.ID, WorktreePath: primary.Path},
			{RepositoryID: "repo-b", WorktreeID: secondary.ID, WorktreePath: secondary.Path},
		},
	}

	// Delete the task's only session — what session.delete does. This
	// cascades away every task_session_worktrees row for it, the only
	// durable path record for a worktree_id-keyed lookup.
	if _, err := repo.DB().ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-multi"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if err := svc.teardownEnvironmentResources(ctx, env, []string{"session-multi"}); err != nil {
		t.Fatalf("teardownEnvironmentResources: %v", err)
	}

	// Both assertions prove the fix: primary and secondary are each resolved only
	// through their TaskEnvironmentRepo path/repository handles.
	if _, statErr := os.Stat(primary.Path); !os.IsNotExist(statErr) {
		t.Errorf("primary worktree directory still on disk: %s (stat err = %v)", primary.Path, statErr)
	}
	if _, statErr := os.Stat(secondary.Path); !os.IsNotExist(statErr) {
		t.Errorf("secondary worktree directory still on disk: %s (stat err = %v)", secondary.Path, statErr)
	}
	assertNoWorktreeRegistration(t, repoAPath, primary.Path)
	assertNoWorktreeRegistration(t, repoBPath, secondary.Path)
}

// assertNoWorktreeRegistration fails the test if repoPath's own `git
// worktree list` still references worktreePath — i.e. the registration
// wasn't (or couldn't be) pruned from the correct source repository.
func assertNoWorktreeRegistration(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	out := string(runGitTestCmd(t, repoPath, "worktree", "list", "--porcelain"))
	if strings.Contains(out, worktreePath) {
		t.Errorf("stale worktree registration for %s remains in source repo %s:\n%s", worktreePath, repoPath, out)
	}
}
