package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestLaunchPreparedSession_SingleRepoWorktree_PersistsSubdirAsWorkspacePath is
// the regression guard for the -32002 Resource not found bug on session/load
// after backend restart for single-repo Worktree tasks. Before the fix,
// computeWorkspacePath collapsed resp.WorktreePath via filepath.Dir and
// wrote the task root as workspace_path, while the hot-start agent cwd was
// the repo subdir. On cold start the persisted (parent) workspace_path
// became the new cwd → SDK sanitized to a different folder and missed the
// agent's jsonl. After the fix, workspace_path == worktree_path (the subdir),
// matching the hot-start cwd.
func TestLaunchPreparedSession_SingleRepoWorktree_PersistsSubdirAsWorkspacePath(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-wt-single-1"
	sessionID := "session-wt-single-1"
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

	const subdir = "/home/u/.kandev/tasks/test-task_abc/repo-only"

	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			// Single-repo Worktree preparer returns wt.Path (subdir with repo
			// folder), executor_standalone.go:147 mirrors it into
			// metadata["worktree_path"], the lifecycle adapter surfaces it as
			// resp.WorktreePath.
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-1",
				WorktreeID:       "wt-1",
				WorktreePath:     subdir,
				WorktreeBranch:   "feat/test",
				PrepareResult: &lifecycle.EnvPrepareResult{
					Success:    true,
					WorktreeID: "wt-1",
				},
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "SingleRepoWorktree"}
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionID, LaunchOptions{
		AgentProfileID: "profile-123",
		StartAgent:     false,
	}); err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}

	if len(repo.taskEnvironments) != 1 {
		t.Fatalf("expected 1 task_environment, got %d", len(repo.taskEnvironments))
	}
	var env *models.TaskEnvironment
	for _, e := range repo.taskEnvironments {
		env = e
	}
	if env.WorktreePath != subdir {
		t.Errorf("worktree_path = %q, want %q", env.WorktreePath, subdir)
	}
	if env.WorkspacePath != subdir {
		t.Errorf("workspace_path = %q, want %q (== worktree_path so process cwd matches between hot and cold start)",
			env.WorkspacePath, subdir)
	}
}
