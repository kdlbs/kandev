package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
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
				WorktreePath:     "/tasks/x/frontend",
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
