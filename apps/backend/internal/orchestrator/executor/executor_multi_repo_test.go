package executor

import (
	"context"
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
		Repository:     repo,
	}
	branchInfo := &repoInfo{
		RepositoryID:   "repo-kandev",
		RepositoryPath: "/repos/kandev",
		BaseBranch:     "main",
		CheckoutBranch: "branch-5hn",
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
