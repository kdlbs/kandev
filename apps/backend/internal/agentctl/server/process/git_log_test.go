package process

import (
	"context"
	"strings"
	"testing"
)

// TestGetLog_NoBaseReturnsFullGraph guards against a regression where
// --first-parent gets applied unconditionally to GetLog. The flag is only
// correct for the divergence-range path (baseCommit != ""); the open-ended
// "recent N commits" path must keep returning the full commit graph so
// future history-view callers (activity widgets, etc.) aren't silently
// filtered.
func TestGetLog_NoBaseReturnsFullGraph(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	ctx := context.Background()

	// Build a graph where main has commits brought in via a merge from a
	// side branch. Without --first-parent, GetLog returns all commits;
	// with --first-parent, only the merge commit's first-parent line.
	branchPoint := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-b", "side", branchPoint)
	writeFile(t, repoDir, "side.txt", "side\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "side: commit a")
	sideCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "Merge side", "side")

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.GetLog(ctx, "", 0)
	if err != nil {
		t.Fatalf("GetLog returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetLog failed: %s", result.Error)
	}
	gotSHAs := make(map[string]bool, len(result.Commits))
	for _, c := range result.Commits {
		gotSHAs[c.CommitSHA] = true
	}
	if !gotSHAs[sideCommit] {
		t.Errorf("expected side-branch commit %s in no-base log (full graph), missing — --first-parent likely leaked into this path", sideCommit)
	}
}

// TestGetLog_StaleLocalBranchScenario reproduces the bug where a worktree's
// local main has fallen behind origin/main. If the merge-base is computed
// against the local ref, it returns an old SHA and the log range sweeps in
// dozens of commits that aren't actually on the feature branch. The fix is
// to compute merge-base against origin/<branch> — exercised at the API layer
// in TestRunGitLogForRepo_PrefersOriginRef. This test documents the shape of
// the underlying git scenario.
func TestGetLog_StaleLocalBranchScenario(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	ctx := context.Background()

	// Capture initial main tip — local main will stay pinned here while
	// the remote moves forward (simulating a worktree that hasn't fetched).
	staleLocalMain := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Push the staleLocalMain so origin has it, then advance both local main
	// and origin/main with extra commits, then reset local main back so it
	// looks stale.
	for i := 0; i < 3; i++ {
		writeFile(t, repoDir, "main-x.txt", strings.Repeat("main x\n", i+1))
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "chore: main forward")
	}
	advancedMain := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "push", "origin", "main")
	// Reset local main back to staleLocalMain (simulates "didn't fetch").
	runGit(t, repoDir, "reset", "--hard", staleLocalMain)
	// Now refetch so origin/main is ahead but local main isn't.
	runGit(t, repoDir, "fetch", "origin", "refs/heads/main:refs/remotes/origin/main")

	// Branch from origin/main (the current upstream tip).
	runGit(t, repoDir, "checkout", "-b", "feature/x", advancedMain)
	writeFile(t, repoDir, "feature.txt", "feature work\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feat: feature work")

	gitOp := NewGitOperator(repoDir, log, nil)

	// Computing merge-base against the stale LOCAL main returns the older
	// SHA, and the log range balloons to include the 3 main commits plus
	// the branch's own commit.
	staleMergeBase, err := gitOp.GetMergeBase(ctx, "HEAD", "main")
	if err != nil {
		t.Fatalf("merge-base local main failed: %v", err)
	}
	if staleMergeBase != staleLocalMain {
		t.Fatalf("expected stale merge-base = %s, got %s", staleLocalMain, staleMergeBase)
	}
	staleResult, err := gitOp.GetLog(ctx, staleMergeBase, 0)
	if err != nil {
		t.Fatalf("GetLog with stale base failed: %v", err)
	}
	if len(staleResult.Commits) != 4 {
		t.Errorf("stale local main: expected 4 commits leaked in, got %d", len(staleResult.Commits))
	}

	// Computing against origin/main returns the correct base — only the
	// branch's own commit shows up.
	correctMergeBase, err := gitOp.GetMergeBase(ctx, "HEAD", "origin/main")
	if err != nil {
		t.Fatalf("merge-base origin/main failed: %v", err)
	}
	if correctMergeBase != advancedMain {
		t.Fatalf("expected correct merge-base = %s, got %s", advancedMain, correctMergeBase)
	}
	correctResult, err := gitOp.GetLog(ctx, correctMergeBase, 0)
	if err != nil {
		t.Fatalf("GetLog with correct base failed: %v", err)
	}
	if len(correctResult.Commits) != 1 {
		t.Errorf("origin/main: expected 1 branch-only commit, got %d", len(correctResult.Commits))
	}
}

// TestGetLog_FirstParentSkipsMergedInCommits verifies that GetLog uses
// --first-parent so commits brought in via a merge from main do not appear in
// the branch's commit list. Without --first-parent, "git log <merge-base>..HEAD"
// follows both parents of the merge commit and surfaces main's commits, which
// confuses users expecting "what this branch did".
func TestGetLog_FirstParentSkipsMergedInCommits(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	ctx := context.Background()

	// Capture the original main tip — used as the branch point.
	branchPoint := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Create a feature branch and make one commit.
	runGit(t, repoDir, "checkout", "-b", "feature/x")
	writeFile(t, repoDir, "feature.txt", "feature work\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feat: feature work")
	featureCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Move main forward with two unrelated commits.
	runGit(t, repoDir, "checkout", "main")
	writeFile(t, repoDir, "main-a.txt", "main a\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "chore: main a")
	mainA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "main-b.txt", "main b\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "chore: main b")
	mainB := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Merge main into feature/x with --no-ff so a real merge commit is created.
	runGit(t, repoDir, "checkout", "feature/x")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "Merge main into feature/x", "main")
	mergeCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// One more commit on the feature branch after the merge.
	writeFile(t, repoDir, "feature2.txt", "more feature work\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feat: more feature work")
	postMerge := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.GetLog(ctx, branchPoint, 0)
	if err != nil {
		t.Fatalf("GetLog returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetLog failed: %s", result.Error)
	}

	gotSHAs := make(map[string]bool, len(result.Commits))
	for _, c := range result.Commits {
		gotSHAs[c.CommitSHA] = true
	}

	wantPresent := []struct {
		sha   string
		label string
	}{
		{postMerge, "post-merge feature commit"},
		{mergeCommit, "merge commit"},
		{featureCommit, "initial feature commit"},
	}
	for _, w := range wantPresent {
		if !gotSHAs[w.sha] {
			t.Errorf("expected %s (%s) in log, missing", w.label, w.sha)
		}
	}

	wantAbsent := []struct {
		sha   string
		label string
	}{
		{mainA, "main commit a (came in via merge)"},
		{mainB, "main commit b (came in via merge)"},
	}
	for _, w := range wantAbsent {
		if gotSHAs[w.sha] {
			t.Errorf("expected %s (%s) to be absent (--first-parent should skip it)", w.label, w.sha)
		}
	}

	if len(result.Commits) != 3 {
		shas := make([]string, 0, len(result.Commits))
		for _, c := range result.Commits {
			shas = append(shas, c.CommitSHA[:7]+" "+c.CommitMessage)
		}
		t.Errorf("expected exactly 3 commits with --first-parent, got %d:\n%s",
			len(result.Commits), strings.Join(shas, "\n"))
	}
}
