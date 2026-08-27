package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/office/costs/modelsdev"
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
	if err := testRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-s1", TaskID: "t1", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	session, err := testRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.TaskEnvironmentID = "env-s1"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link session to environment: %v", err)
	}
	wt := &models.TaskEnvironmentRepo{
		ID: "wt-s1", TaskEnvironmentID: "env-s1",
		WorktreeID: "wtree-s1", RepositoryID: "repo1",
		WorktreeBranch: "feature/a", CreatedAt: now,
	}
	if err := testRepo.CreateTaskEnvironmentRepo(ctx, wt); err != nil {
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

func TestGitStatusHashIncludesRepositoryName(t *testing.T) {
	status := &lifecycle.GitStatusData{RepositoryName: "backend", Branch: "feature/x", HeadCommit: "abc"}
	other := *status
	other.RepositoryName = "frontend"
	if gitStatusHash(status) == gitStatusHash(&other) {
		t.Fatal("status snapshots from different repositories must not share a hash")
	}
}

func TestHandleBranchSwitched_RepositoryScopedUpdateKeepsSiblingBranch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	taskRoot := t.TempDir()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-multi", "s-multi", "step1")
	for _, repository := range []*models.Repository{
		{ID: "repo-backend", WorkspaceID: "ws1", Name: "backend", CreatedAt: now, UpdatedAt: now},
		{ID: "repo-frontend", WorkspaceID: "ws1", Name: "frontend", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, testRepo.CreateRepository(ctx, repository))
	}

	require.NoError(t, testRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-multi", TaskID: "t-multi", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}))
	multiSession, err := testRepo.GetTaskSession(ctx, "s-multi")
	require.NoError(t, err)
	multiSession.TaskEnvironmentID = "env-multi"
	require.NoError(t, testRepo.UpdateTaskSession(ctx, multiSession))
	require.NoError(t, testRepo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID: "wt-multi-backend", TaskEnvironmentID: "env-multi", WorktreeID: "worktree-backend", RepositoryID: "repo-backend",
		WorktreePath: filepath.Join(taskRoot, "backend"), WorktreeBranch: "feature/backend-old", Position: 0, CreatedAt: now,
	}))
	require.NoError(t, testRepo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID: "wt-multi-frontend", TaskEnvironmentID: "env-multi", WorktreeID: "worktree-frontend", RepositoryID: "repo-frontend",
		WorktreePath: filepath.Join(taskRoot, "frontend"), WorktreeBranch: "feature/frontend-old", Position: 1, CreatedAt: now,
	}))

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID: "t-multi", SessionID: "s-multi",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/backend-old", CurrentBranch: "feature/backend-new", RepositoryName: "backend",
		},
	})

	worktrees, err := testRepo.ListTaskSessionWorktrees(ctx, "s-multi")
	require.NoError(t, err)
	require.Len(t, worktrees, 2)
	branches := make(map[string]string, len(worktrees))
	for _, worktree := range worktrees {
		branches[worktree.RepositoryID] = worktree.WorktreeBranch
	}
	require.Equal(t, "feature/backend-new", branches["repo-backend"])
	require.Equal(t, "feature/frontend-old", branches["repo-frontend"])
}

func TestHandleBranchSwitched_UnknownRepositoryNameDoesNotOverwriteSiblings(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	taskRoot := t.TempDir()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-multi-unknown", "s-multi-unknown", "step1")
	for _, repository := range []*models.Repository{
		{ID: "repo-unknown-backend", WorkspaceID: "ws1", Name: "backend", CreatedAt: now, UpdatedAt: now},
		{ID: "repo-unknown-frontend", WorkspaceID: "ws1", Name: "frontend", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, testRepo.CreateRepository(ctx, repository))
	}

	require.NoError(t, testRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-multi-unknown", TaskID: "t-multi-unknown", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}))
	multiUnknownSession, err := testRepo.GetTaskSession(ctx, "s-multi-unknown")
	require.NoError(t, err)
	multiUnknownSession.TaskEnvironmentID = "env-multi-unknown"
	require.NoError(t, testRepo.UpdateTaskSession(ctx, multiUnknownSession))
	require.NoError(t, testRepo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID: "wt-unknown-backend", TaskEnvironmentID: "env-multi-unknown", WorktreeID: "worktree-unknown-backend", RepositoryID: "repo-unknown-backend",
		WorktreePath: filepath.Join(taskRoot, "backend"), WorktreeBranch: "feature/backend-old", Position: 0, CreatedAt: now,
	}))
	require.NoError(t, testRepo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID: "wt-unknown-frontend", TaskEnvironmentID: "env-multi-unknown", WorktreeID: "worktree-unknown-frontend", RepositoryID: "repo-unknown-frontend",
		WorktreePath: filepath.Join(taskRoot, "frontend"), WorktreeBranch: "feature/frontend-old", Position: 1, CreatedAt: now,
	}))

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID: "t-multi-unknown", SessionID: "s-multi-unknown",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/old", CurrentBranch: "feature/new", RepositoryName: "unknown-repository",
		},
	})

	worktrees, err := testRepo.ListTaskSessionWorktrees(ctx, "s-multi-unknown")
	require.NoError(t, err)
	branches := make(map[string]string, len(worktrees))
	for _, worktree := range worktrees {
		branches[worktree.RepositoryID] = worktree.WorktreeBranch
	}
	require.Equal(t, "feature/backend-old", branches["repo-unknown-backend"])
	require.Equal(t, "feature/frontend-old", branches["repo-unknown-frontend"])
}

