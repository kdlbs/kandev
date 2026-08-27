package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepoWithOriginAheadLocal builds the shape that matters here: a normal
// repository sitting on `main`, wired to a reachable bare `origin`, whose local
// `main` carries a commit that was never pushed. origin/main is therefore a
// strict ancestor of local main.
//
// Returns the working repository path and the local-only commit SHA.
func initGitRepoWithOriginAheadLocal(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	originPath := filepath.Join(base, "origin.git")
	repoPath := filepath.Join(base, "repo")

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

	// The commit that only exists locally — the equivalent of a user's unpushed
	// work on their base branch.
	writeRepoFile(t, repoPath, "local-only.txt", "unpushed\n")
	runGit(t, repoPath, "add", "local-only.txt")
	runGit(t, repoPath, "commit", "-m", "local-only commit")

	localHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))
	return repoPath, localHead
}

// captureSyncProgress records every SyncProgressEvent so a test can assert
// which branch of the sync actually ran, not merely what it returned.
func captureSyncProgress(events *[]SyncProgressEvent) SyncProgressCallback {
	return func(event SyncProgressEvent) { *events = append(*events, event) }
}

// syncOutputs joins the captured progress output for assertion messages.
func syncOutputs(events []SyncProgressEvent) string {
	parts := make([]string, 0, len(events))
	for _, e := range events {
		parts = append(parts, e.Output)
	}
	return strings.Join(parts, " | ")
}

// TestPullBaseBranch_PullFailureKeepsLocalCommits pins the behaviour that a
// transient `git pull` failure must not silently rebase the worktree's starting
// point onto origin/<branch>.
//
// When the pull fails for an environmental reason (throttle contention, a
// timeout, a stale index.lock) origin/<branch> is not fresher than the local
// branch — here it is a strict ancestor of it. Returning the remote ref
// therefore discards the user's unpushed commits from the new worktree, and it
// does so silently: creation still succeeds, so nothing surfaces the loss.
//
// The two sibling fallbacks in manager_git.go (handleFetchFallback, and the
// non-current-branch path in resolveLocalBaseRef) already resolve to the local
// branch on failure. This one must agree with them.
func TestPullBaseBranch_PullFailureKeepsLocalCommits(t *testing.T) {
	repoPath, localHead := initGitRepoWithOriginAheadLocal(t)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	// Let the fetch succeed (so we reach the pull) but force the pull to fail
	// the way contention makes it fail in a loaded run. A zero timeout yields
	// an already-expired context, so the pull cannot accidentally succeed.
	mgr.pullTimeout = 0

	var events []SyncProgressEvent
	resolved, err := mgr.pullBaseBranch(context.Background(), repoPath, "main", captureSyncProgress(&events))
	if err != nil {
		t.Fatalf("pullBaseBranch() error = %v", err)
	}
	if resolved == "" {
		t.Fatal("pullBaseBranch returned an empty ref")
	}

	// Prove the failure path actually ran. Both the success and failure paths
	// resolve to `main` here, so the SHA assertion below cannot distinguish
	// them on its own — without this the test would pass vacuously if the pull
	// ever succeeded. The failure path reports "Pull <reason>; using <ref>",
	// the success path "Synced and using <ref>".
	outputs := syncOutputs(events)
	if !strings.Contains(outputs, "Pull ") {
		t.Fatalf(
			"pull was expected to fail and take the fallback path, but sync progress was %q;\n"+
				"the assertion below would pass without exercising the fallback",
			outputs,
		)
	}

	// Assert on the commit the ref actually points at, not on the ref's name:
	// the defect is that the worktree starts from the wrong commit, and a
	// name-only assertion would still pass if the mapping changed.
	resolvedSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", resolved))
	if resolvedSHA != localHead {
		t.Fatalf(
			"pullBaseBranch resolved %q to %s, which drops the unpushed local commit %s;\n"+
				"a failed pull must not move the worktree base backwards onto origin/main",
			resolved, resolvedSHA, localHead,
		)
	}
}

func TestResolveBaseRefWithFallback_RequiredRefreshFailureStopsPreparation(t *testing.T) {
	repoPath, _ := initGitRepoWithOriginAheadLocal(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  fetch)
    echo "fatal: Authentication failed" >&2
    exit 1
    ;;
  *)
    exit 0
    ;;
esac
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	req := CreateRequest{
		RepositoryPath: repoPath, BaseBranch: "main", PullBeforeWorktree: true,
	}
	var events []SyncProgressEvent
	req.OnSyncProgress = captureSyncProgress(&events)
	_, _, _, err = mgr.resolveBaseRefWithFallback(context.Background(), &req)
	if err == nil {
		t.Fatal("resolveBaseRefWithFallback() succeeded after required refresh failure")
	}
	if len(events) != 2 || events[0].Status != SyncProgressRunning || events[1].Status != SyncProgressFailed {
		t.Fatalf("required refresh progress = %#v, want running then failed", events)
	}
	if events[1].Error == "" || strings.Contains(events[1].Error, "Authentication") {
		t.Fatalf("required refresh progress exposed unsafe error detail: %#v", events[1])
	}
}

