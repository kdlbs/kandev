package backendapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

func TestBootMessageAdapterResumedMessageDoesNotLeaveActiveTurn(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID:   "workspace-1",
		Name: "Workspace",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID:          "task-1",
		WorkspaceID: "workspace-1",
		Title:       "Task",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	message, err := (&bootMsgAdapter{svc: harness.taskSvc}).CreateMessage(
		ctx,
		&lifecycle.BootMessageRequest{
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			AuthorType:    "agent",
			Type:          "script_execution",
			Metadata: map[string]interface{}{
				"script_type": "agent_boot",
				"is_resuming": true,
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message == nil {
		t.Fatal("CreateMessage returned nil")
	}

	activeTurn, err := harness.taskSvc.GetActiveTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if activeTurn != nil {
		t.Fatalf("active turn = %q, want none", activeTurn.ID)
	}

	turn, err := harness.taskSvc.GetTurn(ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatalf("resume message turn %q is not completed", turn.ID)
	}
}

// TestDetectBranchRemote_ReturnsConfiguredUpstream covers the happy path: a
// branch with an explicit `branch.<name>.remote` config returns that remote
// (covers fork-workflow repos whose primary remote is named "upstream",
// "github", etc., where hard-coding "origin" would push to the wrong place).
func TestDetectBranchRemote_ReturnsConfiguredUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// Mimic what `git push --set-upstream <remote> <branch>` would write.
	mustGit(t, dir, "config", "branch.feature.remote", "upstream")

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != "upstream" {
		t.Errorf("got %q, want upstream", got)
	}
}

func TestDetectBranchRemote_NoUpstreamFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// No branch.feature.remote config — git config returns non-zero.

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

func TestDetectBranchRemote_NonGitDirFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// `git config` in a non-git dir errors out, so detectBranchRemote
	// should fall back to the default remote rather than propagate the error.
	if got := detectBranchRemote(context.Background(), t.TempDir(), "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

// TestEnvironmentDestroyerAdapter_DestroyWorktreeAtForwardsArgumentsInOrder
// is the regression test for Review round 2's test-rigor residual:
// environmentDestroyerAdapter.DestroyWorktreeAt is a one-line delegation to
// worktree.Manager.RemoveAt with no test of its own — a parameter-order
// swap among the three string args would compile and pass the entire
// suite. Calling it with the real (worktreeID, worktreePath, repositoryID)
// order must actually remove the worktree directory; a swap (e.g.
// worktreeID/worktreePath transposed) would pass a UUID where a filesystem
// path is expected, silently no-op via the "already removed" branch, and
// leave the directory on disk — which this test would catch via the
// directory-removed assertion below.
func TestEnvironmentDestroyerAdapter_DestroyWorktreeAtForwardsArgumentsInOrder(t *testing.T) {
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, cleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	store, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	mgr, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: t.TempDir(), BranchPrefix: "kandev/",
	}, store, log)
	if err != nil {
		t.Fatalf("worktree.NewManager: %v", err)
	}

	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &models.Task{
		ID: "task-adapter", WorkspaceID: "workspace", Title: "Adapter", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-adapter", TaskID: "task-adapter", State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	repoPath := t.TempDir()
	mustGit(t, repoPath, "init", "--quiet", "-b", "main")
	mustGit(t, repoPath, "commit", "--allow-empty", "-m", "init")

	wt, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: "task-adapter", SessionID: "session-adapter", TaskTitle: "Adapter",
		RepositoryID: "repo-adapter", RepositoryPath: repoPath,
		BaseBranch: "main", TaskDirName: "task-adapter", RepoName: "repo-adapter",
	})
	if err != nil {
		t.Fatalf("mgr.Create: %v", err)
	}

	adapter := &environmentDestroyerAdapter{worktrees: mgr}
	if err := adapter.DestroyWorktreeAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
		t.Fatalf("DestroyWorktreeAt: %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Fatalf("worktree directory still on disk: %s (stat err = %v)", wt.Path, statErr)
	}
}

// mustGit runs `git -C dir <args...>` and fails the test on non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	// Start from the parent environment so git keeps HOME/PATH/etc., then:
	//   - disable system config (/etc/gitconfig) to prevent host policy from
	//     interfering with the test
	//   - set an identity inline so `git commit` works on CI runners that
	//     have no user.email/user.name configured. Both AUTHOR and COMMITTER
	//     are needed — git fails fast if either is missing.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