// seedBranchSwitchSession wires the task/session/repository/worktree rows
// handleBranchSwitched needs to resolve a session's repository, with the
// session's worktree parked on startBranch.
func seedBranchSwitchSession(t *testing.T, startBranch string) (*Service, *mockGitHubService) {
	t.Helper()
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
	if err := testRepo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	if err := testRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-s1", TaskID: "t1", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{
				ID: "wt-s1", WorktreeID: "wtree-s1", RepositoryID: "repo1",
				WorktreeBranch: startBranch, CreatedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	session, err := testRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.TaskEnvironmentID = "env-s1"
	session.RepositoryID = "repo1"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link session to environment: %v", err)
	}

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)
	return svc, ghSvc
}

// switchBranch tags the event with RepositoryName, so these tests drive
// resolvePushRepo's named-repository routing — the multi-repo path — rather
// than its empty-name fallback to the session's primary repo. The git-status
// test below leaves RepositoryName empty and covers the fallback.
func switchBranch(t *testing.T, svc *Service, from, to string) {
	t.Helper()
	svc.handleBranchSwitched(context.Background(), watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: from,
			CurrentBranch:  to,
			BaseCommit:     "deadbeef",
			RepositoryName: "myrepo",
		},
	})
}

// Regression: a branch switch must not re-point a watch that already found a
// PR. That watch is the only handle keeping the previous branch's PR synced —
// the poller and the on-demand sync both iterate watches, and CI automation
// only runs off the events they publish. Re-pointing it froze the earlier PR
// at its last-observed checks/review state, so on a task working two branches
// auto-fix stopped seeing new failures and auto-merge never saw the PR turn
// mergeable. The new branch gets its own watch instead.
func TestHandleBranchSwitched_KeepsWatchThatFoundAPR(t *testing.T) {
	svc, ghSvc := seedBranchSwitchSession(t, "feature/a")
	ghSvc.sessionWatches = []*github.PRWatch{
		{ID: "watch-1", RepositoryID: "repo1", Branch: "feature/a", PRNumber: 42},
	}

	switchBranch(t, svc, "feature/a", "feature/b")

	if ghSvc.resetWatchCalls != 0 {
		t.Errorf("ResetPRWatch called %d times, want 0 — feature/a's PR 42 lost its watch", ghSvc.resetWatchCalls)
	}
	if ghSvc.ensureWatchCalls != 1 {
		t.Fatalf("EnsurePRWatch called %d times, want 1", ghSvc.ensureWatchCalls)
	}
	if ghSvc.ensureWatchBranch != "feature/b" {
		t.Errorf("ensured watch branch = %q, want feature/b", ghSvc.ensureWatchBranch)
	}
}

// A branch renamed before any PR existed still reuses the searching watch, so
// branch churn can't accumulate watches that will never find a PR.
func TestHandleBranchSwitched_ReusesSearchingWatch(t *testing.T) {
	svc, ghSvc := seedBranchSwitchSession(t, "feature/a")
	ghSvc.sessionWatches = []*github.PRWatch{
		{ID: "watch-1", RepositoryID: "repo1", Branch: "feature/a", PRNumber: 0},
	}

	switchBranch(t, svc, "feature/a", "feature/b")

	if ghSvc.resetWatchCalls != 1 {
		t.Errorf("ResetPRWatch called %d times, want 1", ghSvc.resetWatchCalls)
	}
	if ghSvc.resetWatchBranch != "feature/b" {
		t.Errorf("reset watch branch = %q, want feature/b", ghSvc.resetWatchBranch)
	}
	if ghSvc.ensureWatchCalls != 0 {
		t.Errorf("EnsurePRWatch called %d times, want 0 — the searching watch should be reused", ghSvc.ensureWatchCalls)
	}
}

