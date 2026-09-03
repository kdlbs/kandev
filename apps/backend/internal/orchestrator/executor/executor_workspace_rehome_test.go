package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	api "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-001.4
func TestRetryLaunchAfterMissingWorkspaceRetriesExactlyOnce(t *testing.T) {
	repo := newMockRepository()
	claims := 0
	repo.claimTaskEnvironmentRehomeFunc = func(context.Context, string, string, string, bool) (bool, error) {
		claims++
		return true, nil
	}
	manager := &mockAgentManager{launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		return &LaunchAgentResponse{AgentExecutionID: "replacement", Status: api.AgentStatusStarting}, nil
	}}
	exec := newTestExecutor(t, manager, repo)
	req := &LaunchAgentRequest{TaskID: "task", SessionID: "session", WorkspaceReuseRequired: true, PreviousExecutionID: "old", WorktreeID: "top", Repositories: []RepoSpec{{RepositoryID: "repo-a", WorktreeID: "repo-worktree"}}, Metadata: map[string]interface{}{workspaceMetadataSSHRemoteTaskDir: "/missing"}}
	env := &models.TaskEnvironment{ID: "env", TaskID: "task", Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/missing"}

	resp, err := exec.retryLaunchAfterMissingWorkspace(context.Background(), "task", "session", env, req, &models.MissingTaskWorkspaceError{}, false)
	if err != nil || resp == nil || resp.AgentExecutionID != "replacement" {
		t.Fatalf("retry = %+v, %v", resp, err)
	}
	if claims != 1 || manager.launchAgentCallCount != 1 {
		t.Fatalf("claims=%d launches=%d, want one each", claims, manager.launchAgentCallCount)
	}
	if req.WorkspaceReuseRequired || req.PreviousExecutionID != "" {
		t.Fatalf("replacement request retained reuse identity: %+v", req)
	}
	if req.WorktreeID != "" || req.Repositories[0].WorktreeID != "" {
		t.Fatalf("replacement request retained worktree identity: %+v", req)
	}
	if req.TaskID != "task" || req.SessionID != "session" {
		t.Fatalf("replacement changed task/session identity: %+v", req)
	}
}

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-001.5
func TestRetryLaunchAfterMissingWorkspacePreservesBothErrors(t *testing.T) {
	repo := newMockRepository()
	repo.claimTaskEnvironmentRehomeFunc = func(context.Context, string, string, string, bool) (bool, error) { return true, nil }
	recoveryErr := errors.New("replacement failed")
	manager := &mockAgentManager{launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) { return nil, recoveryErr }}
	exec := newTestExecutor(t, manager, repo)
	original := &models.MissingTaskWorkspaceError{Detail: "missing remote task directory"}

	env := &models.TaskEnvironment{ID: "env", Status: models.TaskEnvironmentStatusReady}
	_, err := exec.retryLaunchAfterMissingWorkspace(context.Background(), "task", "session", env, &LaunchAgentRequest{}, original, false)
	var combined *WorkspaceRehomeError
	if !errors.As(err, &combined) || !errors.Is(err, models.ErrWorkspaceReuseUnsafe) || !errors.Is(combined.Recovery, recoveryErr) {
		t.Fatalf("error = %v, want original and recovery causes", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("replacement launches = %d, want exactly one", manager.launchAgentCallCount)
	}
	if env.Status != models.TaskEnvironmentStatusFailed || env.MaterializationSessionID != "" {
		t.Fatalf("replacement failure left environment non-terminal: %+v", env)
	}
}

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-002.1
func TestRetryLaunchAfterMissingWorkspaceDoesNotLaunchWithoutLossAuthorization(t *testing.T) {
	repo := newMockRepository()
	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)

	_, err := exec.retryLaunchAfterMissingWorkspace(context.Background(), "task", "session", &models.TaskEnvironment{ID: "env"}, &LaunchAgentRequest{}, &models.MissingTaskWorkspaceError{}, false)
	var combined *WorkspaceRehomeError
	if !errors.As(err, &combined) || !errors.Is(combined.Recovery, models.ErrWorkspaceRehomeNeedsAuthorization) {
		t.Fatalf("error = %v, want data-loss authorization warning", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("launches = %d, want zero", manager.launchAgentCallCount)
	}
}
