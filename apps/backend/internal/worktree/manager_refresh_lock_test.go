package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// initGitRepoWithRemoteAhead builds a checkout on main whose origin carries one
// additional commit, so a normal fetch and fast-forward pull can be exercised.
func initGitRepoWithRemoteAhead(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	originPath := filepath.Join(base, "origin.git")
	repoPath := filepath.Join(base, "repo")
	otherPath := filepath.Join(base, "other")

	runGit(t, base, "init", "--bare", "-b", "main", originPath)
	runGit(t, base, "init", "-b", "main", repoPath)
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, repoPath, "README.md", "initial\n")
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "origin", "main")

	runGit(t, base, "clone", originPath, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, otherPath, "remote.txt", "remote\n")
	runGit(t, otherPath, "add", "remote.txt")
	runGit(t, otherPath, "commit", "-m", "remote commit")
	runGit(t, otherPath, "push", "origin", "main")

	return repoPath, strings.TrimSpace(runGit(t, otherPath, "rev-parse", "HEAD"))
}

// installGitPullLockFixture puts a Git wrapper on PATH. The first pull creates
// refs/heads/main.lock, runs the real pull (which updates the index but cannot
// move the branch), records that partial state, releases the lock, and returns
// the real failure. A retried pull therefore completes deterministically.
func installGitPullLockFixture(t *testing.T) (pullCountPath, statePrefix string) {
	t.Helper()

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("resolve real git: %v", err)
	}
	dir := t.TempDir()
	pullCountPath = filepath.Join(dir, "pull-count")
	statePrefix = filepath.Join(dir, "partial")
	t.Setenv("KD_REAL_GIT", realGit)
	t.Setenv("KD_PULL_COUNT", pullCountPath)
	t.Setenv("KD_PULL_STATE", statePrefix)

	script := `#!/bin/sh
case "${1:-}" in
  -c) [ "${2:-}" != "core.longpaths=true" ] || shift 2 ;;
esac
if [ "${1:-}" = "pull" ]; then
  count_file="${KD_PULL_COUNT:?}"
  count=0
  [ ! -f "$count_file" ] || count=$(cat "$count_file")
  count=$((count + 1))
  printf '%s' "$count" > "$count_file"
  if [ "$count" -ne 1 ]; then exec "$KD_REAL_GIT" "$@"; fi
  : > .git/refs/heads/main.lock
  "$KD_REAL_GIT" "$@"
  status=$?
  "$KD_REAL_GIT" rev-parse refs/heads/main > "${KD_PULL_STATE:?}.old-head" 2>/dev/null
  "$KD_REAL_GIT" diff --cached --name-only > "${KD_PULL_STATE:?}.staged" 2>/dev/null
  rm -f .git/refs/heads/main.lock
  exit "$status"
fi
exec "$KD_REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0755); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return pullCountPath, statePrefix
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func lockRemoteTrackingMain(t *testing.T, repoPath string) {
	t.Helper()
	lockPath := filepath.Join(repoPath, ".git", "refs", "remotes", "origin", "main.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("create remote-tracking lock directory: %v", err)
	}
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatalf("create remote-tracking lock: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lockPath) })
}

func newRefreshLockCreateRequest(taskID, repoPath string) CreateRequest {
	return CreateRequest{
		TaskID:             taskID,
		SessionID:          "session-" + taskID,
		RepositoryID:       "repo-" + taskID,
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		PullBeforeWorktree: true,
		TaskDirName:        taskID,
		RepoName:           "repo",
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.31
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.34
func TestCreateWorktree_RefreshLockReleased(t *testing.T) {
	repoPath, remoteHead := initGitRepoWithRemoteAhead(t)
	oldHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))
	pullCountPath, statePrefix := installGitPullLockFixture(t)

	mgr := newRecreateTestManager(t)
	wt, err := mgr.Create(context.Background(), newRefreshLockCreateRequest("task-refresh-lock-released", repoPath))
	if err != nil {
		t.Fatalf("Create() error = %v, want the retried pull to complete", err)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main")); got != remoteHead {
		t.Fatalf("local main = %s, want retried fast-forward %s", got, remoteHead)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != remoteHead {
		t.Fatalf("worktree HEAD = %s, want %s", got, remoteHead)
	}
	if readFixtureFile(t, pullCountPath) != "2" {
		t.Fatalf("pull count = %q, want exactly one failed and one successful pull",
			readFixtureFile(t, pullCountPath))
	}
	if got := strings.TrimSpace(readFixtureFile(t, statePrefix+".old-head")); got != oldHead {
		t.Fatalf("partial pull branch ref = %s, want the old head %s", got, oldHead)
	}
	if staged := readFixtureFile(t, statePrefix+".staged"); !strings.Contains(staged, "remote.txt") {
		t.Fatalf("partial pull staged files = %q, want remote.txt to prove the index advanced", staged)
	}
	if wt.BaseBranchFallbackWarning != "" {
		t.Fatalf("worktree warning = %q, want no warning after recovery", wt.BaseBranchFallbackWarning)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.31
func TestCreateWorktree_RefreshLockPreservesEdits(t *testing.T) {
	repoPath, remoteHead := initGitRepoWithRemoteAhead(t)
	writeRepoFile(t, repoPath, "local-staged.txt", "staged local work\n")
	runGit(t, repoPath, "add", "local-staged.txt")
	writeRepoFile(t, repoPath, "README.md", "initial\nlocal unstaged edit\n")
	pullCountPath, statePrefix := installGitPullLockFixture(t)

	mgr := newRecreateTestManager(t)
	wt, err := mgr.Create(context.Background(), newRefreshLockCreateRequest("task-refresh-lock-edits", repoPath))
	if err != nil {
		t.Fatalf("Create() error = %v, want the retried pull to complete", err)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main")); got != remoteHead {
		t.Fatalf("local main = %s, want retried fast-forward %s", got, remoteHead)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != remoteHead {
		t.Fatalf("worktree HEAD = %s, want %s", got, remoteHead)
	}
	if got := readFixtureFile(t, filepath.Join(repoPath, "local-staged.txt")); got != "staged local work\n" {
		t.Fatalf("staged local file = %q, want it preserved", got)
	}
	if got := readFixtureFile(t, filepath.Join(repoPath, "README.md")); !strings.Contains(got, "local unstaged edit") {
		t.Fatalf("unstaged local edit = %q, want it preserved", got)
	}
	status := runGit(t, repoPath, "status", "--porcelain")
	if !strings.Contains(status, "A  local-staged.txt") || !strings.Contains(status, " M README.md") {
		t.Fatalf("local status = %q, want independent staged and unstaged changes", status)
	}
	if staged := readFixtureFile(t, statePrefix+".staged"); !strings.Contains(staged, "local-staged.txt") ||
		!strings.Contains(staged, "remote.txt") {
		t.Fatalf("partial pull staged files = %q, want local staged work and remote.txt", staged)
	}
	if readFixtureFile(t, pullCountPath) != "2" {
		t.Fatalf("pull count = %q, want one failed and one successful pull",
			readFixtureFile(t, pullCountPath))
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.32
func TestCreateWorktree_RefreshLockPolicy(t *testing.T) {
	t.Run("required PR refresh stays strict", func(t *testing.T) {
		repoPath, _ := initGitRepoWithRemoteAhead(t)
		lockRemoteTrackingMain(t, repoPath)
		mgr := newRecreateTestManager(t)
		req := newRefreshLockCreateRequest("task-refresh-lock-pr", repoPath)
		req.PRNumber = 7
		req.BaseBranchFallbackPolicy = models.BaseBranchFallbackPolicyStrict

		wt, err := mgr.Create(context.Background(), req)
		if wt != nil {
			t.Fatalf("Create() returned a worktree after persistent lock contention: %#v", wt)
		}
		if !errors.Is(err, ErrGitCommandFailed) {
			t.Fatalf("Create() error = %v, want ErrGitCommandFailed", err)
		}
		if !strings.Contains(err.Error(), "lock contention") || !strings.Contains(err.Error(), "4 attempts") {
			t.Fatalf("Create() error = %v, want lock contention and attempt count", err)
		}
	})

	t.Run("remote-only refresh stays strict", func(t *testing.T) {
		repoPath, _ := initGitRepoWithRemoteAhead(t)
		lockRemoteTrackingMain(t, repoPath)
		mgr := newRecreateTestManager(t)
		req := newRefreshLockCreateRequest("task-refresh-lock-remote", repoPath)
		req.BaseBranch = "origin/main"

		wt, err := mgr.Create(context.Background(), req)
		if wt != nil {
			t.Fatalf("Create() returned a worktree after remote-only lock contention: %#v", wt)
		}
		if !errors.Is(err, ErrGitCommandFailed) {
			t.Fatalf("Create() error = %v, want ErrGitCommandFailed", err)
		}
		if !strings.Contains(err.Error(), "lock contention") {
			t.Fatalf("Create() error = %v, want lock contention context", err)
		}
	})

	t.Run("best-effort local refresh keeps warning", func(t *testing.T) {
		repoPath, _ := initGitRepoWithRemoteAhead(t)
		localHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))
		lockRemoteTrackingMain(t, repoPath)
		mgr := newRecreateTestManager(t)
		req := newRefreshLockCreateRequest("task-refresh-lock-local", repoPath)
		var events []SyncProgressEvent
		req.OnSyncProgress = captureSyncProgress(&events)

		wt, err := mgr.Create(context.Background(), req)
		if err != nil {
			t.Fatalf("Create() error = %v, want the verified local base", err)
		}
		if len(events) != 2 || events[1].Status != SyncProgressCompleted || events[1].Warning == "" {
			t.Fatalf("sync progress = %#v, want a bounded completed warning", events)
		}
		if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != localHead {
			t.Fatalf("worktree HEAD = %s, want local base %s", got, localHead)
		}
	})

	t.Run("cancellation creates no fallback", func(t *testing.T) {
		repoPath, _ := initGitRepoWithRemoteAhead(t)
		mgr := newRecreateTestManager(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		wt, err := mgr.Create(ctx, newRefreshLockCreateRequest("task-refresh-lock-cancel", repoPath))
		if wt != nil {
			t.Fatalf("Create() returned a worktree after cancellation: %#v", wt)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Create() error = %v, want context.Canceled", err)
		}
	})
}
