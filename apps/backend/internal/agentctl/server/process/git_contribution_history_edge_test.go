package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestContributionHistoryExpiredReflogFallsBackToNeutral(t *testing.T) {
	_, linkedDir, localHead, remoteHead, _, cleanup := seedContributionHistoryRepo(t)
	defer cleanup()

	runGit(t, linkedDir, "reflog", "expire", "--expire=now", "--all")
	result, err := NewGitOperator(linkedDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonNoMatch)
}

func TestContributionHistoryMissingObjectsFallsBackToNeutral(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	localHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	missingHead := strings.Repeat("0", 40)
	result, err := NewGitOperator(repoDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "main", localHead, missingHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonMissing)
}

func TestContributionHistoryPostRebaseCommitFallsBackToNeutral(t *testing.T) {
	_, linkedDir, _, remoteHead, _, cleanup := seedContributionHistoryRepo(t)
	defer cleanup()

	writeFile(t, linkedDir, "after-rebase.txt", "post-rebase change\n")
	runGit(t, linkedDir, "add", "after-rebase.txt")
	runGit(t, linkedDir, "commit", "-m", "post-rebase commit")
	localHead := strings.TrimSpace(runGit(t, linkedDir, "rev-parse", "HEAD"))
	result, err := NewGitOperator(linkedDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonNoMatch)
}

func TestContributionHistoryInProgressRebaseIsAmbiguous(t *testing.T) {
	_, linkedDir, currentHead, remoteHead, _, cleanup := seedContributionHistoryConflictRepo(t)
	defer cleanup()

	result, err := NewGitOperator(linkedDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "feature/history", currentHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonAmbiguous)
	runGit(t, linkedDir, "rebase", "--abort")
}

func TestContributionHistoryConflictResolutionKeepsRebaseEvidence(t *testing.T) {
	_, linkedDir, _, remoteHead, ontoHead, cleanup := seedContributionHistoryConflictRepo(t)
	defer cleanup()

	writeFile(t, linkedDir, "conflict.txt", "resolved after rebase\n")
	runGit(t, linkedDir, "add", "conflict.txt")
	runGit(t, linkedDir, "-c", "core.editor=true", "rebase", "--continue")
	localHead := strings.TrimSpace(runGit(t, linkedDir, "rev-parse", "HEAD"))
	result, err := NewGitOperator(linkedDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	if result.Kind != contributionHistoryKindLocalRebase || result.Reason != contributionHistoryReasonMatched {
		t.Fatalf("result = %+v, want matched conflict-resolved rebase", result)
	}
	if result.OntoHead != ontoHead {
		t.Fatalf("onto_head = %q, want %q", result.OntoHead, ontoHead)
	}
	if result.TaskCommitCount == nil || *result.TaskCommitCount != 1 {
		t.Fatalf("task_commit_count = %v, want 1", result.TaskCommitCount)
	}
	if result.PublishedCommitCount == nil || *result.PublishedCommitCount != 1 {
		t.Fatalf("published_commit_count = %v, want 1", result.PublishedCommitCount)
	}
	if result.NewBaseCommitCount == nil || *result.NewBaseCommitCount != 1 {
		t.Fatalf("new_base_commit_count = %v, want 1", result.NewBaseCommitCount)
	}
}

func TestContributionHistoryMergeRangeOmitsPublishedCount(t *testing.T) {
	_, linkedDir, localHead, remoteHead, ontoHead, cleanup := seedContributionHistoryMergeRepo(t)
	defer cleanup()

	result, err := NewGitOperator(linkedDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	if result.Kind != contributionHistoryKindLocalRebase || result.Reason != contributionHistoryReasonMatched {
		t.Fatalf("result = %+v, want matched rebase with a merge range", result)
	}
	if result.OntoHead != ontoHead {
		t.Fatalf("onto_head = %q, want %q", result.OntoHead, ontoHead)
	}
	if result.PublishedCommitCount != nil {
		t.Fatalf("published_commit_count = %v, want omitted for a merge range", result.PublishedCommitCount)
	}
}

func TestContributionHistoryReflogObservationUsesTwoHundredEntryBound(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	localHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	argsFile := filepath.Join(t.TempDir(), "git-args")
	installContributionHistoryGitShim(t, contributionHistoryGitShimBound)
	t.Setenv(contributionHistoryGitShimArgsFileEnv, argsFile)

	result, err := NewGitOperator(repoDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "main", localHead, localHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonNoMatch)
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read captured git arguments: %v", err)
	}
	if !strings.Contains(string(args), "-n200") {
		t.Fatalf("reflog command arguments = %q, want -n200", args)
	}
}

func TestContributionHistoryReflogOverflowFallsBackToBoundedNeutral(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	installContributionHistoryGitShim(t, contributionHistoryGitShimOverflow)
	t.Setenv(contributionHistoryGitShimHeadEnv, head)

	result, err := NewGitOperator(repoDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "main", head, head)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonLimit)
}

func TestContributionHistoryCommitCountBoundary(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-b", "feature/count-limit")
	for index := 0; index < contributionHistoryCommitLimit+1; index++ {
		runGit(t, repoDir, "commit", "--allow-empty", "-m", fmt.Sprintf("count commit %d", index))
	}
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	limited := operator.contributionHistoryCount(context.Background(), base, head)
	if !limited.limited || limited.count != nil {
		t.Fatalf("count at limit = %+v, want limited with no count", limited)
	}

	underLimitHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD~1"))
	underLimit := operator.contributionHistoryCount(context.Background(), base, underLimitHead)
	if underLimit.limited || underLimit.count == nil || *underLimit.count != contributionHistoryCommitLimit {
		t.Fatalf("count below limit = %+v, want exact count %d", underLimit, contributionHistoryCommitLimit)
	}
}

