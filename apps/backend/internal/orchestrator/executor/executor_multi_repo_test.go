package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedMultiRepoTask wires the mock repository with two repositories linked to
// taskID, returning the captured launch request after LaunchPreparedSession.
func seedMultiRepoTask(t *testing.T, repo *mockRepository, taskID string) {
	t.Helper()
	repo.repositories["repo-front"] = &models.Repository{
		ID:                   "repo-front",
		Name:                 "frontend",
		LocalPath:            "/repos/frontend",
		WorktreeBranchPrefix: "feat/",
	}
	repo.repositories["repo-back"] = &models.Repository{
		ID:                   "repo-back",
		Name:                 "backend",
		LocalPath:            "/repos/backend",
		WorktreeBranchPrefix: "feat/",
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.taskRepositories["tr-2"] = &models.TaskRepository{
		ID: "tr-2", TaskID: taskID, RepositoryID: "repo-back", Position: 1, BaseBranch: "main",
	}
}

func seedWorktreeExecutor(repo *mockRepository) {
	repo.executors[models.ExecutorIDWorktree] = &models.Executor{
		ID:        models.ExecutorIDWorktree,
		Type:      models.ExecutorTypeWorktree,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}
}

func TestLaunchPreparedSession_MultiRepo_PopulatesRequestRepositories(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-multi-1"
	sessionID := "session-multi-1"
	seedMultiRepoTask(t, repo, taskID)

	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	var captured *LaunchAgentRequest
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			captured = req
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-1",
				// Multi-repo: top-level WorktreePath mirrors agentctl's WorkDir
				// (= task root), per executor_standalone.go:147 + lifecycle adapter.
				WorktreePath: "/tasks/x",
				Worktrees: []RepoWorktreeResult{
					{RepositoryID: "repo-front", WorktreeID: "wt-front", WorktreeBranch: "feat/x-1", WorktreePath: "/tasks/x/frontend"},
					{RepositoryID: "repo-back", WorktreeID: "wt-back", WorktreeBranch: "feat/x-2", WorktreePath: "/tasks/x/backend"},
				},
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi"}
	_, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}

	if captured == nil {
		t.Fatal("expected launch agent to be called")
	}
	if len(captured.Repositories) != 2 {
		t.Fatalf("expected req.Repositories length 2, got %d", len(captured.Repositories))
	}
	if captured.Repositories[0].RepositoryID != "repo-front" || captured.Repositories[1].RepositoryID != "repo-back" {
		t.Errorf("unexpected repo order: %+v", captured.Repositories)
	}
	// Legacy single-repo top-level fields stay populated from the primary.
	if captured.RepositoryPath != "/repos/frontend" {
		t.Errorf("expected primary repo path on top-level field, got %q", captured.RepositoryPath)
	}
}

func TestLaunchPreparedSession_MultiRepo_LaunchFailureReportsFailingSecondaryRepositoryID(t *testing.T) {
	repo := newMockRepository()
	const taskID = "task-multi-launch-failure"
	const sessionID = "session-multi-launch-failure"
	seedMultiRepoTask(t, repo, taskID)
	repo.taskRepositories["tr-2"].CheckoutBranch = "feature/foo"
	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	launchErr := errors.New("fatal: couldn't find remote ref feature/foo")
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	var callbackRepositoryID string
	exec.SetOnLaunchFailed(func(_ context.Context, callbackTaskID, callbackSessionID, repositoryID string, callbackErr error) {
		if callbackTaskID != taskID || callbackSessionID != sessionID {
			t.Errorf("unexpected callback target: task=%q session=%q", callbackTaskID, callbackSessionID)
		}
		if !errors.Is(callbackErr, launchErr) {
			t.Errorf("unexpected callback error: %v", callbackErr)
		}
		callbackRepositoryID = repositoryID
	})

	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{
		ID: taskID, WorkspaceID: "ws-1", Title: "Multi",
	}, sessionID, LaunchOptions{AgentProfileID: "profile-123", StartAgent: false})
	if !errors.Is(err, launchErr) {
		t.Fatalf("LaunchPreparedSession error = %v, want %v", err, launchErr)
	}
	if callbackRepositoryID != "repo-back" {
		t.Fatalf("launch failure repository ID = %q, want failing secondary repository %q", callbackRepositoryID, "repo-back")
	}
}

