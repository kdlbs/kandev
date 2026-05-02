package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Regression: when the agent renames or switches branches inside a session,
// handleBranchSwitched must persist the new branch to task_session_worktrees.
// Without this, downstream PR watch reconciliation keeps resolving the stale
// worktree_branch and fails to associate PRs created on the new branch.
func TestHandleBranchSwitched_UpdatesWorktreeBranch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t1", "s1", "step1")

	// Seed a repository + worktree on the original branch.
	rObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := testRepo.CreateRepository(ctx, rObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	wt := &models.TaskSessionWorktree{
		ID: "wt-s1", SessionID: "s1",
		WorktreeID: "wtree-s1", RepositoryID: "repo1",
		WorktreeBranch: "feature/a", CreatedAt: now,
	}
	if err := testRepo.CreateTaskSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/a",
			CurrentBranch:  "feature/b",
			BaseCommit:     "deadbeef",
		},
	})

	// Verify the DB now reflects the new branch.
	wts, err := testRepo.ListTaskSessionWorktrees(ctx, "s1")
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].WorktreeBranch != "feature/b" {
		t.Errorf("WorktreeBranch = %q, want %q", wts[0].WorktreeBranch, "feature/b")
	}
}

// Regression: when a PR watch already exists for the session and the branch
// is switched, the watch must be reset (branch updated, pr_number cleared) so
// the poller re-searches for the PR on the new branch. This covers both
// rename and stacked-PR workflows.
func TestHandleBranchSwitched_ResetsPRWatch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t1", "s1", "step1")

	rObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := testRepo.CreateRepository(ctx, rObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	wt := &models.TaskSessionWorktree{
		ID: "wt-s1", SessionID: "s1",
		WorktreeID: "wtree-s1", RepositoryID: "repo1",
		WorktreeBranch: "feature/a", CreatedAt: now,
	}
	if err := testRepo.CreateTaskSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		prWatch: &github.PRWatch{ID: "watch-1", Branch: "feature/a", PRNumber: 42},
	}
	svc.SetGitHubService(ghSvc)

	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/a",
			CurrentBranch:  "feature/b",
			BaseCommit:     "deadbeef",
		},
	})

	if ghSvc.resetWatchCalls != 1 {
		t.Errorf("expected 1 ResetPRWatch call, got %d", ghSvc.resetWatchCalls)
	}
	if ghSvc.resetWatchBranch != "feature/b" {
		t.Errorf("reset watch branch = %q, want feature/b", ghSvc.resetWatchBranch)
	}
}

// shouldFirePushDetection is the trigger predicate for trackPushAndAssociatePR.
// The first-observation case is the regression that was breaking multi-repo
// PR detection in production: any repo whose first agentctl status poll landed
// after the push had completed would silently never get its PR associated,
// because the >0→0 transition was never observed. See task
// 4fdff41b-095a-4158-a311-4a1a23abe064 for the original failure mode.
func TestShouldFirePushDetection(t *testing.T) {
	tests := []struct {
		name    string
		loaded  bool
		prev    int
		status  *lifecycle.GitStatusData
		wantOn  bool
		wantWhy string
	}{
		{
			name:    "first observation, ahead=0 with remote branch fires (push happened pre-poll)",
			loaded:  false,
			prev:    0,
			status:  &lifecycle.GitStatusData{Ahead: 0, RemoteBranch: "origin/feature/x"},
			wantOn:  true,
			wantWhy: "first-observation sync — pre-existing remote branch in synced state means a push completed before we started watching",
		},
		{
			name:   "first observation, no remote branch does not fire",
			loaded: false,
			prev:   0,
			// Branch never been pushed — RemoteBranch is empty.
			status: &lifecycle.GitStatusData{Ahead: 0, RemoteBranch: ""},
			wantOn: false,
		},
		{
			name:   "first observation, ahead>0 does not fire (waiting for transition)",
			loaded: false,
			prev:   0,
			status: &lifecycle.GitStatusData{Ahead: 3, RemoteBranch: "origin/feature/x"},
			wantOn: false,
		},
		{
			name:   "transition >0 to 0 with remote fires (legacy in-session push)",
			loaded: true,
			prev:   2,
			status: &lifecycle.GitStatusData{Ahead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: true,
		},
		{
			name:   "stays at 0 after first fire does not refire",
			loaded: true,
			prev:   0,
			status: &lifecycle.GitStatusData{Ahead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: false,
		},
		{
			name:   "transition with no remote does not fire (local-only commit was undone)",
			loaded: true,
			prev:   1,
			status: &lifecycle.GitStatusData{Ahead: 0, RemoteBranch: ""},
			wantOn: false,
		},
		{
			name:   "nil status does not fire",
			loaded: true,
			prev:   1,
			status: nil,
			wantOn: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFirePushDetection(tt.loaded, tt.prev, tt.status)
			if got != tt.wantOn {
				t.Errorf("shouldFirePushDetection = %v, want %v", got, tt.wantOn)
			}
		})
	}
}

// pushTrackerForget must drop every entry for the given session, regardless of
// how many repos are tracked under it. Multi-repo tasks accumulate one entry
// per repo (key = "session|repo"); leaving them behind on session delete
// would slowly leak memory across the lifetime of the process.
func TestPushTrackerForget(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	// Three sessions, multiple repos for s1.
	svc.pushTracker.Store(pushTrackerKey("s1", "frontend"), 0)
	svc.pushTracker.Store(pushTrackerKey("s1", "backend"), 0)
	svc.pushTracker.Store(pushTrackerKey("s1", ""), 0) // legacy single-repo key
	svc.pushTracker.Store(pushTrackerKey("s2", "frontend"), 0)
	// "s10" is a guard against accidental prefix-match: "s1|" must not match "s10|".
	svc.pushTracker.Store(pushTrackerKey("s10", "frontend"), 0)

	svc.pushTrackerForget("s1")

	for _, k := range []string{"s1|frontend", "s1|backend", "s1|"} {
		if _, ok := svc.pushTracker.Load(k); ok {
			t.Errorf("expected %q to be removed", k)
		}
	}
	for _, k := range []string{"s2|frontend", "s10|frontend"} {
		if _, ok := svc.pushTracker.Load(k); !ok {
			t.Errorf("expected %q to survive (different session)", k)
		}
	}
}
