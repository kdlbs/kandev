package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The workflow-step guards deny by returning the one not-found error the
// workflow package uses for both "not yours" and "not there". These handlers
// turned every error into INTERNAL_ERROR, so an agent that named a workflow,
// step or task it cannot reach was told the backend had broken — while the
// REST and WS surfaces answered not-found for the same request.

// scopedMCPCtx carries a real (non-synthetic) identity, the way
// internal/mcp/scope attaches the task owner's identity to an in-session call.
func scopedMCPCtx() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Role: authn.RoleMember})
}

// setupScopedWorkflowHandlers builds the MCP handlers over a real workflow
// service whose task-domain checker answers with the supplied error.
func setupScopedWorkflowHandlers(t *testing.T, checkerErr error) (*Handlers, string) {
	t.Helper()
	h, _, repo := setupImportHandlers(t)
	h.workflowCtrl = workflowctrl.NewController(h.workflowSvc)
	h.workflowSvc.SetWorkflowAccessChecker(func(context.Context, string) error { return checkerErr })
	h.workflowSvc.SetWorkspaceAccessChecker(func(context.Context, string) error { return checkerErr })

	step := &wfmodels.WorkflowStep{WorkflowID: "wf-other", Name: "Backlog", Color: "#111111"}
	require.NoError(t, h.workflowSvc.CreateStep(context.Background(), step))
	// Keep the repo handle used, so the fixture stays honest about writing a
	// real row rather than a fake.
	stored, err := repo.GetStep(context.Background(), step.ID)
	require.NoError(t, err)
	return h, stored.ID
}

func requireErrorCode(t *testing.T, msg *ws.Message, want string) {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, msg.Type, "payload: %s", msg.Payload)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	require.Equal(t, want, payload.Code, "message: %s", payload.Message)
}

func TestMCPWorkflowHandlersReportDenialsAsNotFound(t *testing.T) {
	h, stepID := setupScopedWorkflowHandlers(t, repoerrors.ErrWorkspaceNotFound)
	ctx := scopedMCPCtx()

	cases := []struct {
		name    string
		handler func(context.Context, *ws.Message) (*ws.Message, error)
		payload map[string]any
	}{
		{"delete step", h.handleDeleteWorkflowStep, map[string]any{"step_id": stepID}},
		{"update step", h.handleUpdateWorkflowStep, map[string]any{"step_id": stepID, "name": "Renamed"}},
		{"create step", h.handleCreateWorkflowStep, map[string]any{"workflow_id": "wf-other", "name": "New"}},
		{"reorder steps", h.handleReorderWorkflowSteps,
			map[string]any{"workflow_id": "wf-other", "step_ids": []string{stepID}}},
		{"export workflow", h.handleExportWorkflow, map[string]any{"workflow_id": "wf-other"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.payload)
			require.NoError(t, err)
			resp, err := tc.handler(ctx, &ws.Message{ID: "req-1", Action: "test", Payload: raw})
			require.NoError(t, err)
			requireErrorCode(t, resp, ws.ErrorCodeNotFound)
		})
	}
}

// A failure is still a failure: masking a database error as not-found would
// tell an agent its workflow is gone and invite it to recreate one.
func TestMCPWorkflowHandlersKeepRealFailuresInternal(t *testing.T) {
	h, _ := setupScopedWorkflowHandlers(t, errors.New("database is locked"))

	raw, err := json.Marshal(map[string]any{"workflow_id": "wf-other"})
	require.NoError(t, err)
	resp, err := h.handleExportWorkflow(scopedMCPCtx(), &ws.Message{ID: "req-1", Action: "test", Payload: raw})
	require.NoError(t, err)
	requireErrorCode(t, resp, ws.ErrorCodeInternalError)
}
