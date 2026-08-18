package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// retryFallbackWarningMarker is the phrase unique to the warning
// resolveRetryStartPoint produces. The generic "fetch failed entirely" fallback
// in fetchBranchToLocal also ends on the local branch with a warning set, so
// asserting merely "local branch" or "some warning" cannot tell the two apart —
// a test that did would still pass with the retry skipped entirely.
const retryFallbackWarningMarker = "contains every commit"

// genericFetchFailureWarningMarker is the phrase from that other fallback,
// asserted absent so the tests below fail if they stop traversing the retry.
const genericFetchFailureWarningMarker = "Could not fetch latest from origin"

func writeRepoFile(t *testing.T, repoPath, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(contents), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// initGitRepoWithCheckedOutBranchAheadOfOrigin builds the shape that triggers
// the fetch-refused retry with local work at stake: a repository sitting *on*
// `feature/pr-branch` (the normal state of a user's working branch) whose local
// ref carries a commit that was never pushed.
//
// Returns the repository path, the local head SHA, and the origin head SHA.
func initGitRepoWithCheckedOutBranchAheadOfOrigin(t *testing.T) (string, string, string) {
	t.Helper()

	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "checkout", "feature/pr-branch")
	originHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "origin/feature/pr-branch"))

	// The commit that only exists locally: the user's unpushed work.
	writeRepoFile(t, repoPath, "local-only.txt", "unpushed\n")
	runGit(t, repoPath, "add", "local-only.txt")
	runGit(t, repoPath, "commit", "-m", "local-only commit")
	localHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))

	if localHead == originHead {
		t.Fatalf("fixture did not diverge: local and origin both at %s", localHead)
	}
	return repoPath, localHead, originHead
}

// requireFetchRefusedByCheckout asserts the fixture actually reaches the retry
// under test: git must refuse the `<branch>:<branch>` refspec because the
// branch is checked out. Without this the test could pass through an unrelated
// path and prove nothing about retryFetchAsRemoteTrackingRef. It doubles as a
// pin on isFetchRefusedCheckedOut's matched substring.
func requireFetchRefusedByCheckout(t *testing.T, repoPath, branch string) {
	t.Helper()

	cmd := exec.Command("git", "fetch", "--no-tags", "origin", branch+":"+branch)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("git fetch %s:%s succeeded, so the checked-out refusal path is not exercised", branch, branch)
	}
	if !isFetchRefusedCheckedOut(string(out)) {
		t.Fatalf("git fetch failed for the wrong reason; isFetchRefusedCheckedOut did not match:\n%s", out)
	}
}

// TestCreateWorktree_CheckedOutBranchRetryKeepsUnpushedCommits pins the
// behaviour that the fetch retry must not silently rebase a new worktree onto
// origin/<branch>.
//
// The first fetch (`<branch>:<branch>`) is refused whenever the branch is
// checked out, which for a user's own repository is the normal state of the
// branch they are working on. The retry fetches only the remote-tracking ref,
// so it deliberately leaves the local branch alone. Adopting origin/<branch> as
// the worktree start point therefore drops any commit the user has not pushed,
// and it does so silently: Create still succeeds, so nothing surfaces the loss.
//
// The direct-checkout path immediately above this one in addWorktreeForBranch
// checks out the local branch, as do handleFetchFallback and the
// non-current-branch path in resolveLocalBaseRef. This path must agree.
func TestCreateWorktree_CheckedOutBranchRetryKeepsUnpushedCommits(t *testing.T) {
	repoPath, localHead, originHead := initGitRepoWithCheckedOutBranchAheadOfOrigin(t)
	requireFetchRefusedByCheckout(t, repoPath, "feature/pr-branch")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-1", SessionID: "session-1", TaskTitle: "Work on my branch",
		RepositoryID: "repo-1", RepositoryPath: repoPath,
		BaseBranch: "main", CheckoutBranch: "feature/pr-branch",
		TaskDirName: "task-1", RepoName: "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// The branch is checked out in the repository itself, so the worktree gets
	// a suffixed branch created from the resolved start point. Assert it, so a
	// future change that returns to the direct-checkout path cannot make the
	// SHA assertion below pass for the wrong reason.
	if !strings.HasPrefix(wt.Branch, "feature/pr-branch-") {
		t.Fatalf("worktree branch = %q, want a suffixed feature/pr-branch-*", wt.Branch)
	}

	// Assert the commit the worktree actually starts from, not a ref name: the
	// defect is that the worktree starts from the wrong commit, and a name-only
	// assertion would still pass if the ref mapping changed.
	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != localHead {
		t.Fatalf(
			"worktree HEAD = %s, want the local branch head %s (origin is at %s);\n"+
				"the fetch retry must not drop the user's unpushed commits from the new worktree",
			worktreeSHA, localHead, originHead,
		)
	}

	// Pin the *path*, not just the outcome. The generic fetch-failure fallback
	// also lands on the local branch, so without this the test would still pass
	// with the retry skipped altogether. Here the local branch is a strict
	// superset of the remote ref, so the retry path warns about nothing at all
	// while the generic fallback always sets its own warning.
	if wt.FetchWarning != "" {
		t.Fatalf("expected no warning when the local branch is strictly ahead, got %q;\n"+
			"a %q warning means the retry never ran and this test is not exercising it",
			wt.FetchWarning, genericFetchFailureWarningMarker)
	}
}

