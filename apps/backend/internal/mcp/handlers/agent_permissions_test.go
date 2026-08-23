package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type permissionServiceStub struct {
	permissions []streams.PendingAgentPermission
	listErr     error
	resolve     *orchestrator.ResolveAgentPermissionResult
	resolveErr  error
	listedTask  string
	listedSess  string
	resolved    orchestrator.ResolveAgentPermissionRequest
}

func (s *permissionServiceStub) ListPendingAgentPermissions(_ context.Context, taskID, sessionID string) ([]streams.PendingAgentPermission, error) {
	s.listedTask, s.listedSess = taskID, sessionID
	return s.permissions, s.listErr
}

func (s *permissionServiceStub) ResolveAgentPermission(_ context.Context, req orchestrator.ResolveAgentPermissionRequest) (*orchestrator.ResolveAgentPermissionResult, error) {
	s.resolved = req
	return s.resolve, s.resolveErr
}

func TestHandleListPendingAgentPermissionsReturnsSafeServiceProjection(t *testing.T) {
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	stub := &permissionServiceStub{permissions: []streams.PendingAgentPermission{{
		TaskID: "task-1", SessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		Title: "Run command", Action: streams.PermissionAction{Type: "command", Command: "git status", CWD: "/workspace", Redacted: true},
		Options:   []streams.PermissionChoice{{OptionID: "allow-once", Name: "Allow once", Kind: streams.PermissionOptionKindAllowOnce}},
		CreatedAt: createdAt, Status: streams.PermissionStatusPending,
	}}}
	h := &Handlers{agentPermissionSvc: stub, logger: testLogger(t).WithFields()}

	resp, err := h.handleListPendingAgentPermissions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingAgentPermissions, map[string]any{
		"task_id": "task-1", "session_id": "session-1",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, "task-1", stub.listedTask)
	assert.Equal(t, "session-1", stub.listedSess)
	assert.NotContains(t, string(resp.Payload), "SECRET_CANARY")

	var payload struct {
		TaskID      string                           `json:"task_id"`
		Permissions []streams.PendingAgentPermission `json:"permissions"`
		Total       int                              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "task-1", payload.TaskID)
	require.Len(t, payload.Permissions, 1)
	assert.Equal(t, "request-1", payload.Permissions[0].RequestID)
	assert.Equal(t, 1, payload.Total)
}

func TestHandleResolveAgentPermissionForwardsOnlyExactIdentity(t *testing.T) {
	stub := &permissionServiceStub{resolve: &orchestrator.ResolveAgentPermissionResult{
		TaskID: "task-1", SessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		OptionID: "allow-once", OptionKind: streams.PermissionOptionKindAllowOnce,
		Source: models.PermissionSourceExternalMCP, Status: "resolved",
	}}
	h := &Handlers{agentPermissionSvc: stub, logger: testLogger(t).WithFields()}

	resp, err := h.handleResolveAgentPermission(mcporigin.WithTrustedExternalTransport(context.Background()), makeWSMessage(t, ws.ActionMCPResolveAgentPermission, map[string]any{
		"task_id": "task-1", "session_id": "session-1", "request_id": "request-1",
		"pending_id": "pending-1", "option_id": "allow-once",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, orchestrator.ResolveAgentPermissionRequest{
		TaskID: "task-1", SessionID: "session-1", RequestID: "request-1", PendingID: "pending-1",
		OptionID: "allow-once", Source: models.PermissionSourceExternalMCP,
	}, stub.resolved)
	assert.Contains(t, string(resp.Payload), `"status":"resolved"`)
}

func TestHandleResolveAgentPermissionRejectsUntrustedTransport(t *testing.T) {
	stub := &permissionServiceStub{resolve: &orchestrator.ResolveAgentPermissionResult{Status: "resolved"}}
	h := &Handlers{agentPermissionSvc: stub, logger: testLogger(t).WithFields()}

	resp, err := h.handleResolveAgentPermission(context.Background(), makeWSMessage(t, ws.ActionMCPResolveAgentPermission, map[string]any{
		"task_id": "task-1", "session_id": "session-1", "request_id": "request-1",
		"pending_id": "pending-1", "option_id": "allow-once",
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
	assert.Contains(t, string(resp.Payload), ws.ErrorCodeForbidden)
	assert.Empty(t, stub.resolved.TaskID, "untrusted dispatch must not reach the permission service")
}

func TestAgentPermissionHandlersValidateRequiredIdentity(t *testing.T) {
	h := &Handlers{agentPermissionSvc: &permissionServiceStub{}, logger: testLogger(t).WithFields()}

	listResp, err := h.handleListPendingAgentPermissions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingAgentPermissions, map[string]any{}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, listResp.Type)
	assert.Contains(t, string(listResp.Payload), ws.ErrorCodeValidation)

	resolveResp, err := h.handleResolveAgentPermission(context.Background(), makeWSMessage(t, ws.ActionMCPResolveAgentPermission, map[string]any{
		"task_id": "task-1", "session_id": "session-1", "request_id": "request-1", "pending_id": "pending-1",
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resolveResp.Type)
	assert.Contains(t, string(resolveResp.Payload), "option_id is required")
}

func TestAgentPermissionHandlersPreserveStableDomainFailures(t *testing.T) {
	cases := []error{
		orchestrator.ErrPermissionTaskOrSessionNotFound,
		orchestrator.ErrPermissionNotFound,
		orchestrator.ErrPermissionStale,
		orchestrator.ErrPermissionAlreadyResolved,
		orchestrator.ErrPermissionResolutionInProgress,
		orchestrator.ErrPermissionOptionNotOffered,
		orchestrator.ErrPermissionAuditFailed,
		orchestrator.ErrPermissionDeliveryFailed,
	}
	for _, domainErr := range cases {
		t.Run(domainErr.Error(), func(t *testing.T) {
			stub := &permissionServiceStub{resolveErr: domainErr}
			h := &Handlers{agentPermissionSvc: stub, logger: testLogger(t).WithFields()}
			resp, err := h.handleResolveAgentPermission(mcporigin.WithTrustedExternalTransport(context.Background()), makeWSMessage(t, ws.ActionMCPResolveAgentPermission, map[string]any{
				"task_id": "task-1", "session_id": "session-1", "request_id": "request-1",
				"pending_id": "pending-1", "option_id": "allow-once",
			}))
			require.NoError(t, err)
			assert.Equal(t, ws.MessageTypeError, resp.Type)
			assert.Contains(t, string(resp.Payload), `"code":"`+domainErr.Error()+`"`)
		})
	}
}

func TestAgentPermissionHandlersHideUnexpectedErrors(t *testing.T) {
	stub := &permissionServiceStub{listErr: errors.New("database password SECRET_CANARY")}
	h := &Handlers{agentPermissionSvc: stub, logger: testLogger(t).WithFields()}
	resp, err := h.handleListPendingAgentPermissions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingAgentPermissions, map[string]any{"task_id": "task-1"}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
	assert.NotContains(t, string(resp.Payload), "SECRET_CANARY")
}
