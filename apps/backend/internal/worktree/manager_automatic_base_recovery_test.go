package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultMissingRemoteRecovers(t *testing.T) {
	for _, tc := range []struct {
		name               string
		pullBeforeWorktree bool
	}{
		{name: "refresh disabled"},
		{name: "refresh enabled", pullBeforeWorktree: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := cloneAutomaticBaseRecoveryRepository(t)
			wantHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/main"))
			store := newMockStore()
			mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}
			var events []SyncProgressEvent

			wt, err := mgr.Create(context.Background(), CreateRequest{
				TaskID:                   "task-automatic-base-recovery",
				SessionID:                "session-automatic-base-recovery",
				TaskTitle:                "Recover automatic base",
				RepositoryID:             "repo-automatic-base-recovery",
				RepositoryPath:           repoPath,
				BaseBranch:               "retired-base",
				FallbackBaseBranch:       "retired-base",
				BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
				PullBeforeWorktree:       tc.pullBeforeWorktree,
				TaskDirName:              "task-automatic-base-recovery",
				RepoName:                 "repo-automatic-base-recovery",
				OnSyncProgress:           captureSyncProgress(&events),
			})
			if err != nil {
				t.Fatalf("Create() failed to recover automatic base: %v", err)
			}
			if wt.BaseBranch != "main" {
				t.Fatalf("worktree base = %q, want main", wt.BaseBranch)
			}
			if !strings.Contains(wt.BaseBranchFallbackWarning, "retired-base") ||
				!strings.Contains(wt.BaseBranchFallbackWarning, "main") {
				t.Fatalf("fallback warning = %q, want both branch names", wt.BaseBranchFallbackWarning)
			}
			if gotHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); gotHead != wantHead {
				t.Fatalf("worktree HEAD = %s, want refreshed default %s", gotHead, wantHead)
			}
			if tc.pullBeforeWorktree && (len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted) {
				t.Fatalf("final sync progress = %#v, want completed", events)
			}
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultDeletedRemoteTrackingRefUsesRefreshedFallback(t *testing.T) {
	remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
	runGit(t, sourcePath, "checkout", "-b", "retired-base")
	writeRepoFile(t, sourcePath, "retired-sentinel.txt", "retired branch sentinel\n")
	runGit(t, sourcePath, "add", "retired-sentinel.txt")
	runGit(t, sourcePath, "commit", "-m", "publish retired base")
	retiredHead := strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
	runGit(t, sourcePath, "push", "origin", "retired-base")

	cachePath := filepath.Join(t.TempDir(), "cache")
	runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
	if got := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/retired-base")); got != retiredHead {
		t.Fatalf("cached retired ref = %s, want sentinel commit %s", got, retiredHead)
	}
	defaultHead := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main"))
	runGit(t, sourcePath, "push", "origin", "--delete", "retired-base")
	if got := strings.TrimSpace(runGit(t, sourcePath, "ls-remote", "--heads", "origin", "refs/heads/retired-base")); got != "" {
		t.Fatalf("origin still advertises retired-base: %s", got)
	}

	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	var events []SyncProgressEvent
	request := automaticBaseRecoveryRequest(cachePath, "task-deleted-remote-tracking-ref")
	request.BaseBranch = "origin/retired-base"
	request.FallbackBaseBranch = "main"
	request.OnSyncProgress = captureSyncProgress(&events)

	wt, err := mgr.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("Create() failed to recover from deleted remote-tracking base: %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree base = %q, want refreshed default main", wt.BaseBranch)
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "origin/retired-base") ||
		!strings.Contains(wt.BaseBranchFallbackWarning, "main") {
		t.Fatalf("fallback warning = %q, want requested and substituted branches", wt.BaseBranchFallbackWarning)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != defaultHead || got == retiredHead {
		t.Fatalf("worktree HEAD = %s, want refreshed default %s instead of retired commit %s", got, defaultHead, retiredHead)
	}
	if len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted {
		t.Fatalf("final sync progress = %#v, want completed", events)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultDiscoversCurrentRemoteDefault(t *testing.T) {
	for _, tc := range []struct {
		name                string
		uncachedDefault     bool
		remoteDefaultBranch string
	}{
		{name: "stale origin head points to deleted base", remoteDefaultBranch: "main"},
		{name: "origin head absent and default uncached", uncachedDefault: true, remoteDefaultBranch: "new-default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
			runGit(t, sourcePath, "checkout", "-b", "retired-base")
			writeRepoFile(t, sourcePath, "retired-sentinel.txt", "retired branch sentinel\n")
			runGit(t, sourcePath, "add", "retired-sentinel.txt")
			runGit(t, sourcePath, "commit", "-m", "publish retired base")
			runGit(t, sourcePath, "push", "origin", "retired-base")

			cachePath := filepath.Join(t.TempDir(), "cache")
			runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
			var wantHead string
			if tc.uncachedDefault {
				runGit(t, sourcePath, "checkout", "main")
				runGit(t, sourcePath, "checkout", "-b", tc.remoteDefaultBranch)
				writeRepoFile(t, sourcePath, "new-default.txt", "new remote default\n")
				runGit(t, sourcePath, "add", "new-default.txt")
				runGit(t, sourcePath, "commit", "-m", "publish new remote default")
				wantHead = strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
				runGit(t, sourcePath, "push", "origin", tc.remoteDefaultBranch)
				runGit(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/"+tc.remoteDefaultBranch)
				runGit(t, cachePath, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
			} else {
				wantHead = strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main"))
				runGit(t, cachePath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/retired-base")
			}
			runGit(t, sourcePath, "push", "origin", "--delete", "retired-base")

			store := newMockStore()
			mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}
			var events []SyncProgressEvent
			request := automaticBaseRecoveryRequest(cachePath, "task-live-default-"+strings.ReplaceAll(tc.name, " ", "-"))
			request.FallbackBaseBranch = "retired-base"
			request.OnSyncProgress = captureSyncProgress(&events)

			wt, err := mgr.Create(context.Background(), request)
			if err != nil {
				t.Fatalf("Create() failed to discover and refresh remote default: %v", err)
			}
			if wt.BaseBranch != tc.remoteDefaultBranch {
				t.Fatalf("worktree base = %q, want current remote default %q", wt.BaseBranch, tc.remoteDefaultBranch)
			}
			if !strings.Contains(wt.BaseBranchFallbackWarning, "retired-base") ||
				!strings.Contains(wt.BaseBranchFallbackWarning, tc.remoteDefaultBranch) {
				t.Fatalf("fallback warning = %q, want both branch names", wt.BaseBranchFallbackWarning)
			}
			if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantHead {
				t.Fatalf("worktree HEAD = %s, want current remote default %s", got, wantHead)
			}
			if len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted {
				t.Fatalf("final sync progress = %#v, want completed", events)
			}
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultFallbackUsesFetchedCommitWithNarrowRefspec(t *testing.T) {
	remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
	runGit(t, sourcePath, "checkout", "-b", "retired-base")
	writeRepoFile(t, sourcePath, "retired-sentinel.txt", "retired branch sentinel\n")
	runGit(t, sourcePath, "add", "retired-sentinel.txt")
	runGit(t, sourcePath, "commit", "-m", "publish retired base")
	runGit(t, sourcePath, "push", "origin", "retired-base")

	cachePath := filepath.Join(t.TempDir(), "cache")
	runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
	oldDefaultHead := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main"))
	runGit(t, sourcePath, "checkout", "main")
	writeRepoFile(t, sourcePath, "updated-default.txt", "updated remote default\n")
	runGit(t, sourcePath, "add", "updated-default.txt")
	runGit(t, sourcePath, "commit", "-m", "advance remote default")
	wantHead := strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
	runGit(t, sourcePath, "push", "origin", "main")
	runGit(t, sourcePath, "push", "origin", "--delete", "retired-base")
	runGit(t, cachePath, "checkout", "-b", "topic", "refs/remotes/origin/main")
	runGit(t, cachePath, "branch", "-D", "main")
	runGit(t, cachePath, "config", "--unset-all", "remote.origin.fetch")
	runGit(t, cachePath, "config", "--add", "remote.origin.fetch", "+refs/heads/topic:refs/remotes/origin/topic")
	if got := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main")); got != oldDefaultHead {
		t.Fatalf("cached origin/main = %s, want stale commit %s", got, oldDefaultHead)
	}

	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	var events []SyncProgressEvent
	request := automaticBaseRecoveryRequest(cachePath, "task-narrow-fallback-refspec")
	request.FallbackBaseBranch = "main"
	request.OnSyncProgress = captureSyncProgress(&events)

	wt, err := mgr.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("Create() failed to refresh default with a narrow fetch refspec: %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree base = %q, want main", wt.BaseBranch)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("worktree HEAD = %s, want fetched default commit %s", got, wantHead)
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "retired-base") ||
		!strings.Contains(wt.BaseBranchFallbackWarning, "main") {
		t.Fatalf("fallback warning = %q, want both branch names", wt.BaseBranchFallbackWarning)
	}
	if len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted {
		t.Fatalf("final sync progress = %#v, want completed", events)
	}
}

func cloneAutomaticBaseRecoveryRepository(t *testing.T) string {
	t.Helper()
	remotePath, _ := newExplicitBaseRemoteFixture(t)
	cachePath := filepath.Join(t.TempDir(), "cache")
	runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
	return cachePath
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultFallbackRequiresVerifiedRefresh(t *testing.T) {
	t.Run("uses the configured candidate before the live default", func(t *testing.T) {
		remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
		cachePath := filepath.Join(t.TempDir(), "cache")
		runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)

		runGit(t, sourcePath, "checkout", "-b", "release")
		writeRepoFile(t, sourcePath, "release.txt", "configured fallback\n")
		runGit(t, sourcePath, "add", "release.txt")
		runGit(t, sourcePath, "commit", "-m", "publish configured fallback")
		wantHead := strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
		runGit(t, sourcePath, "push", "origin", "release")

		mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		request := automaticBaseRecoveryRequest(cachePath, "task-configured-default")
		request.FallbackBaseBranch = "release"
		wt, err := mgr.Create(context.Background(), request)
		if err != nil {
			t.Fatalf("Create() failed to refresh the configured fallback: %v", err)
		}
		if wt.BaseBranch != "release" {
			t.Fatalf("worktree base = %q, want configured fallback release", wt.BaseBranch)
		}
		if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantHead {
			t.Fatalf("worktree HEAD = %s, want configured fallback %s", got, wantHead)
		}
	})

	t.Run("uses the newly published default commit", func(t *testing.T) {
		remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
		cachePath := filepath.Join(t.TempDir(), "cache")
		runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
		oldHead := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main"))

		writeRepoFile(t, sourcePath, "remote-refresh.txt", "new default commit\n")
		runGit(t, sourcePath, "add", "remote-refresh.txt")
		runGit(t, sourcePath, "commit", "-m", "advance default branch")
		wantHead := strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
		runGit(t, sourcePath, "push", "origin", "main")
		if got := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "refs/remotes/origin/main")); got != oldHead {
			t.Fatalf("cached origin/main = %s, want pre-publish commit %s", got, oldHead)
		}

		store := newMockStore()
		mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		var events []SyncProgressEvent
		request := automaticBaseRecoveryRequest(cachePath, "task-fresh-default")
		request.OnSyncProgress = captureSyncProgress(&events)

		wt, err := mgr.Create(context.Background(), request)
		if err != nil {
			t.Fatalf("Create() failed to refresh the default: %v", err)
		}
		if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantHead {
			t.Fatalf("worktree HEAD = %s, want freshly published default %s", got, wantHead)
		}
		if wt.BaseBranch != "main" || len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted {
			t.Fatalf("worktree base = %q, final sync progress = %#v; want main and completed", wt.BaseBranch, events)
		}
	})

	t.Run("does not use a cached default after refresh fails", func(t *testing.T) {
		repoPath := cloneAutomaticBaseRecoveryRepository(t)
		installAutomaticBaseRecoveryGit(t, `
case "$*" in
  *"retired-base"*) echo "fatal: couldn't find remote ref retired-base" >&2; exit 128 ;;
  *"main"*) echo "fatal: Authentication failed" >&2; exit 128 ;;
esac
`)
		store := newMockStore()
		mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		createdCallbacks := 0
		request := automaticBaseRecoveryRequest(repoPath, "task-failed-default-refresh")
		request.OnWorktreeCreated = func(*Worktree) { createdCallbacks++ }

		wt, err := mgr.Create(context.Background(), request)
		if err == nil || !errors.Is(err, ErrAuthFailed) {
			t.Fatalf("Create() error = %v, want required fallback auth failure", err)
		}
		if wt != nil || createdCallbacks != 0 || len(store.worktrees) != 0 {
			t.Fatalf("failed fallback created effects: worktree=%#v callbacks=%d stored=%d", wt, createdCallbacks, len(store.worktrees))
		}
	})
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultRecoveryRejectsUnprovenFailure(t *testing.T) {
	tests := []struct {
		name                string
		fetchScript         string
		fetchTimeout        time.Duration
		cancelOnFetch       bool
		wantCause           error
		wantLockExhaustion  bool
		wantMissingRefCause bool
	}{
		{
			name:        "authentication failure",
			fetchScript: `echo "fatal: Authentication failed" >&2; exit 128`,
			wantCause:   ErrAuthFailed,
		},
		{
			name:        "network failure",
			fetchScript: `echo "fatal: unable to access https://example.invalid/repo.git: Could not resolve host" >&2; exit 128`,
			wantCause:   ErrGitCommandFailed,
		},
		{
			name:        "generic failure",
			fetchScript: `echo "fatal: remote request failed before ref lookup" >&2; exit 128`,
			wantCause:   ErrGitCommandFailed,
		},
		{
			name:                "timeout with missing-ref output",
			fetchScript:         `echo "fatal: couldn't find remote ref retired-base" >&2; sleep 2; exit 128`,
			fetchTimeout:        100 * time.Millisecond,
			wantCause:           context.DeadlineExceeded,
			wantMissingRefCause: false,
		},
		{
			name:                "cancellation with missing-ref output",
			fetchScript:         `echo "fatal: couldn't find remote ref retired-base" >&2; touch "${KD_FETCH_REPORTED:?}"; sleep 2; exit 128`,
			cancelOnFetch:       true,
			wantCause:           context.Canceled,
			wantMissingRefCause: false,
		},
		{
			name:               "exhausted lock contention",
			fetchScript:        `echo "fatal: Unable to create '.git/refs/heads/main.lock': File exists." >&2; exit 128`,
			wantCause:          ErrGitCommandFailed,
			wantLockExhaustion: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := cloneAutomaticBaseRecoveryRepository(t)
			startPath := filepath.Join(t.TempDir(), "fetch-reported")
			if tc.cancelOnFetch {
				t.Setenv("KD_FETCH_REPORTED", startPath)
			}
			installAutomaticBaseRecoveryGit(t, `
if [ "${1:-}" = "fetch" ]; then
`+tc.fetchScript+`
fi
`)
			store := newMockStore()
			mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}
			if tc.fetchTimeout > 0 {
				mgr.fetchTimeout = tc.fetchTimeout
			}
			createdCallbacks := 0
			request := automaticBaseRecoveryRequest(repoPath, "task-unproven-"+strings.ReplaceAll(tc.name, " ", "-"))
			request.OnWorktreeCreated = func(*Worktree) { createdCallbacks++ }

			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.cancelOnFetch {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}
			type createResult struct {
				worktree *Worktree
				err      error
			}
			result := make(chan createResult, 1)
			go func() {
				worktree, createErr := mgr.Create(ctx, request)
				result <- createResult{worktree: worktree, err: createErr}
			}()
			if tc.cancelOnFetch {
				waitForAutomaticRecoveryFile(t, startPath)
				cancel()
			}
			created := <-result
			if created.err == nil || created.worktree != nil {
				t.Fatalf("Create() = (%#v, %v), want failure without a worktree", created.worktree, created.err)
			}
			if !errors.Is(created.err, tc.wantCause) {
				t.Fatalf("Create() error = %v, want cause %v", created.err, tc.wantCause)
			}
			if tc.name == "network failure" &&
				(strings.Contains(created.err.Error(), "example.invalid") || strings.Contains(created.err.Error(), "Could not resolve host")) {
				t.Fatalf("Create() leaked raw fetch output: %v", created.err)
			}
			if errors.Is(created.err, ErrRemoteRefMissing) != tc.wantMissingRefCause {
				t.Fatalf("Create() error = %v, ErrRemoteRefMissing = %t, want %t", created.err, errors.Is(created.err, ErrRemoteRefMissing), tc.wantMissingRefCause)
			}
			if tc.wantLockExhaustion {
				var lockErr *refreshLockExhaustedError
				if !errors.As(created.err, &lockErr) {
					t.Fatalf("Create() error = %v, want exhausted lock identity", created.err)
				}
			}
			if createdCallbacks != 0 || len(store.worktrees) != 0 {
				t.Fatalf("failed recovery created effects: callbacks=%d stored=%d", createdCallbacks, len(store.worktrees))
			}
		})
	}

	t.Run("successful fetch without a verified ref is not absence proof", func(t *testing.T) {
		repoPath := cloneAutomaticBaseRecoveryRepository(t)
		installAutomaticBaseRecoveryGit(t, `
if [ "${1:-}" = "fetch" ]; then
  case "$*" in
    *"retired-base"*) exit 0 ;;
  esac
fi
`)
		mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		wt, err := mgr.Create(context.Background(), automaticBaseRecoveryRequest(repoPath, "task-uncertain-original-fetch"))
		if err == nil || wt != nil {
			t.Fatalf("Create() = (%#v, %v), want unverified-ref failure", wt, err)
		}
		if errors.Is(err, ErrRemoteRefMissing) {
			t.Fatalf("Create() error = %v, successful fetch verification failure must not prove absence", err)
		}
	})

	t.Run("uncertain replacement verification does not use cached refs", func(t *testing.T) {
		repoPath := cloneAutomaticBaseRecoveryRepository(t)
		runGit(t, repoPath, "checkout", "-b", "work")
		installAutomaticBaseRecoveryGit(t, `
if [ "${1:-}" = "fetch" ]; then
  case "$*" in
    *"retired-base"*) echo "fatal: couldn't find remote ref retired-base" >&2; exit 128 ;;
    *"main"*) "$KD_REAL_GIT" update-ref -d refs/remotes/origin/main; exit 0 ;;
  esac
fi
`)
		mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		wt, err := mgr.Create(context.Background(), automaticBaseRecoveryRequest(repoPath, "task-uncertain-replacement"))
		if err == nil || wt != nil {
			t.Fatalf("Create() = (%#v, %v), want replacement verification failure", wt, err)
		}
		if errors.Is(err, ErrRemoteRefMissing) {
			t.Fatalf("Create() error = %v, candidate verification failure must not prove requested-ref absence", err)
		}
	})
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.26
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
func TestCreateWorktree_AutomaticDefaultRecoveryKeepsStrictSelection(t *testing.T) {
	repoPath := cloneAutomaticBaseRecoveryRepository(t)
	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	createdCallbacks := 0
	request := automaticBaseRecoveryRequest(repoPath, "task-strict-base-recovery")
	request.BaseBranchFallbackPolicy = models.BaseBranchFallbackPolicyStrict
	request.OnWorktreeCreated = func(*Worktree) { createdCallbacks++ }

	wt, err := mgr.Create(context.Background(), request)
	if err == nil || wt != nil {
		t.Fatalf("Create() = (%#v, %v), want strict missing-base failure", wt, err)
	}
	if !errors.Is(err, ErrRemoteRefMissing) || !errors.Is(err, ErrGitCommandFailed) {
		t.Fatalf("Create() error = %v, want both confirmed missing-ref and Git failure identities", err)
	}
	if !strings.Contains(err.Error(), "retired-base") || !strings.Contains(err.Error(), "missing_remote_ref") {
		t.Fatalf("Create() error = %q, want requested ref and classification", err)
	}
	if createdCallbacks != 0 || len(store.worktrees) != 0 {
		t.Fatalf("strict failure created effects: callbacks=%d stored=%d", createdCallbacks, len(store.worktrees))
	}
}

func automaticBaseRecoveryRequest(repoPath, taskID string) CreateRequest {
	return CreateRequest{
		TaskID:                   taskID,
		SessionID:                "session-" + taskID,
		TaskTitle:                "Recover automatic base",
		RepositoryID:             "repo-" + taskID,
		RepositoryPath:           repoPath,
		BaseBranch:               "retired-base",
		FallbackBaseBranch:       "retired-base",
		BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
		PullBeforeWorktree:       true,
		TaskDirName:              taskID,
		RepoName:                 "repo-" + taskID,
	}
}

func installAutomaticBaseRecoveryGit(t *testing.T, fetchScript string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Git wrapper fixture is unavailable on Windows")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("resolve real git: %v", err)
	}
	t.Setenv("KD_REAL_GIT", realGit)
	scriptDir := writeFakeGitScript(t, `
if [ "${1:-}" = "fetch" ]; then
`+fetchScript+`
fi
exec "$KD_REAL_GIT" "$@"
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func waitForAutomaticRecoveryFile(t *testing.T, path string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("timed out waiting for Git wrapper marker %q", path)
		}
	}
}
