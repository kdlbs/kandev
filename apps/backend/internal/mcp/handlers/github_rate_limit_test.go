package handlers

import (
	"context"
	"encoding/json"
	"testing"

	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type githubRateLimitServiceFake struct {
	workspaceID string
	snapshot    githubsvc.WorkspaceRateLimitSnapshot
}

func (f *githubRateLimitServiceFake) GetWorkspaceRateLimitSnapshot(
	_ context.Context,
	workspaceID string,
) (githubsvc.WorkspaceRateLimitSnapshot, error) {
	f.workspaceID = workspaceID
	return f.snapshot, nil
}

func TestGetGitHubRateLimitDerivesWorkspaceFromAuthorizedTask(t *testing.T) {
	taskService, repository := newTestTaskService(t)
	_, task, _ := seedTaskWithSession(
		t, taskService, repository, models.TaskSessionStateWaitingForInput,
	)
	fake := &githubRateLimitServiceFake{snapshot: githubsvc.WorkspaceRateLimitSnapshot{
		WorkspaceID: task.WorkspaceID,
		Core: githubsvc.RateLimitBucketSnapshot{
			Resource: githubsvc.ResourceCore, Known: true, Fresh: true,
			Limit: 5000, Remaining: 5000,
		},
		InteractiveAllowed: true,
		BackgroundAllowed:  true,
	}}
	handler := NewHandlers(
		taskService, nil, nil, nil, nil, repository, repository,
		nil, nil, nil, nil, nil, testLogger(t),
	)
	handler.SetGitHubRateLimitService(fake)
	message := makeWSMessage(t, ws.ActionMCPGetGitHubRateLimit, map[string]interface{}{
		"task_id": task.ID,
		// A caller cannot choose a workspace independently of its task.
		"workspace_id": "foreign-workspace",
	})

	response, err := handler.handleGetGitHubRateLimit(context.Background(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, task.WorkspaceID, fake.workspaceID)
	var snapshot githubsvc.WorkspaceRateLimitSnapshot
	require.NoError(t, json.Unmarshal(response.Payload, &snapshot))
	require.Equal(t, task.WorkspaceID, snapshot.WorkspaceID)
}

func TestGetGitHubRateLimitRejectsMissingTask(t *testing.T) {
	taskService, repository := newTestTaskService(t)
	handler := NewHandlers(
		taskService, nil, nil, nil, nil, repository, repository,
		nil, nil, nil, nil, nil, testLogger(t),
	)
	handler.SetGitHubRateLimitService(&githubRateLimitServiceFake{})
	message := makeWSMessage(t, ws.ActionMCPGetGitHubRateLimit, map[string]interface{}{})

	response, err := handler.handleGetGitHubRateLimit(context.Background(), message)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
}