func TestFailingLaunchRepositoryID_UsesExactBranchToken(t *testing.T) {
	req := &LaunchAgentRequest{Repositories: []RepoSpec{{
		RepositoryID:   "repo-secondary",
		CheckoutBranch: "feature/foo",
	}}}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "remote ref exact match",
			err:  errors.New("fatal: couldn't find remote ref feature/foo"),
			want: "repo-secondary",
		},
		{
			name: "quoted branch exact match",
			err:  errors.New("branch \"feature/foo\" not found locally or on remote"),
			want: "repo-secondary",
		},
		{
			name: "pathspec exact match",
			err:  errors.New("error: pathspec 'feature/foo' did not match any file(s) known to git"),
			want: "repo-secondary",
		},
		{
			name: "remote ref prefix collision",
			err:  errors.New("fatal: couldn't find remote ref feature/foo-deleted"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failingLaunchRepositoryID(req, tt.err); got != tt.want {
				t.Fatalf("failingLaunchRepositoryID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLaunchPreparedSession_MultiRepo_PersistsPerRepoEnvironmentAndWorktreeRows(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-multi-2"
	sessionID := "session-multi-2"
	seedMultiRepoTask(t, repo, taskID)

	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-2",
				WorktreeID:       "wt-front", // legacy mirror of Worktrees[0]
				// Multi-repo: top-level WorktreePath = task root, matching how
				// executor_standalone.go:147 writes metadata["worktree_path"] from
				// req.WorkspacePath (set to task root by the multi-repo preparer).
				WorktreePath: "/tasks/x",
				Worktrees: []RepoWorktreeResult{
					{RepositoryID: "repo-front", WorktreeID: "wt-front", WorktreeBranch: "feat/x-1", WorktreePath: "/tasks/x/frontend"},
					{RepositoryID: "repo-back", WorktreeID: "wt-back", WorktreeBranch: "feat/x-2", WorktreePath: "/tasks/x/backend"},
				},
				PrepareResult: &lifecycle.EnvPrepareResult{
					Success: true,
					Worktrees: []lifecycle.RepoWorktreeResult{
						{RepositoryID: "repo-front", WorktreeID: "wt-front"},
						{RepositoryID: "repo-back", WorktreeID: "wt-back"},
					},
				},
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi"}
	_, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}

	// One TaskEnvironment row + 2 TaskEnvironmentRepo rows.
	if len(repo.taskEnvironments) != 1 {
		t.Fatalf("expected 1 task_environment, got %d", len(repo.taskEnvironments))
	}
	var envID string
	for id := range repo.taskEnvironments {
		envID = id
	}
	if got := len(repo.taskEnvironmentRepos[envID]); got != 2 {
		t.Errorf("expected 2 task_environment_repos, got %d", got)
	}

	// Two TaskSessionWorktree rows, one per repo.
	if len(repo.sessionWorktrees) != 2 {
		t.Fatalf("expected 2 session_worktree rows, got %d", len(repo.sessionWorktrees))
	}
	repoIDsSeen := map[string]bool{}
	for _, w := range repo.sessionWorktrees {
		repoIDsSeen[w.RepositoryID] = true
	}
	if !repoIDsSeen["repo-front"] || !repoIDsSeen["repo-back"] {
		t.Errorf("expected both repo IDs persisted; got %v", repoIDsSeen)
	}
}

func TestLaunchPreparedSession_MultiRepo_ReusesPerRepoWorktreeIDsFromEnvironment(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-multi-reuse"
	sessionID := "session-multi-reuse"
	seedMultiRepoTask(t, repo, taskID)
	seedWorktreeExecutor(repo)

	repo.taskEnvironments["env-existing"] = &models.TaskEnvironment{
		ID:           "env-existing",
		TaskID:       taskID,
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusReady,
		WorktreeID:   "wt-front",
		Repos: []*models.TaskEnvironmentRepo{
			{
				TaskEnvironmentID: "env-existing",
				RepositoryID:      "repo-front",
				WorktreeID:        "wt-front",
				Position:          0,
			},
			{
				TaskEnvironmentID: "env-existing",
				RepositoryID:      "repo-back",
				WorktreeID:        "wt-back",
				Position:          1,
			},
		},
	}
	repo.taskEnvironmentRepos["env-existing"] = repo.taskEnvironments["env-existing"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		ExecutorID:     models.ExecutorIDWorktree,
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	var captured *LaunchAgentRequest
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			captured = req
			return &LaunchAgentResponse{AgentExecutionID: "exec-reuse"}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi"}
	_, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		ExecutorID:     models.ExecutorIDWorktree,
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}

	if captured == nil {
		t.Fatal("expected launch request to be captured")
	}
	if len(captured.Repositories) != 2 {
		t.Fatalf("expected 2 repo specs, got %d", len(captured.Repositories))
	}
	if captured.Repositories[0].WorktreeID != "wt-front" {
		t.Errorf("front WorktreeID = %q, want wt-front", captured.Repositories[0].WorktreeID)
	}
	if captured.Repositories[1].WorktreeID != "wt-back" {
		t.Errorf("back WorktreeID = %q, want wt-back", captured.Repositories[1].WorktreeID)
	}
}

func TestReuseExistingRepositoryWorktrees_EnvironmentRowsWinOverSessionRows(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-env-wins"
	envID := "env-env-wins"
	now := time.Now().UTC()
	repo.sessions["session-prev"] = &models.TaskSession{
		ID:                "session-prev",
		TaskID:            taskID,
		TaskEnvironmentID: envID,
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.sessionWorktrees = append(repo.sessionWorktrees,
		&models.TaskSessionWorktree{
			SessionID:    "session-prev",
			RepositoryID: "repo-a",
			BranchSlug:   "main",
			WorktreeID:   "wt-session-a",
		},
		&models.TaskSessionWorktree{
			SessionID:    "session-prev",
			RepositoryID: "repo-b",
			BranchSlug:   "feature",
			WorktreeID:   "wt-session-b",
		},
	)
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:      taskID,
		UseWorktree: true,
		Repositories: []RepoSpec{
			{RepositoryID: "repo-a", BranchIdentitySlug: "main"},
			{RepositoryID: "repo-b", BranchIdentitySlug: "feature"},
		},
	}
	env := &models.TaskEnvironment{
		ID: envID,
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", BranchSlug: "main", WorktreeID: "wt-env-a"},
		},
	}

	exec.reuseExistingRepositoryWorktrees(context.Background(), req, env)

	if req.Repositories[0].WorktreeID != "wt-env-a" {
		t.Fatalf("repo-a WorktreeID = %q, want env row to win with wt-env-a", req.Repositories[0].WorktreeID)
	}
	if req.Repositories[1].WorktreeID != "wt-session-b" {
		t.Fatalf("repo-b WorktreeID = %q, want session fallback wt-session-b", req.Repositories[1].WorktreeID)
	}
	if req.WorktreeID != "wt-env-a" {
		t.Fatalf("top-level WorktreeID = %q, want first repo env worktree wt-env-a", req.WorktreeID)
	}
}

func TestReuseExistingRepositoryWorktrees_LegacyFlatEnvWorktreeFeedsFlatBranchSpec(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:       "task-legacy-flat",
		RepositoryID: "repo-kandev",
		UseWorktree:  true,
		Repositories: []RepoSpec{
			{
				RepositoryID:       "repo-kandev",
				BranchIdentitySlug: "release-1-2",
				BranchSlug:         "",
			},
			{
				RepositoryID:       "repo-kandev",
				BranchIdentitySlug: "main",
				BranchSlug:         "main",
			},
		},
	}
	env := &models.TaskEnvironment{
		ID:           "env-legacy-flat",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-legacy-flat",
	}

	exec.reuseExistingRepositoryWorktrees(context.Background(), req, env)

	if req.Repositories[0].WorktreeID != "wt-legacy-flat" {
		t.Fatalf("flat repo WorktreeID = %q, want legacy top-level wt-legacy-flat", req.Repositories[0].WorktreeID)
	}
	if req.Repositories[1].WorktreeID != "" {
		t.Fatalf("nested repo WorktreeID = %q, want empty", req.Repositories[1].WorktreeID)
	}
	if req.WorktreeID != "wt-legacy-flat" {
		t.Fatalf("top-level WorktreeID = %q, want wt-legacy-flat", req.WorktreeID)
	}
}

