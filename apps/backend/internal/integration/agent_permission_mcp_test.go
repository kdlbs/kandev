package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/securityutil"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedMCPAgentPermissionListResolveAndReplay(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(func() { ts.Close() })

	h := mcphandlers.NewHandlers(
		ts.TaskSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		ts.OrchestratorSvc, ts.OrchestratorSvc.GetMessageQueue(), ts.Logger,
	)
	h.SetAgentPermissionService(ts.OrchestratorSvc)
	h.RegisterHandlers(ts.Gateway.Dispatcher)
	ts.OrchestratorSvc.SetSessionAccessChecker(ts.TaskSvc.AuthorizeSessionAccess)
	ts.OrchestratorSvc.SetTaskAccessChecker(ts.TaskSvc.AuthorizeTaskAccess)

	ownerCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "owner-user", TokenID: "SECRET_TOKEN_RECORD_ID", Role: authn.RoleMember,
	})
	otherCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "other-user", TokenID: "other-token", Role: authn.RoleAdmin,
	})
	taskID, sessionID := createOwnedPermissionSession(t, ts, ownerCtx)
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	const (
		actionCommandSentinel = "echo kandev_pat_ACTION_COMMAND_SECRET"
		actionCWDSentinel     = "/workspace/token=ACTION_CWD_SECRET"
		actionDetailsSentinel = "ghp_AAAAAAAAAAAAAAAAAAAA"
	)
	action := securityutil.ProjectPermissionAction("command", "Run command", map[string]any{
		"command": actionCommandSentinel,
		"cwd":     actionCWDSentinel,
	})
	permission := streams.PendingAgentPermission{
		TaskID: taskID, SessionID: sessionID, RequestID: "request-1", PendingID: "pending-1",
		ToolCallID: "tool-call-1", Title: "Run command",
		Action: streams.PermissionAction{
			Type: action.Type, Description: action.Description, Command: action.Command,
			CWD: action.CWD, Redacted: action.Redacted,
		},
		Options: []streams.PermissionChoice{
			{OptionID: "allow-once", Name: "Allow once", Kind: streams.PermissionOptionKindAllowOnce},
			{OptionID: "reject-once", Name: "Deny", Kind: streams.PermissionOptionKindRejectOnce},
		},
		CreatedAt: createdAt, Status: streams.PermissionStatusPending,
	}
	ts.AgentManager.SetPendingPermissions(sessionID, []streams.PendingAgentPermission{permission})
	messageAdapter := &testMessageCreatorAdapter{svc: ts.TaskSvc}
	_, err := messageAdapter.CreatePermissionRequestMessage(ownerCtx, taskID, sessionID, permission.RequestID, permission.PendingID,
		permission.ToolCallID, permission.Title, "", []map[string]interface{}{{"option_id": "allow-once"}}, "command", map[string]interface{}{"command": actionDetailsSentinel})
	require.NoError(t, err)

	foreign := dispatchPermissionMCP(t, ts, otherCtx, ws.ActionMCPListPendingAgentPermissions, map[string]any{"task_id": taskID, "session_id": sessionID})
	require.Equal(t, ws.MessageTypeError, foreign.Type)
	assert.Contains(t, string(foreign.Payload), "task_or_session_not_found")
	assert.Equal(t, 0, ts.AgentManager.PermissionResponseCount())

	listed := dispatchPermissionMCP(t, ts, ownerCtx, ws.ActionMCPListPendingAgentPermissions, map[string]any{"task_id": taskID, "session_id": sessionID})
	require.Equal(t, ws.MessageTypeResponse, listed.Type)
	assert.Contains(t, string(listed.Payload), `"request_id":"request-1"`)
	assert.Contains(t, string(listed.Payload), `"option_id":"allow-once"`)
	for _, sentinel := range []string{"SECRET_TOKEN_RECORD_ID", actionCommandSentinel, actionCWDSentinel, actionDetailsSentinel} {
		assert.NotContains(t, string(listed.Payload), sentinel)
	}

	resolveArgs := map[string]any{
		"task_id": taskID, "session_id": sessionID, "request_id": "request-1",
		"pending_id": "pending-1", "option_id": "allow-once",
	}
	inSession := dispatchInSessionPermissionMCP(t, ts, ownerCtx, ws.ActionMCPResolveAgentPermission, resolveArgs)
	require.Equal(t, ws.MessageTypeError, inSession.Type)
	assert.Contains(t, string(inSession.Payload), ws.ErrorCodeForbidden)
	assert.Equal(t, 0, ts.AgentManager.PermissionResponseCount())
	forgedAudit, err := ts.TaskSvc.GetPermissionResolutionAudit(ownerCtx, taskID, sessionID, "request-1", "pending-1")
	require.NoError(t, err)
	assert.Nil(t, forgedAudit, "in-session dispatch must not create an external-MCP audit")

	resolved := dispatchPermissionMCP(t, ts, ownerCtx, ws.ActionMCPResolveAgentPermission, resolveArgs)
	require.Equal(t, ws.MessageTypeResponse, resolved.Type, string(resolved.Payload))
	assert.Contains(t, string(resolved.Payload), `"option_kind":"allow_once"`)
	assert.Equal(t, 1, ts.AgentManager.PermissionResponseCount())

	replayed := dispatchPermissionMCP(t, ts, ownerCtx, ws.ActionMCPResolveAgentPermission, resolveArgs)
	require.Equal(t, ws.MessageTypeError, replayed.Type)
	assert.Contains(t, string(replayed.Payload), "permission_already_resolved")
	assert.Equal(t, 1, ts.AgentManager.PermissionResponseCount(), "replay must not reach the provider")

	audit, err := ts.TaskSvc.GetPermissionResolutionAudit(ownerCtx, taskID, sessionID, "request-1", "pending-1")
	require.NoError(t, err)
	require.NotNil(t, audit)
	assert.Equal(t, models.PermissionActorPersonalAccessToken, audit.ActorKind)
	assert.Equal(t, models.PermissionSourceExternalMCP, audit.Source)
	encoded, err := json.Marshal(audit)
	require.NoError(t, err)
	for _, sentinel := range []string{"SECRET_TOKEN_RECORD_ID", actionCommandSentinel, actionCWDSentinel, actionDetailsSentinel} {
		assert.NotContains(t, string(encoded), sentinel)
	}
}

