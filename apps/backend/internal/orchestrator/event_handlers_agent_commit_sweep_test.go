package orchestrator

import (
	"context"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// Regression: the live commit-created event can miss a commit (e.g. pushed
// just before agentctl's next poll tick, see filterLocalCommits' upstream
// filter). captureSessionCommitsSweep is the per-turn reconcile pass that
// runs while the agent process is still alive - unlike archive capture,
// whose GetGitLog call usually finds it already gone - and must persist any
// commit the live path missed.
func TestCaptureSessionCommitsSweep_FillsCommitLivePathMissed(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-sweep", "s-sweep", "step1")

	session, err := testRepo.GetTaskSession(ctx, "s-sweep")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(_ context.Context, sessionID, baseCommit string, _ int, _ string) (*client.GitLogResult, error) {
			if sessionID != "s-sweep" || baseCommit != "base-sha" {
				t.Fatalf("GetGitLog called with (%q, %q), want (s-sweep, base-sha)", sessionID, baseCommit)
			}
			return &client.GitLogResult{
				Success: true,
				Commits: []*client.GitCommitInfo{
					{CommitSHA: "missed-sha", CommitMessage: "feat: missed by live path", CommittedAt: "2026-04-02T03:04:05Z", Insertions: 5},
				},
			}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.captureSessionCommitsSweep(ctx, "s-sweep")

	commits, err := testRepo.GetSessionCommits(ctx, "s-sweep")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("commit rows = %d, want 1", len(commits))
	}
	if commits[0].CommitSHA != "missed-sha" {
		t.Errorf("CommitSHA = %q, want missed-sha", commits[0].CommitSHA)
	}
}

// Regression: when the agent process is gone (GetGitLog returns a nil
// result with a nil error, matching the documented "agent not running"
// contract), the sweep must not error or persist anything - archive capture
// remains the final safety net.
func TestCaptureSessionCommitsSweep_AgentNotRunningIsNoop(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-sweep-gone", "s-sweep-gone", "step1")

	session, err := testRepo.GetTaskSession(ctx, "s-sweep-gone")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			return nil, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.captureSessionCommitsSweep(ctx, "s-sweep-gone")

	commits, err := testRepo.GetSessionCommits(ctx, "s-sweep-gone")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("commit rows = %d, want 0", len(commits))
	}
}