// TestCreateWorktree_CheckedOutBranchRetryAdoptsRemoteCommits guards the reason
// the remote ref is returned at all, so the fix cannot degrade into a blanket
// "always use the local branch".
//
// The retry *does* refresh origin/<branch>, so when the local branch is simply
// stale the remote ref is the better start point and returning the local branch
// would lose the pushed commits instead. Together with the test above, this
// pair distinguishes the two directions of divergence: the two cases resolve to
// different commits, so neither assertion can pass vacuously.
func TestCreateWorktree_CheckedOutBranchRetryAdoptsRemoteCommits(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "checkout", "feature/pr-branch")
	localHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))

	// A second clone pushes a commit the working repository has not seen yet,
	// leaving its local branch a strict ancestor of origin/feature/pr-branch.
	otherPath := filepath.Join(t.TempDir(), "other")
	originURL := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin"))
	runGit(t, filepath.Dir(otherPath), "clone", originURL, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	runGit(t, otherPath, "checkout", "feature/pr-branch")
	writeRepoFile(t, otherPath, "remote-only.txt", "pushed\n")
	runGit(t, otherPath, "add", "remote-only.txt")
	runGit(t, otherPath, "commit", "-m", "remote-only commit")
	runGit(t, otherPath, "push", "origin", "feature/pr-branch")
	remoteHead := strings.TrimSpace(runGit(t, otherPath, "rev-parse", "HEAD"))

	if remoteHead == localHead {
		t.Fatalf("fixture did not diverge: local and origin both at %s", localHead)
	}
	requireFetchRefusedByCheckout(t, repoPath, "feature/pr-branch")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-1", SessionID: "session-1", TaskTitle: "Work on my branch",
		RepositoryID: "repo-1", RepositoryPath: repoPath,
		BaseBranch: "main", CheckoutBranch: "feature/pr-branch",
		TaskDirName: "task-1", RepoName: "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if !strings.HasPrefix(wt.Branch, "feature/pr-branch-") {
		t.Fatalf("worktree branch = %q, want a suffixed feature/pr-branch-*", wt.Branch)
	}

	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != remoteHead {
		t.Fatalf(
			"worktree HEAD = %s, want the fetched remote head %s (local is at %s);\n"+
				"a local branch that origin already contains must not pin the worktree to stale commits",
			worktreeSHA, remoteHead, localHead,
		)
	}
	if wt.FetchWarning != "" {
		t.Fatalf("unexpected fetch warning when origin is simply ahead: %q", wt.FetchWarning)
	}
}