func createOwnedPermissionSession(t *testing.T, ts *OrchestratorTestServer, ctx context.Context) (string, string) {
	t.Helper()
	workspace, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Permission workspace"})
	require.NoError(t, err)
	templateID := "simple"
	workflow, err := ts.TaskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspace.ID, Name: "Permission workflow", WorkflowTemplateID: &templateID,
	})
	require.NoError(t, err)
	steps, err := ts.WorkflowSvc.ListStepsByWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	created, err := ts.TaskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: workspace.ID, WorkflowID: workflow.ID, WorkflowStepID: steps[0].ID,
		Title: "Permission task", Description: "Test permission resolution", Priority: "medium",
	})
	require.NoError(t, err)
	sessionID := "permission-session"
	now := time.Now().UTC()
	require.NoError(t, ts.TaskRepo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: created.Task.ID, IsPrimary: true, State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}))
	return created.Task.ID, sessionID
}

func dispatchPermissionMCP(t *testing.T, ts *OrchestratorTestServer, ctx context.Context, action string, payload map[string]any) *ws.Message {
	t.Helper()
	return dispatchPermissionMCPContext(t, ts, mcporigin.WithTrustedExternalTransport(ctx), action, payload)
}

func dispatchInSessionPermissionMCP(t *testing.T, ts *OrchestratorTestServer, ctx context.Context, action string, payload map[string]any) *ws.Message {
	t.Helper()
	return dispatchPermissionMCPContext(t, ts, ctx, action, payload)
}

func dispatchPermissionMCPContext(t *testing.T, ts *OrchestratorTestServer, ctx context.Context, action string, payload map[string]any) *ws.Message {
	t.Helper()
	request, err := ws.NewRequest("permission-request", action, payload)
	require.NoError(t, err)
	response, err := ts.Gateway.Dispatcher.Dispatch(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}