// The git-status branch sync has the same multi-branch hazard: it used to load
// "a" watch for the session and re-point it whenever it was still searching.
// With one watch per branch that could drag a searching watch onto a branch
// another watch already covers. It must leave an already-covered branch alone.
func TestHandleGitStatusUpdate_LeavesAlreadyWatchedBranchAlone(t *testing.T) {
	svc, ghSvc := seedBranchSwitchSession(t, "feature/b")
	ghSvc.sessionWatches = []*github.PRWatch{
		{ID: "watch-1", RepositoryID: "repo1", Branch: "feature/a", PRNumber: 0},
		{ID: "watch-2", RepositoryID: "repo1", Branch: "feature/b", PRNumber: 7},
	}

	svc.handleGitStatusUpdate(context.Background(), watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		Status:    &lifecycle.GitStatusData{Branch: "feature/b"},
	})

	if ghSvc.resetWatchCalls != 0 {
		t.Errorf("ResetPRWatch called %d times, want 0 — feature/b already has watch-2", ghSvc.resetWatchCalls)
	}
}

// Switching back to a branch that already has a watch is a no-op. The previous
// code compared only the branch it happened to load and reset the watch when
// it had a PR number, so hopping between two branches repeatedly cleared each
// PR association it had already made.
func TestHandleBranchSwitched_AlreadyWatchedBranchIsNoop(t *testing.T) {
	svc, ghSvc := seedBranchSwitchSession(t, "feature/b")
	ghSvc.sessionWatches = []*github.PRWatch{
		{ID: "watch-1", RepositoryID: "repo1", Branch: "feature/a", PRNumber: 42},
		{ID: "watch-2", RepositoryID: "repo1", Branch: "feature/b", PRNumber: 43},
	}

	switchBranch(t, svc, "feature/b", "feature/a")

	if ghSvc.resetWatchCalls != 0 {
		t.Errorf("ResetPRWatch called %d times, want 0", ghSvc.resetWatchCalls)
	}
	if ghSvc.ensureWatchCalls != 0 {
		t.Errorf("EnsurePRWatch called %d times, want 0", ghSvc.ensureWatchCalls)
	}
}