func TestContributionHistoryRunningCommandStopsAtObservationDeadline(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	installContributionHistoryGitShim(t, contributionHistoryGitShimTimeout)

	started := time.Now()
	result, err := NewGitOperator(repoDir, newTestLogger(t), nil).ExplainContributionHistory(
		context.Background(), "main", head, head)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("observation took %s, want it to stop near the 2-second deadline", elapsed)
	}
	assertContributionHistoryNeutral(t, result, contributionHistoryReasonUnavailable)
}

func installContributionHistoryGitShim(t *testing.T, mode string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("exec.LookPath(git) returned error: %v", err)
	}
	shimDir := t.TempDir()
	shimName := "git"
	if runtime.GOOS == "windows" {
		shimName += ".exe"
	}
	shimPath := filepath.Join(shimDir, shimName)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	if err := os.WriteFile(shimPath, contents, 0o755); err != nil {
		t.Fatalf("write Git shim: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(shimPath, 0o755); err != nil {
			t.Fatalf("make Git shim executable: %v", err)
		}
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(contributionHistoryGitShimModeEnv, mode)
	t.Setenv(contributionHistoryGitShimRealGitEnv, realGit)
}

func assertContributionHistoryNeutral(t *testing.T, result *ContributionHistoryExplanationResult, reason string) {
	t.Helper()
	if result.Kind != contributionHistoryKindUnexplained || result.Reason != reason {
		t.Fatalf("result = %+v, want unexplained/%s", result, reason)
	}
	if result.OntoHead != "" || result.TaskCommitCount != nil || result.PublishedCommitCount != nil || result.NewBaseCommitCount != nil {
		t.Fatalf("result = %+v, want no inferred rebase details", result)
	}
}

func seedContributionHistoryConflictRepo(t *testing.T) (repoDir, linkedDir, currentHead, remoteHead, ontoHead string, cleanup func()) {
	t.Helper()
	repoDir, cleanup = setupTestRepo(t)
	runGit(t, repoDir, "checkout", "-b", "feature/history")
	writeFile(t, repoDir, "conflict.txt", "feature version\n")
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "feature conflict commit")
	runGit(t, repoDir, "push", "-u", "origin", "feature/history")
	remoteHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	runGit(t, repoDir, "checkout", "main")
	writeFile(t, repoDir, "conflict.txt", "base version\n")
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "base conflict commit")
	runGit(t, repoDir, "push", "origin", "main")
	ontoHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "origin/main"))

	linkedDir = filepath.Join(t.TempDir(), "linked-feature")
	runGit(t, repoDir, "worktree", "add", linkedDir, "feature/history")
	runGitExpectFailure(t, linkedDir, "rebase", "origin/main")
	currentHead = strings.TrimSpace(runGit(t, linkedDir, "rev-parse", "HEAD"))
	return repoDir, linkedDir, currentHead, remoteHead, ontoHead, cleanup
}

func seedContributionHistoryMergeRepo(t *testing.T) (repoDir, linkedDir, localHead, remoteHead, ontoHead string, cleanup func()) {
	t.Helper()
	repoDir, cleanup = setupTestRepo(t)
	runGit(t, repoDir, "checkout", "-b", "feature/history")
	writeFile(t, repoDir, "feature-one.txt", "feature one\n")
	runGit(t, repoDir, "add", "feature-one.txt")
	runGit(t, repoDir, "commit", "-m", "feature one")

	runGit(t, repoDir, "checkout", "main")
	writeFile(t, repoDir, "base-one.txt", "base one\n")
	runGit(t, repoDir, "add", "base-one.txt")
	runGit(t, repoDir, "commit", "-m", "base one")
	runGit(t, repoDir, "push", "origin", "main")

	runGit(t, repoDir, "checkout", "feature/history")
	writeFile(t, repoDir, "feature-two.txt", "feature two\n")
	runGit(t, repoDir, "add", "feature-two.txt")
	runGit(t, repoDir, "commit", "-m", "feature two")
	runGit(t, repoDir, "merge", "--no-ff", "main", "-m", "merge main into feature")
	runGit(t, repoDir, "push", "-u", "origin", "feature/history")
	remoteHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	runGit(t, repoDir, "checkout", "main")
	writeFile(t, repoDir, "base-two.txt", "base two\n")
	runGit(t, repoDir, "add", "base-two.txt")
	runGit(t, repoDir, "commit", "-m", "base two")
	runGit(t, repoDir, "push", "origin", "main")
	ontoHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "origin/main"))

	linkedDir = filepath.Join(t.TempDir(), "linked-feature")
	runGit(t, repoDir, "worktree", "add", linkedDir, "feature/history")
	runGit(t, linkedDir, "rebase", "origin/main")
	localHead = strings.TrimSpace(runGit(t, linkedDir, "rev-parse", "HEAD"))
	return repoDir, linkedDir, localHead, remoteHead, ontoHead, cleanup
}

func runGitExpectFailure(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = filterTestGitEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("git %v succeeded, want failure", args)
	}
	return string(out)
}