func TestReuseExistingRepositoryWorktrees_LegacyEmptyBranchRowFeedsFlatBranchSpec(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:      "task-legacy-empty-row",
		UseWorktree: true,
		Repositories: []RepoSpec{
			{
				RepositoryID:       "repo-kandev",
				BranchIdentitySlug: "main",
				BranchSlug:         "",
			},
			{
				RepositoryID:       "repo-kandev",
				BranchIdentitySlug: "feature-x",
				BranchSlug:         "feature-x",
			},
		},
	}
	env := &models.TaskEnvironment{
		ID: "env-legacy-empty-row",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "", WorktreeID: "wt-flat-row"},
		},
	}

	exec.reuseExistingRepositoryWorktrees(context.Background(), req, env)

	if req.Repositories[0].WorktreeID != "wt-flat-row" {
		t.Fatalf("flat repo WorktreeID = %q, want legacy row wt-flat-row", req.Repositories[0].WorktreeID)
	}
	if req.Repositories[1].WorktreeID != "" {
		t.Fatalf("nested repo WorktreeID = %q, want empty", req.Repositories[1].WorktreeID)
	}
}

func TestReuseExistingEnvironment_SingleRepoUsesBranchScopedEnvRow(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:       "task-single-branch-row",
		RepositoryID: "repo-kandev",
		BaseBranch:   "feature-x",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ID:           "env-single-branch-row",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-main",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
			{RepositoryID: "repo-kandev", BranchSlug: "feature-x", WorktreeID: "wt-feature"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-feature" {
		t.Fatalf("top-level WorktreeID = %q, want branch-scoped wt-feature", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_SingleRepoBranchMatchKeepsScopedRepoSpec(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:         "task-single-branch-row",
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		RepoName:       "kandev",
		BaseBranch:     "feature-x",
		UseWorktree:    true,
	}
	env := &models.TaskEnvironment{
		ID:           "env-single-branch-row",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-main",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
			{RepositoryID: "repo-kandev", BranchSlug: "feature-x", WorktreeID: "wt-feature"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-feature" {
		t.Fatalf("top-level WorktreeID = %q, want branch-scoped wt-feature", req.WorktreeID)
	}
	if len(req.Repositories) != 1 {
		t.Fatalf("Repositories length = %d, want one branch-scoped reuse spec", len(req.Repositories))
	}
	got := req.Repositories[0]
	if got.WorktreeID != "wt-feature" {
		t.Fatalf("spec WorktreeID = %q, want wt-feature", got.WorktreeID)
	}
	if got.BranchIdentitySlug != "feature-x" {
		t.Fatalf("spec BranchIdentitySlug = %q, want feature-x", got.BranchIdentitySlug)
	}
	if req.BranchIdentitySlug != "feature-x" {
		t.Fatalf("top-level BranchIdentitySlug = %q, want feature-x", req.BranchIdentitySlug)
	}
	if got.BranchSlug != "" {
		t.Fatalf("spec BranchSlug = %q, want empty to preserve the existing path", got.BranchSlug)
	}
	if req.BranchSlug != "" {
		t.Fatalf("top-level BranchSlug = %q, want empty to preserve the existing path", req.BranchSlug)
	}
}

func TestReuseExistingEnvironment_SingleRepoUsesDefaultBranchScopedEnvRow(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:        "task-single-default-branch-row",
		RepositoryID:  "repo-kandev",
		DefaultBranch: "main",
		UseWorktree:   true,
	}
	env := &models.TaskEnvironment{
		ID:           "env-single-default-branch-row",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-feature",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-main" {
		t.Fatalf("top-level WorktreeID = %q, want default-branch wt-main", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_SingleRepoDoesNotFallBackToWrongScopedEnvWorktree(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:       "task-single-no-match",
		RepositoryID: "repo-kandev",
		BaseBranch:   "feature-x",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ID:           "env-single-no-match",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-main",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Fatalf("top-level WorktreeID = %q, want no stale env-level fallback", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_SingleRepoUnmatchedScopedEnvUsesBranchPathSlug(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:               "task-single-new-scoped-branch",
		RepositoryID:         "repo-kandev",
		RepositoryPath:       "/repos/kandev",
		RepoName:             "kandev",
		BaseBranch:           "feature/new-path",
		DefaultBranch:        "main",
		WorktreeBranchPrefix: "task/",
		UseWorktree:          true,
	}
	env := &models.TaskEnvironment{
		ID:           "env-single-new-scoped-branch",
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-main",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Fatalf("top-level WorktreeID = %q, want no stale env-level fallback", req.WorktreeID)
	}
	if len(req.Repositories) != 1 {
		t.Fatalf("Repositories length = %d, want one branch-scoped prepare spec", len(req.Repositories))
	}
	got := req.Repositories[0]
	if got.BranchSlug != "feature-new-path" {
		t.Fatalf("BranchSlug = %q, want feature-new-path", got.BranchSlug)
	}
	if got.BranchIdentitySlug != "feature-new-path" {
		t.Fatalf("BranchIdentitySlug = %q, want feature-new-path", got.BranchIdentitySlug)
	}
	if req.BranchSlug != "feature-new-path" {
		t.Fatalf("top-level BranchSlug = %q, want feature-new-path", req.BranchSlug)
	}
	if req.BranchIdentitySlug != "feature-new-path" {
		t.Fatalf("top-level BranchIdentitySlug = %q, want feature-new-path", req.BranchIdentitySlug)
	}
	if got.RepositoryPath != req.RepositoryPath || got.RepoName != req.RepoName {
		t.Fatalf("repo fields not carried into scoped spec: %+v", got)
	}
	if got.WorktreeBranchPrefix != req.WorktreeBranchPrefix || got.DefaultBranch != req.DefaultBranch {
		t.Fatalf("branch defaults not carried into scoped spec: %+v", got)
	}
}

func TestReuseExistingEnvironment_SingleRepoIgnoresEmptySessionBranchWhenEnvIsScoped(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-single-empty-session-branch"
	envID := "env-single-empty-session-branch"
	now := time.Now().UTC()
	repo.sessions["session-prev"] = &models.TaskSession{
		ID:                "session-prev",
		TaskID:            taskID,
		TaskEnvironmentID: envID,
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.sessionWorktrees = append(repo.sessionWorktrees, &models.TaskSessionWorktree{
		SessionID:    "session-prev",
		RepositoryID: "repo-kandev",
		BranchSlug:   "",
		WorktreeID:   "wt-stale-empty-branch",
	})
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:       taskID,
		RepositoryID: "repo-kandev",
		BaseBranch:   "feature-x",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ID:           envID,
		RepositoryID: "repo-kandev",
		WorktreeID:   "wt-main",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-main"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Fatalf("top-level WorktreeID = %q, want no empty-branch session fallback", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_SingleRepoIgnoresEmptySessionBranchForNewBranch(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-single-empty-session-new-branch"
	envID := "env-single-empty-session-new-branch"
	now := time.Now().UTC()
	repo.sessions["session-prev"] = &models.TaskSession{
		ID:                "session-prev",
		TaskID:            taskID,
		TaskEnvironmentID: envID,
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.sessionWorktrees = append(repo.sessionWorktrees, &models.TaskSessionWorktree{
		SessionID:    "session-prev",
		RepositoryID: "repo-kandev",
		BranchSlug:   "",
		WorktreeID:   "wt-stale-empty-branch",
	})
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:       taskID,
		RepositoryID: "repo-kandev",
		BaseBranch:   "newbranch",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ID:           envID,
		RepositoryID: "repo-kandev",
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Fatalf("top-level WorktreeID = %q, want no empty-branch session fallback", req.WorktreeID)
	}
}

func TestLaunchPreparedSession_MultiBranch_ReusesWorktreeIDsByBranchSlug(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-multi-branch-reuse"
	sessionID := "session-multi-branch-reuse"
	sourceSessionID := "session-source"
	now := time.Now().UTC()
	seedWorktreeExecutor(repo)

	repo.repositories["repo-kandev"] = &models.Repository{
		ID:                   "repo-kandev",
		Name:                 "kandev",
		LocalPath:            "/repos/kandev",
		WorktreeBranchPrefix: "feature/",
	}
	repo.taskRepositories["tr-main"] = &models.TaskRepository{
		ID: "tr-main", TaskID: taskID, RepositoryID: "repo-kandev", Position: 0, BaseBranch: "main",
	}
	repo.taskRepositories["tr-branch"] = &models.TaskRepository{
		ID: "tr-branch", TaskID: taskID, RepositoryID: "repo-kandev", Position: 1, BaseBranch: "main", CheckoutBranch: "branch-5hn",
	}
	repo.taskEnvironments["env-existing"] = &models.TaskEnvironment{
		ID:           "env-existing",
		TaskID:       taskID,
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusReady,
		WorktreeID:   "wt-main",
	}
	repo.sessions[sourceSessionID] = &models.TaskSession{
		ID:                sourceSessionID,
		TaskID:            taskID,
		TaskEnvironmentID: "env-existing",
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		ExecutorID:     models.ExecutorIDWorktree,
		State:          models.TaskSessionStateCreated,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	repo.sessionWorktrees = append(repo.sessionWorktrees,
		&models.TaskSessionWorktree{
			SessionID:      sourceSessionID,
			RepositoryID:   "repo-kandev",
			WorktreeID:     "wt-main",
			BranchSlug:     "main",
			WorktreePath:   "/tasks/t/kandev",
			WorktreeBranch: "feature/t",
		},
		&models.TaskSessionWorktree{
			SessionID:      sourceSessionID,
			RepositoryID:   "repo-kandev",
			WorktreeID:     "wt-branch",
			BranchSlug:     "branch-5hn",
			WorktreePath:   "/tasks/t/kandev-branch-5hn",
			WorktreeBranch: "branch-5hn",
		},
	)

	var captured *LaunchAgentRequest
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			captured = req
			return &LaunchAgentResponse{AgentExecutionID: "exec-reuse-branch"}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi Branch"}
	_, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		ExecutorID:     models.ExecutorIDWorktree,
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}

	if captured == nil {
		t.Fatal("expected launch request to be captured")
	}
	if len(captured.Repositories) != 2 {
		t.Fatalf("expected 2 repo specs, got %d", len(captured.Repositories))
	}
	if captured.Repositories[0].WorktreeID != "wt-main" {
		t.Errorf("main WorktreeID = %q, want wt-main", captured.Repositories[0].WorktreeID)
	}
	if captured.Repositories[0].BranchIdentitySlug != "main" {
		t.Errorf("main BranchIdentitySlug = %q, want main", captured.Repositories[0].BranchIdentitySlug)
	}
	if captured.Repositories[0].BranchSlug != "" {
		t.Errorf("main BranchSlug = %q, want empty flat-path slug", captured.Repositories[0].BranchSlug)
	}
	if captured.Repositories[1].BranchSlug != "branch-5hn" {
		t.Errorf("branch spec BranchSlug = %q, want branch-5hn", captured.Repositories[1].BranchSlug)
	}
	if captured.Repositories[1].BranchIdentitySlug != "branch-5hn" {
		t.Errorf("branch spec BranchIdentitySlug = %q, want branch-5hn", captured.Repositories[1].BranchIdentitySlug)
	}
	if captured.Repositories[1].WorktreeID != "wt-branch" {
		t.Errorf("branch WorktreeID = %q, want wt-branch", captured.Repositories[1].WorktreeID)
	}
	if captured.WorktreeID != "wt-main" {
		t.Errorf("top-level WorktreeID = %q, want wt-main", captured.WorktreeID)
	}
}

func TestBuildRepoSpecs_MultiBranchIdentityStableAcrossReorder(t *testing.T) {
	repo := &models.Repository{ID: "repo-kandev", Name: "kandev", DefaultBranch: "main"}
	mainInfo := &repoInfo{
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		BaseBranch:     "main",
		Position:       0,
		Repository:     repo,
	}
	branchInfo := &repoInfo{
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		BaseBranch:     "main",
		CheckoutBranch: "branch-5hn",
		Position:       1,
		Repository:     repo,
	}

	first := buildRepoSpecs([]*repoInfo{mainInfo, branchInfo})
	second := buildRepoSpecs([]*repoInfo{branchInfo, mainInfo})

	if first[0].BranchIdentitySlug != "main" || first[0].BranchSlug != "" {
		t.Fatalf("first main plan = identity %q path %q, want main/empty", first[0].BranchIdentitySlug, first[0].BranchSlug)
	}
	if first[1].BranchIdentitySlug != "branch-5hn" || first[1].BranchSlug != "branch-5hn" {
		t.Fatalf("first branch plan = identity %q path %q, want branch-5hn/branch-5hn", first[1].BranchIdentitySlug, first[1].BranchSlug)
	}
	if second[0].BranchIdentitySlug != "branch-5hn" || second[0].BranchSlug != "branch-5hn" {
		t.Fatalf("reordered branch plan = identity %q path %q, want branch-5hn/branch-5hn", second[0].BranchIdentitySlug, second[0].BranchSlug)
	}
	if second[1].BranchIdentitySlug != "main" || second[1].BranchSlug != "" {
		t.Fatalf("reordered main plan = identity %q path %q, want main/empty", second[1].BranchIdentitySlug, second[1].BranchSlug)
	}
}

func TestBuildRepoSpecs_MultiBranchFlatPathFollowsLowestTaskRepoPosition(t *testing.T) {
	repo := &models.Repository{ID: "repo-kandev", Name: "kandev", DefaultBranch: "main"}
	releaseInfo := &repoInfo{
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		BaseBranch:     "release/1.2",
		Position:       0,
		Repository:     repo,
	}
	mainInfo := &repoInfo{
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		BaseBranch:     "main",
		Position:       1,
		Repository:     repo,
	}

	specs := buildRepoSpecs([]*repoInfo{mainInfo, releaseInfo})

	if specs[0].BranchIdentitySlug != "main" || specs[0].BranchSlug != "main" {
		t.Fatalf("main plan = identity %q path %q, want main/main", specs[0].BranchIdentitySlug, specs[0].BranchSlug)
	}
	if specs[1].BranchIdentitySlug != "release-1.2" || specs[1].BranchSlug != "" {
		t.Fatalf("release plan = identity %q path %q, want release-1.2/empty", specs[1].BranchIdentitySlug, specs[1].BranchSlug)
	}
}

func TestBuildRepoSpecs_AssignsBranchIdentityForDistinctRepos(t *testing.T) {
	frontendInfo := &repoInfo{
		RepositoryID:   "repo-front",
		RepositoryPath: "/repos/frontend",
		BaseBranch:     "main",
		Position:       0,
		Repository:     &models.Repository{ID: "repo-front", Name: "frontend", DefaultBranch: "main"},
	}
	backendInfo := &repoInfo{
		RepositoryID:   "repo-back",
		RepositoryPath: "/repos/backend",
		BaseBranch:     "release/1.2",
		Position:       1,
		Repository:     &models.Repository{ID: "repo-back", Name: "backend", DefaultBranch: "main"},
	}

	specs := buildRepoSpecs([]*repoInfo{frontendInfo, backendInfo})

	if specs[0].BranchIdentitySlug != "main" || specs[0].BranchSlug != "" {
		t.Fatalf("frontend plan = identity %q path %q, want main/empty", specs[0].BranchIdentitySlug, specs[0].BranchSlug)
	}
	if specs[1].BranchIdentitySlug != "release-1.2" || specs[1].BranchSlug != "" {
		t.Fatalf("backend plan = identity %q path %q, want release-1.2/empty", specs[1].BranchIdentitySlug, specs[1].BranchSlug)
	}
}

func TestPersistTaskEnvironmentRepos_RefreshesExistingRows(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.taskEnvironmentRepos["env-refresh"] = []*models.TaskEnvironmentRepo{
		{
			ID:                "env-refresh-repo-main",
			TaskEnvironmentID: "env-refresh",
			RepositoryID:      "repo-kandev",
			BranchSlug:        "main",
			WorktreeID:        "wt-stale",
			WorktreePath:      "/old/path",
			WorktreeBranch:    "old-branch",
			Position:          5,
			ErrorMessage:      "old error",
		},
	}

	exec.persistTaskEnvironmentRepos(context.Background(), "env-refresh", []RepoWorktreeResult{
		{
			RepositoryID:   "repo-kandev",
			BranchSlug:     "main",
			WorktreeID:     "wt-repaired",
			WorktreePath:   "/new/path",
			WorktreeBranch: "new-branch",
			ErrorMessage:   "",
		},
	})

	rows := repo.taskEnvironmentRepos["env-refresh"]
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.WorktreeID != "wt-repaired" || row.WorktreePath != "/new/path" || row.WorktreeBranch != "new-branch" || row.Position != 0 || row.ErrorMessage != "" {
		t.Fatalf("row was not refreshed: %+v", row)
	}
}

func TestPersistTaskEnvironmentRepos_MigratesLegacyFlatRowToBranchIdentity(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.taskEnvironmentRepos["env-legacy"] = []*models.TaskEnvironmentRepo{
		{
			ID:                "env-legacy-repo-flat",
			TaskEnvironmentID: "env-legacy",
			RepositoryID:      "repo-kandev",
			BranchSlug:        "",
			WorktreeID:        "wt-flat",
			WorktreePath:      "/old/flat",
		},
	}

	exec.persistTaskEnvironmentRepos(context.Background(), "env-legacy", []RepoWorktreeResult{
		{
			RepositoryID:   "repo-kandev",
			BranchSlug:     "release-1.2",
			WorktreeID:     "wt-flat",
			WorktreePath:   "/new/flat",
			WorktreeBranch: "feature/task",
		},
	})

	rows := repo.taskEnvironmentRepos["env-legacy"]
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.BranchSlug != "release-1.2" || row.WorktreeID != "wt-flat" || row.WorktreePath != "/new/flat" || row.WorktreeBranch != "feature/task" {
		t.Fatalf("legacy row was not migrated: %+v", row)
	}
}

func TestMockRepositoryTaskEnvironmentRepoUpdatesAreBranchScoped(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "env-branches",
		RepositoryID:      "repo-kandev",
		BranchSlug:        "main",
		WorktreeID:        "wt-main",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo(main): %v", err)
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "env-branches",
		RepositoryID:      "repo-kandev",
		BranchSlug:        "feature",
		WorktreeID:        "wt-feature",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo(feature): %v", err)
	}
	rows := repo.taskEnvironmentRepos["env-branches"]
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].ID == rows[1].ID {
		t.Fatalf("branch-specific rows should have distinct IDs, both got %q", rows[0].ID)
	}
	featureRowID := rows[1].ID

	if err := repo.UpdateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                featureRowID,
		TaskEnvironmentID: "env-branches",
		RepositoryID:      "repo-kandev",
		BranchSlug:        "feature",
		WorktreeID:        "wt-feature-updated",
	}); err != nil {
		t.Fatalf("UpdateTaskEnvironmentRepo(feature): %v", err)
	}

	rows = repo.taskEnvironmentRepos["env-branches"]
	if rows[0].WorktreeID != "wt-main" {
		t.Fatalf("main WorktreeID = %q, want wt-main", rows[0].WorktreeID)
	}
	if rows[1].WorktreeID != "wt-feature-updated" {
		t.Fatalf("feature WorktreeID = %q, want wt-feature-updated", rows[1].WorktreeID)
	}
}

func TestLaunchPreparedSession_SingleRepo_DoesNotPopulateRequestRepositories(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-single-1"
	sessionID := "session-single-1"
	repo.repositories["repo-only"] = &models.Repository{
		ID: "repo-only", Name: "only", LocalPath: "/repos/only", WorktreeBranchPrefix: "feat/",
	}
	repo.taskRepositories["tr-only"] = &models.TaskRepository{
		ID: "tr-only", TaskID: taskID, RepositoryID: "repo-only", Position: 0, BaseBranch: "main",
	}
	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	var captured *LaunchAgentRequest
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			captured = req
			return &LaunchAgentResponse{AgentExecutionID: "exec-3"}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1"}
	_, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if len(captured.Repositories) != 0 {
		t.Errorf("single-repo launch should not populate Repositories list; got %d entries", len(captured.Repositories))
	}
	if captured.RepositoryPath != "/repos/only" {
		t.Errorf("expected legacy RepositoryPath populated; got %q", captured.RepositoryPath)
	}
}