// shouldFirePushDetection is the trigger predicate for trackPushAndAssociatePR.
// The first-observation case is the regression that was breaking multi-repo
// PR detection in production: any repo whose first agentctl status poll landed
// after the push had completed would silently never get its PR associated,
// because the >0→0 transition was never observed. See task
// 4fdff41b-095a-4158-a311-4a1a23abe064 for the original failure mode.
//
// Fixtures set RemoteAhead (commits ahead of this branch's own upstream),
// not Ahead (commits ahead of the base branch) — Ahead never reaches zero
// just because a branch got pushed, since pushing doesn't move the base
// branch, so it can't signal push state. This was itself a production bug
// (push detection silently never fired for any real feature branch) found
// via an e2e regression test and fixed alongside this predicate.
//
// `prev` fixtures use pushTrackerUnsynced (not 0) to represent "no upstream
// yet" — trackPushAndAssociatePR's own tracked value, not a raw prior
// RemoteAhead. Collapsing "no upstream" and "upstream, nothing to push"
// into the same 0 value was itself a production bug: a task's very first
// observation (before any push) reads as remote-ahead=0-with-no-branch,
// which would consume the one-time first-observation fast path, and the
// real no-upstream-to-synced transition on the next observation would then
// compare prevValue(0) > 0 and never fire — found via an e2e regression
// test and fixed alongside this predicate.
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
			name:    "first observation, remote-ahead=0 with remote branch fires (push happened pre-poll)",
			loaded:  false,
			prev:    pushTrackerUnsynced,
			status:  &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: "origin/feature/x"},
			wantOn:  true,
			wantWhy: "first-observation sync — pre-existing remote branch in synced state means a push completed before we started watching",
		},
		{
			name:   "first observation, no remote branch does not fire",
			loaded: false,
			prev:   pushTrackerUnsynced,
			// Branch never been pushed — RemoteBranch is empty.
			status: &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: ""},
			wantOn: false,
		},
		{
			name:   "first observation, remote-ahead>0 does not fire (waiting for transition)",
			loaded: false,
			prev:   pushTrackerUnsynced,
			status: &lifecycle.GitStatusData{RemoteAhead: 3, RemoteBranch: "origin/feature/x"},
			wantOn: false,
		},
		{
			name:   "transition >0 to 0 with remote fires (legacy in-session push)",
			loaded: true,
			prev:   2,
			status: &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: true,
		},
		{
			name: "no-upstream-to-synced transition fires (the previously-missed case)",
			// This is the exact bug: a task's first-ever status observation
			// (no commits, no upstream yet) burns the !loaded fast path with
			// a negative result, so the *next* observation — the real push,
			// going from "never had an upstream" to "synced" — must still be
			// recognized as a transition even though it's not a literal
			// N>0-to-0 drop.
			loaded: true,
			prev:   pushTrackerUnsynced,
			status: &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: true,
		},
		{
			name:   "stays at 0 after first fire does not refire",
			loaded: true,
			prev:   0,
			status: &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: false,
		},
		{
			name:   "transition with no remote does not fire (local-only commit was undone)",
			loaded: true,
			prev:   1,
			status: &lifecycle.GitStatusData{RemoteAhead: 0, RemoteBranch: ""},
			wantOn: false,
		},
		{
			name:   "nil status does not fire",
			loaded: true,
			prev:   1,
			status: nil,
			wantOn: false,
		},
		{
			name: "base-branch-ahead is irrelevant: real feature branch, just pushed, still fires",
			// This is the exact production bug: a branch with real commits
			// ahead of main (Ahead > 0, as any normal feature branch has)
			// must still be detected as "just pushed" once RemoteAhead drops
			// to 0 — Ahead staying high must not suppress detection.
			loaded: false,
			prev:   pushTrackerUnsynced,
			status: &lifecycle.GitStatusData{Ahead: 12, RemoteAhead: 0, RemoteBranch: "origin/feature/x"},
			wantOn: true,
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

type fakeModelInfoLookup struct {
	info modelsdev.ModelInfo
	ok   bool
}

func (f fakeModelInfoLookup) LookupModelInfo(context.Context, string) (modelsdev.ModelInfo, bool) {
	return f.info, f.ok
}

func TestResolveContextWindowValues(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{
		Model: "gpt-5.3-codex-spark",
	}))

	base := watcher.ContextWindowData{
		TaskID:        "t1",
		TaskSessionID: "s1",
	}

	t.Run("positive ACP size wins", func(t *testing.T) {
		svc := &Service{
			repo:            repo,
			modelInfoLookup: fakeModelInfoLookup{info: modelsdev.ModelInfo{ContextWindow: 128000}, ok: true},
		}
		size, remaining, efficiency, source, ok := svc.resolveContextWindowValues(ctx, watcher.ContextWindowData{
			TaskID:                 base.TaskID,
			TaskSessionID:          base.TaskSessionID,
			ContextWindowSize:      258400,
			ContextWindowUsed:      100000,
			ContextWindowRemaining: 158400,
			ContextEfficiency:      38.7,
		})

		require.True(t, ok)
		require.Equal(t, int64(258400), size)
		require.Equal(t, int64(158400), remaining)
		require.Equal(t, 38.7, efficiency)
		require.Equal(t, "acp", source)
	})

	t.Run("models.dev fallback supplies missing ACP size", func(t *testing.T) {
		svc := &Service{
			repo:            repo,
			modelInfoLookup: fakeModelInfoLookup{info: modelsdev.ModelInfo{ContextWindow: 128000}, ok: true},
		}
		size, remaining, efficiency, source, ok := svc.resolveContextWindowValues(ctx, watcher.ContextWindowData{
			TaskID:            base.TaskID,
			TaskSessionID:     base.TaskSessionID,
			ContextWindowUsed: 64000,
		})

		require.True(t, ok)
		require.Equal(t, int64(128000), size)
		require.Equal(t, int64(64000), remaining)
		require.Equal(t, 50.0, efficiency)
		require.Equal(t, "api", source)
	})

	t.Run("models.dev fallback uses cached runtime model before session load", func(t *testing.T) {
		svc := &Service{
			modelInfoLookup: fakeModelInfoLookup{info: modelsdev.ModelInfo{ContextWindow: 96000}, ok: true},
		}
		svc.runtimeModelBySession.Store("s1", "gpt-5.3-codex-spark")
		size, remaining, efficiency, source, ok := svc.resolveContextWindowValues(ctx, watcher.ContextWindowData{
			TaskID:            base.TaskID,
			TaskSessionID:     base.TaskSessionID,
			ContextWindowUsed: 24000,
		})

		require.True(t, ok)
		require.Equal(t, int64(96000), size)
		require.Equal(t, int64(72000), remaining)
		require.Equal(t, 25.0, efficiency)
		require.Equal(t, "api", source)
	})

	t.Run("lookup miss hides context window", func(t *testing.T) {
		svc := &Service{repo: repo, modelInfoLookup: fakeModelInfoLookup{}}
		_, _, _, _, ok := svc.resolveContextWindowValues(ctx, watcher.ContextWindowData{
			TaskID:            base.TaskID,
			TaskSessionID:     base.TaskSessionID,
			ContextWindowUsed: 64000,
		})

		require.False(t, ok)
	})
}

