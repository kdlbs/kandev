package orchestrator

import (
	"context"
	"expvar"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// commitCaptureFetchFailedCount reads the current commit_capture_fetch_failed
// count for trigger, treating an unset key as 0.
func commitCaptureFetchFailedCount(trigger commitCaptureTrigger) int64 {
	v := commitCaptureFetchFailed.Get(string(trigger))
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// Regression: captureCommitsForTrigger silently ignored a result-level
// GetGitLog failure (HTTP 200 but Success == false) - it only checked the
// transport err and a nil result, so a git command failure inside agentctl
// (e.g. corrupted worktree, git binary missing) produced zero commits and
// zero observability. This undercuts the writer-health goal recorded
// alongside commitCaptureFailed/commitCaptureObserved: a trigger silently
// going dark should be visible.
func TestCaptureCommitsForTrigger_RecordsTotalFetchFailure(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-fetch-fail", "s-fetch-fail", "step1")

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			return &client.GitLogResult{Success: false, Error: "git log failed: fatal: bad object"}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	before := commitCaptureFetchFailedCount(commitCaptureTriggerSweep)
	svc.captureCommitsForTrigger(ctx, "s-fetch-fail", "base-sha", "", commitCaptureTriggerSweep)
	after := commitCaptureFetchFailedCount(commitCaptureTriggerSweep)

	if after != before+1 {
		t.Fatalf("commit_capture_fetch_failed[sweep] = %d, want %d (a total-failure GetGitLog result must be recorded)", after, before+1)
	}

	commits, err := testRepo.GetSessionCommits(ctx, "s-fetch-fail")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("commit rows = %d, want 0 (nothing to persist on total failure)", len(commits))
	}
}

// Regression: a multi-repo GetGitLog response can report overall Success ==
// true (at least one repo succeeded) while PerRepoErrors names the repos
// that failed - captureCommitsForTrigger persisted the successful commits
// but never recorded or logged the per-repo failures, so a repo silently
// going uncaptured inside an otherwise-succeeding multi-repo session was
// invisible.
func TestCaptureCommitsForTrigger_RecordsPartialMultiRepoFailure(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-partial-fail", "s-partial-fail", "step1")

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			return &client.GitLogResult{
				Success: true,
				Commits: []*client.GitCommitInfo{
					{CommitSHA: "ok-sha", CommitMessage: "feat: from the healthy repo", CommittedAt: "2026-04-02T03:04:05Z", Insertions: 2},
				},
				PerRepoErrors: []client.GitLogRepoError{
					{RepositoryName: "flaky-repo", Error: "git log failed: repository not found"},
				},
			}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	before := commitCaptureFetchFailedCount(commitCaptureTriggerLive)
	svc.captureCommitsForTrigger(ctx, "s-partial-fail", "base-sha", "", commitCaptureTriggerLive)
	after := commitCaptureFetchFailedCount(commitCaptureTriggerLive)

	if after != before+1 {
		t.Fatalf("commit_capture_fetch_failed[live] = %d, want %d (a per-repo failure inside a successful multi-repo result must be recorded)", after, before+1)
	}

	commits, err := testRepo.GetSessionCommits(ctx, "s-partial-fail")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].CommitSHA != "ok-sha" {
		t.Fatalf("commits = %+v, want exactly [ok-sha] (the healthy repo's commit must still persist despite the other repo's failure)", commits)
	}
}

// Regression: a multi-repo total failure (every repo failed) reports
// Success == false AND populates PerRepoErrors for every repo - the two
// signals must not be double-counted into commit_capture_fetch_failed.
func TestCaptureCommitsForTrigger_TotalMultiRepoFailureCountsOncePerRepo(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-total-multi-fail", "s-total-multi-fail", "step1")

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			return &client.GitLogResult{
				Success: false,
				Error:   "git log failed for all 2 repositories",
				PerRepoErrors: []client.GitLogRepoError{
					{RepositoryName: "repo-a", Error: "git log failed: repository not found"},
					{RepositoryName: "repo-b", Error: "git log failed: permission denied"},
				},
			}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	before := commitCaptureFetchFailedCount(commitCaptureTriggerArchive)
	svc.captureCommitsForTrigger(ctx, "s-total-multi-fail", "base-sha", "", commitCaptureTriggerArchive)
	after := commitCaptureFetchFailedCount(commitCaptureTriggerArchive)

	if after != before+2 {
		t.Fatalf("commit_capture_fetch_failed[archive] = %d, want %d (one increment per failed repo, not one more for the overall Success=false)", after, before+2)
	}
}
