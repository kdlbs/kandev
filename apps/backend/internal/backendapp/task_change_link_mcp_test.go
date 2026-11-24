package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
func TestTaskChangeLinkMCPSingleUser(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
	provider := &fakeGitHubChangeLinks{}
	coordinator := taskChangeLinkCoordinator{
		tasks: taskSvc, github: provider,
		singleUserIdentity: taskChangeLinkIdentityResolver(newDisabledAuthService(t)),
	}
	log := newTestLogger()
	resolver := scope.NewResolver(repos, nil, func() bool { return false }, log)
	ctx, err := resolver.Scope(context.Background(), "task-1")
	require.NoError(t, err)
	_, hasIdentity := authn.IdentityFromContext(ctx)
	require.False(t, hasIdentity)
	ctx = scope.WithPrincipal(ctx, scope.Principal{
		WorkspaceID: "ws-1", CallerTaskID: "task-1", CallerSessionID: "session-1",
	})
	response := dispatchTaskChangeLink(t, coordinator, ctx, map[string]any{
		"task_id": "task-1", "caller_task_id": "task-1", "provider": "github",
		"repository_id": "repo-1", "number": 42,
	})
	require.Equal(t, ws.MessageTypeResponse, response.Type, "payload: %s", response.Payload)
	var result struct {
		TaskID string                       `json:"task_id"`
		Links  []mcphandlers.TaskChangeLink `json:"links"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &result))
	require.Equal(t, "task-1", result.TaskID)
	require.Equal(t, []mcphandlers.TaskChangeLink{{Provider: "github", RepositoryID: "repo-1", Number: 42}}, result.Links)
	require.Equal(t, "ws-1", provider.linkedWorkspaceID)
	require.Equal(t, userstore.DefaultUserID, provider.linkedUserID)
}

func dispatchTaskChangeLink(t *testing.T, coordinator taskChangeLinkCoordinator, ctx context.Context, payload map[string]any) *ws.Message {
	t.Helper()
	h := mcphandlers.NewHandlers(coordinator.tasks, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, newTestLogger())
	h.SetTaskChangeLinkService(coordinator)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	msg, err := ws.NewRequest("link-1", ws.ActionMCPLinkTaskPR, payload)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1, AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.4
func TestManageTaskChangeRequestLinkRepositoryIDValidation(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
	now := time.Now().UTC()
	require.NoError(t, repos.CreateRepository(context.Background(), &models.Repository{
		ID: "repo-unattached", WorkspaceID: "ws-1", Name: "other",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "other",
		CreatedAt: now, UpdatedAt: now,
	}))
	provider := &fakeGitHubChangeLinks{}
	coordinator := taskChangeLinkCoordinator{
		tasks: taskSvc, github: provider,
		singleUserIdentity: taskChangeLinkIdentityResolver(newDisabledAuthService(t)),
	}

	for _, repositoryID := range []string{"acme/api", "repo-unattached"} {
		t.Run(repositoryID, func(t *testing.T) {
			response := dispatchTaskChangeManagement(t, taskSvc, repos, coordinator, map[string]interface{}{
				"operation": "link", "task_id": "task-1", "caller_task_id": "task-caller",
				"provider": "github", "repository_id": repositoryID, "number": 42,
			})

			require.Equal(t, ws.MessageTypeError, response.Type)
			var failure ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &failure))
			require.Equal(t, ws.ErrorCodeValidation, failure.Code)
			require.Equal(t, "repository_id must be the Kandev ID of a repository attached to the target task", failure.Message)
			require.Nil(t, failure.Details)
			require.Empty(t, provider.linkedURL)
			require.Empty(t, provider.prs)
		})
	}

	response := dispatchTaskChangeManagement(t, taskSvc, repos, coordinator, map[string]interface{}{
		"operation": "link", "task_id": "task-1", "caller_task_id": "task-caller",
		"provider": "github", "repository_id": "repo-1", "number": 42,
	})
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var result mcphandlers.TaskChangeLinkMutationResult
	require.NoError(t, json.Unmarshal(response.Payload, &result))
	require.Equal(t, []mcphandlers.TaskChangeLink{{Provider: "github", RepositoryID: "repo-1", Number: 42}}, result.Links)
	require.Equal(t, "https://github.com/acme/api/pull/42", provider.linkedURL)

	provider.linkErr = errors.New("provider rejected link")
	response = dispatchTaskChangeManagement(t, taskSvc, repos, coordinator, map[string]interface{}{
		"operation": "link", "task_id": "task-1", "caller_task_id": "task-caller",
		"provider": "github", "repository_id": "repo-1", "number": 43,
	})
	require.Equal(t, ws.MessageTypeError, response.Type)
	var failure ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &failure))
	require.Equal(t, ws.ErrorCodeValidation, failure.Code)
	require.Equal(t, "failed to manage task change request", failure.Message)
	require.Equal(t, "task change request operation failed", failure.Details["operation_error"])
	require.NotContains(t, failure.Details["operation_error"], "provider rejected link")
	require.Equal(t, false, failure.Details["state_known"])
	require.Len(t, provider.prs, 1)
}

// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2
func TestTaskChangeLinkMCPSingleUserRejectsInvalidReach(t *testing.T) {
	for _, tc := range []struct {
		name, caller, target, repository, code string
	}{
		{"forged caller", "task-other", "task-1", "repo-1", ws.ErrorCodeForbidden},
		{"foreign workspace", "task-1", "task-foreign", "repo-foreign", ws.ErrorCodeForbidden},
		{"unattached repository", "task-1", "task-1", "repo-foreign", ws.ErrorCodeValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskSvc, repos := newTaskChangeCoordinatorHarness(t)
			seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
			seedTaskChangeCoordinatorTask(t, repos, "ws-foreign", "task-foreign", "repo-foreign", "https://github.com", "other", "api")
			provider := &fakeGitHubChangeLinks{}
			coordinator := taskChangeLinkCoordinator{
				tasks: taskSvc, github: provider,
				singleUserIdentity: taskChangeLinkIdentityResolver(newDisabledAuthService(t)),
			}
			ctx := scope.WithPrincipal(context.Background(), scope.Principal{
				WorkspaceID: "ws-1", CallerTaskID: "task-1", CallerSessionID: "session-1",
			})
			response := dispatchTaskChangeLink(t, coordinator, ctx, map[string]any{
				"task_id": tc.target, "caller_task_id": tc.caller, "provider": "github",
				"repository_id": tc.repository, "number": 42,
			})
			require.Equal(t, ws.MessageTypeError, response.Type)
			var failure ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &failure))
			require.Equal(t, tc.code, failure.Code)
			require.Empty(t, provider.linkedURL)
			require.Empty(t, provider.prs)
		})
	}
}
