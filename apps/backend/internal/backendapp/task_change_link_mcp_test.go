package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/mcp/scope"
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