func TestContextWindowResetDropsStalePersistence(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "context_window", map[string]interface{}{
		"size": int64(200000), "used": int64(190000),
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	staleGeneration := svc.captureContextWindowGeneration("s1")
	require.NoError(t, svc.clearContextWindowForReset(ctx, "s1"))

	persisted, _, err := svc.persistContextWindowUpdate(ctx, "s1", staleGeneration, map[string]interface{}{
		"size": int64(200000), "used": int64(195000),
	})
	require.NoError(t, err)
	require.False(t, persisted)

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Nil(t, updated.Metadata["context_window"])
}

func TestContextWindowUpdatesPersistInArrivalOrder(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyContextWindow, map[string]interface{}{
		"size": int64(200000), "used": int64(120000),
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleContextWindowUpdated(ctx, watcher.ContextWindowData{
		TaskID:                 "t1",
		TaskSessionID:          "s1",
		ContextWindowSize:      200000,
		ContextWindowUsed:      80000,
		ContextWindowRemaining: 120000,
	})
	svc.handleContextWindowUpdated(ctx, watcher.ContextWindowData{
		TaskID:                 "t1",
		TaskSessionID:          "s1",
		ContextWindowSize:      200000,
		ContextWindowUsed:      120000,
		ContextWindowRemaining: 80000,
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	window, ok := updated.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.True(t, ok)
	require.EqualValues(t, 120000, window["used"])
	require.Equal(t, float64(1), updated.Metadata[models.SessionMetaKeyContextCompactionCount])
}

func TestContextWindowDecreasePersistsCompactionCount(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "context_window", map[string]interface{}{
		"size": int64(200000), "used": int64(120000),
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	generation := svc.captureContextWindowGeneration("s1")
	persisted, _, err := svc.persistContextWindowUpdate(ctx, "s1", generation, map[string]interface{}{
		"size": int64(200000), "used": int64(80000),
	})
	require.NoError(t, err)
	require.True(t, persisted)

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, float64(1), updated.Metadata["context_compaction_count"])
}

func TestContextWindowCompactionCountSurvivesResetAndServiceRestart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyContextWindow, map[string]interface{}{
		"size": int64(200000), "used": int64(120000),
	}))

	first := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	generation := first.captureContextWindowGeneration("s1")
	_, _, err := first.persistContextWindowUpdate(ctx, "s1", generation, map[string]interface{}{
		"size": int64(200000), "used": int64(80000),
	})
	require.NoError(t, err)

	require.NoError(t, first.clearContextWindowForReset(ctx, "s1"))
	cleared, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Nil(t, cleared.Metadata[models.SessionMetaKeyContextWindow])
	require.Equal(t, float64(1), cleared.Metadata[models.SessionMetaKeyContextCompactionCount])

	second := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	secondGeneration := second.captureContextWindowGeneration("s1")
	_, count, err := second.persistContextWindowUpdate(ctx, "s1", secondGeneration, map[string]interface{}{
		"size": int64(200000), "used": int64(40000),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestContextWindowCompactionCountIsPublishedWithWindowUpdate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyContextWindow, map[string]interface{}{
		"size": int64(200000), "used": int64(120000),
	}))
	eventBus := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus
	generation := svc.captureContextWindowGeneration("s1")

	persisted, err := svc.persistAndPublishContextWindowUpdate(ctx, "t1", "s1", generation, map[string]interface{}{
		"size": int64(200000), "used": int64(80000),
	})
	require.NoError(t, err)
	require.True(t, persisted)
	require.Len(t, eventBus.events, 1)

	payload, ok := eventBus.events[0].event.Data.(map[string]interface{})
	require.True(t, ok)
	metadata, ok := payload["metadata"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, int64(1), metadata[models.SessionMetaKeyContextCompactionCount])
	require.NotNil(t, metadata[models.SessionMetaKeyContextWindow])
}

func TestContextWindowResetDropsStaleBroadcast(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eventBus := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	staleGeneration := svc.captureContextWindowGeneration("s1")
	require.NoError(t, svc.clearContextWindowForReset(ctx, "s1"))

	persisted, err := svc.persistAndPublishContextWindowUpdate(ctx, "t1", "s1", staleGeneration, map[string]interface{}{
		"size": int64(200000), "used": int64(195000),
	})
	require.NoError(t, err)
	require.False(t, persisted)
	require.Empty(t, eventBus.events)
}
