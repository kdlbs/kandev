package process

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContributionHistoryLocalRebase(t *testing.T) {
	repoDir, linkedDir, localHead, remoteHead, onto, cleanup := seedContributionHistoryRepo(t)
	defer cleanup()

	operator := NewGitOperator(linkedDir, newTestLogger(t), nil)
	result, err := operator.ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	if result.Kind != "local_rebase" || result.Reason != "matched_reflog" {
		t.Fatalf("result = %+v, want a matched local rebase", result)
	}
	if result.Branch != "feature/history" || result.ExpectedLocalHead != localHead ||
		result.ExpectedRemoteHead != remoteHead {
		t.Fatalf("result identity = %+v, want branch and both requested heads", result)
	}
	if result.OntoHead != onto {
		t.Fatalf("onto_head = %q, want %q", result.OntoHead, onto)
	}
	if result.TaskCommitCount == nil || *result.TaskCommitCount != 5 {
		t.Fatalf("task_commit_count = %v, want 5", result.TaskCommitCount)
	}
	if result.PublishedCommitCount == nil || *result.PublishedCommitCount != 5 {
		t.Fatalf("published_commit_count = %v, want 5", result.PublishedCommitCount)
	}
	if result.NewBaseCommitCount == nil || *result.NewBaseCommitCount != 29 {
		t.Fatalf("new_base_commit_count = %v, want 29", result.NewBaseCommitCount)
	}

	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")); got != "main" {
		t.Fatalf("fixture root branch = %q, want main", got)
	}
}

func TestContributionHistoryUnknown(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/history")
	writeFile(t, repoDir, "feature.txt", "feature\n")
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature commit")
	localHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	remoteHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "origin/main"))

	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := operator.ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	if result.Kind != "unexplained" || result.Reason != "no_matching_reflog" {
		t.Fatalf("result = %+v, want a neutral no-matching-reflog result", result)
	}
	if result.OntoHead != "" || result.TaskCommitCount != nil ||
		result.PublishedCommitCount != nil || result.NewBaseCommitCount != nil {
		t.Fatalf("result = %+v, want no inferred rebase details", result)
	}

	stale, err := operator.ExplainContributionHistory(
		context.Background(), "feature/history", strings.Repeat("a", 40), remoteHead)
	if err != nil {
		t.Fatalf("stale ExplainContributionHistory returned error: %v", err)
	}
	if stale.Kind != "unexplained" || stale.Reason != "stale" {
		t.Fatalf("stale result = %+v, want a stale neutral result", stale)
	}
}

func TestContributionHistoryCounts(t *testing.T) {
	_, linkedDir, localHead, remoteHead, _, cleanup := seedContributionHistoryRepo(t)
	defer cleanup()

	operator := NewGitOperator(linkedDir, newTestLogger(t), nil)
	result, err := operator.ExplainContributionHistory(
		context.Background(), "feature/history", localHead, remoteHead)
	if err != nil {
		t.Fatalf("ExplainContributionHistory returned error: %v", err)
	}
	for name, value := range map[string]*int{
		"task":      result.TaskCommitCount,
		"published": result.PublishedCommitCount,
		"new base":  result.NewBaseCommitCount,
	} {
		if value == nil {
			t.Errorf("%s count is nil, want an exact count", name)
		}
	}
}

func TestContributionHistoryBounds(t *testing.T) {
	writer := newContributionHistoryOutputWriter(8)
	written, err := writer.Write([]byte("123456789"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if written != 9 {
		t.Fatalf("Write returned %d, want the complete input length", written)
	}
	if got := writer.String(); got != "12345678" {
		t.Fatalf("captured output = %q, want the configured bound", got)
	}
	if !writer.Truncated() {
		t.Fatal("Truncated = false, want true after exceeding the bound")
	}
}

func TestContributionHistoryCancellation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	branch := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	result, err := NewGitOperator(repoDir, newTestLogger(t), nil).ExplainContributionHistory(
		ctx, branch, head, head)
	if err != nil {
		t.Fatalf("cancelled ExplainContributionHistory returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled observation took %s, want prompt return", elapsed)
	}
	if result.Kind != "unexplained" || result.Reason != "unavailable" {
		t.Fatalf("result = %+v, want an unavailable neutral result", result)
	}
}

func seedContributionHistoryRepo(t *testing.T) (repoDir, linkedDir, localHead, remoteHead, onto string, cleanup func()) {
	t.Helper()
	repoDir, cleanup = setupTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature/history")
	for i := 1; i <= 5; i++ {
		writeFile(t, repoDir, fmt.Sprintf("feature-commit-%d.txt", i), "feature\n")
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "feature commit")
	}
	runGit(t, repoDir, "push", "-u", "origin", "feature/history")
	remoteHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	runGit(t, repoDir, "checkout", "main")
	for i := 1; i <= 29; i++ {
		writeFile(t, repoDir, fmt.Sprintf("base-commit-%d.txt", i), "base\n")
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "base commit")
	}
	runGit(t, repoDir, "push", "origin", "main")
	onto = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "origin/main"))

	linkedDir = filepath.Join(t.TempDir(), "linked-feature")
	runGit(t, repoDir, "worktree", "add", linkedDir, "feature/history")
	runGit(t, linkedDir, "rebase", "origin/main")
	localHead = strings.TrimSpace(runGit(t, linkedDir, "rev-parse", "HEAD"))
	return repoDir, linkedDir, localHead, remoteHead, onto, cleanup
}