func TestPreferRefreshedRemoteRefSelectsSafeContainingRef(t *testing.T) {
	t.Run("local contains remote", func(t *testing.T) {
		repoPath, _ := initGitRepoWithOriginAheadLocal(t)
		mgr := newRecreateTestManager(t)

		resolved, err := mgr.preferRefreshedRemoteRef(context.Background(), repoPath, "main")
		if err != nil {
			t.Fatalf("preferRefreshedRemoteRef() error: %v", err)
		}
		if resolved != "main" {
			t.Fatalf("resolved ref = %q, want local main", resolved)
		}
	})

	t.Run("remote contains local", func(t *testing.T) {
		repoPath := initGitRepoWithRemote(t)
		otherPath := filepath.Join(t.TempDir(), "other")
		originURL := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin"))
		runGit(t, filepath.Dir(otherPath), "clone", originURL, otherPath)
		runGit(t, otherPath, "config", "user.email", "other@example.com")
		runGit(t, otherPath, "config", "user.name", "Other User")
		runGit(t, otherPath, "config", "commit.gpgsign", "false")
		writeRepoFile(t, otherPath, "remote-only.txt", "pushed\n")
		runGit(t, otherPath, "add", "remote-only.txt")
		runGit(t, otherPath, "commit", "-m", "remote-only commit")
		runGit(t, otherPath, "push", "origin", "main")
		runGit(t, repoPath, "fetch", "origin", "main")

		mgr := newRecreateTestManager(t)
		resolved, err := mgr.preferRefreshedRemoteRef(context.Background(), repoPath, "main")
		if err != nil {
			t.Fatalf("preferRefreshedRemoteRef() error: %v", err)
		}
		if resolved != "origin/main" {
			t.Fatalf("resolved ref = %q, want origin/main", resolved)
		}
	})
}

func TestPreferRefreshedRemoteRefRejectsDivergedRefs(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	writeRepoFile(t, repoPath, "local-only.txt", "local\n")
	runGit(t, repoPath, "add", "local-only.txt")
	runGit(t, repoPath, "commit", "-m", "local-only commit")

	otherPath := filepath.Join(t.TempDir(), "other")
	originURL := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin"))
	runGit(t, filepath.Dir(otherPath), "clone", originURL, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, otherPath, "remote-only.txt", "remote\n")
	runGit(t, otherPath, "add", "remote-only.txt")
	runGit(t, otherPath, "commit", "-m", "remote-only commit")
	runGit(t, otherPath, "push", "origin", "main")
	runGit(t, repoPath, "fetch", "origin", "main")

	mgr := newRecreateTestManager(t)
	if _, err := mgr.preferRefreshedRemoteRef(context.Background(), repoPath, "main"); err == nil {
		t.Fatal("preferRefreshedRemoteRef() accepted diverged local and remote refs")
	}
}

func TestPreferRefreshedRemoteRefRejectsUncertainAncestry(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  rev-parse)
    exit 0
    ;;
  merge-base)
    echo "fatal: unable to inspect repository" >&2
    exit 128
    ;;
  *)
    exit 0
    ;;
esac
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := newRecreateTestManager(t)
	if _, err := mgr.preferRefreshedRemoteRef(context.Background(), t.TempDir(), "main"); err == nil {
		t.Fatal("preferRefreshedRemoteRef() accepted an ancestry probe failure")
	}
}

// TestPullBaseBranch_SuccessfulPullStillAdoptsRemoteCommits guards the happy
// path the fix must not regress: when the pull succeeds, the base branch is
// fast-forwarded and the worktree starts from the updated local branch, which
// now contains the remote commit.
func TestPullBaseBranch_SuccessfulPullStillAdoptsRemoteCommits(t *testing.T) {
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

	// A second clone pushes a commit the working repo has not seen yet.
	runGit(t, base, "clone", originPath, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, otherPath, "remote-only.txt", "pushed\n")
	runGit(t, otherPath, "add", "remote-only.txt")
	runGit(t, otherPath, "commit", "-m", "remote-only commit")
	runGit(t, otherPath, "push", "origin", "main")
	remoteHead := strings.TrimSpace(runGit(t, otherPath, "rev-parse", "HEAD"))

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	resolved, err := mgr.pullBaseBranch(context.Background(), repoPath, "main", nil)
	if err != nil {
		t.Fatalf("pullBaseBranch() error = %v", err)
	}
	resolvedSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", resolved))
	if resolvedSHA != remoteHead {
		t.Fatalf(
			"pullBaseBranch resolved %q to %s, want the pulled remote commit %s",
			resolved, resolvedSHA, remoteHead,
		)
	}
}