// TestCreateWorktree_CheckedOutBranchRetryWarnsOnDivergence covers the case
// where neither ref contains the other: keeping the local branch is still the
// right call (unpushed commits are unrecoverable, staleness is not), but the
// worktree genuinely does miss commits that are on origin, so the existing
// FetchWarning surface must say so rather than reporting a clean success.
func TestCreateWorktree_CheckedOutBranchRetryWarnsOnDivergence(t *testing.T) {
	repoPath, localHead, _ := initGitRepoWithCheckedOutBranchAheadOfOrigin(t)

	// Push a different commit to origin so the two refs diverge.
	otherPath := filepath.Join(t.TempDir(), "other")
	originURL := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin"))
	runGit(t, filepath.Dir(otherPath), "clone", originURL, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	runGit(t, otherPath, "checkout", "feature/pr-branch")
	writeRepoFile(t, otherPath, "remote-only.txt", "pushed\n")
	runGit(t, otherPath, "add", "remote-only.txt")
	runGit(t, otherPath, "commit", "-m", "remote-only commit")
	runGit(t, otherPath, "push", "origin", "feature/pr-branch")
	remoteHead := strings.TrimSpace(runGit(t, otherPath, "rev-parse", "HEAD"))

	requireFetchRefusedByCheckout(t, repoPath, "feature/pr-branch")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-1", SessionID: "session-1", TaskTitle: "Work on my branch",
		RepositoryID: "repo-1", RepositoryPath: repoPath,
		BaseBranch: "main", CheckoutBranch: "feature/pr-branch",
		TaskDirName: "task-1", RepoName: "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != localHead {
		t.Fatalf("worktree HEAD = %s, want the local branch head %s (origin diverged at %s)",
			worktreeSHA, localHead, remoteHead)
	}
	if wt.FetchWarning == "" {
		t.Fatal("diverged local and remote refs must surface a fetch warning, not a silent success")
	}
	if !strings.Contains(wt.FetchWarning, "feature/pr-branch") {
		t.Fatalf("fetch warning does not name the branch: %q", wt.FetchWarning)
	}
	// Assert it is the retry path's warning, not the generic fetch-failure one:
	// both end on the local branch with a warning set, so only the text
	// distinguishes them.
	if !strings.Contains(wt.FetchWarning, retryFallbackWarningMarker) {
		t.Fatalf("warning %q is not the retry path's; the retry may not have run at all", wt.FetchWarning)
	}
}

// TestFetchBranchToLocal_RetryWithoutRemoteTrackingRefKeepsLocalBranch covers
// the third arm: the retry succeeds but leaves no origin/<branch> behind,
// because the remote carries no standard fetch refspec to update it
// opportunistically. Returning "origin/<branch>" there named a ref that does
// not resolve, so the downstream `git worktree add` failed outright; the same
// ancestor guard now keeps the local branch and reports why.
func TestFetchBranchToLocal_RetryWithoutRemoteTrackingRefKeepsLocalBranch(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "checkout", "feature/pr-branch")
	runGit(t, repoPath, "config", "--unset", "remote.origin.fetch")
	runGit(t, repoPath, "update-ref", "-d", "refs/remotes/origin/feature/pr-branch")
	requireFetchRefusedByCheckout(t, repoPath, "feature/pr-branch")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result, err := mgr.fetchBranchToLocal(context.Background(), repoPath, "feature/pr-branch", 0)
	if err != nil {
		t.Fatalf("fetchBranchToLocal() unexpected error: %v", err)
	}
	if result.StartPoint != "" {
		t.Fatalf("StartPoint = %q, want empty so the caller uses the local branch;\n"+
			"origin/feature/pr-branch does not exist, so that ref cannot be checked out",
			result.StartPoint)
	}
	if result.Warning == "" {
		t.Fatal("an unresolvable remote-tracking ref must surface a warning, not a silent success")
	}
	if !strings.Contains(result.Warning, retryFallbackWarningMarker) {
		t.Fatalf("warning %q is not the retry path's; the retry may not have run at all", result.Warning)
	}
}
