package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// wsWorkflowRequest builds a request frame with an arbitrary payload.
func wsWorkflowRequest(t *testing.T, action string, payload any) *ws.Message {
	t.Helper()
	msg, err := ws.NewRequest("req-1", action, payload)
	require.NoError(t, err)
	return msg
}

// wsMalformedRequest builds a frame whose payload is not valid JSON, which is
// the only way to reach the ParsePayload error branches.
func wsMalformedRequest(action string) *ws.Message {
	return &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: action, Payload: json.RawMessage(`{`)}
}

func wsWorkflowResponse(t *testing.T, msg *ws.Message, out any) {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, msg.Type, "expected a response frame, got %s", msg.Payload)
	require.NoError(t, json.Unmarshal(msg.Payload, out))
}

func TestWSListWorkflowsExcludesOfficeByDefault(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowList, map[string]any{"workspace_id": "ws-1"})

	resp, err := h.wsListWorkflows(context.Background(), msg)

	require.NoError(t, err)
	var list dto.ListWorkflowsResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, []string{"wf-1"}, workflowIDsOf(list.Workflows))
	require.Equal(t, 1, list.Total)
}

func TestWSListWorkflowsIncludesOfficeWhenExcludeOfficeIsFalse(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowList, map[string]any{
		"workspace_id":   "ws-1",
		"exclude_office": false,
		"include_hidden": true,
	})

	resp, err := h.wsListWorkflows(context.Background(), msg)

	require.NoError(t, err)
	var list dto.ListWorkflowsResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, []string{"wf-1", "wf-office"}, workflowIDsOf(list.Workflows))
	require.True(t, repo.listedHidden)
}

func TestWSListWorkflowsRejectsMalformedPayload(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsListWorkflows(context.Background(), wsMalformedRequest(ws.ActionWorkflowList))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeBadRequest), payload.Code)
	require.Contains(t, payload.Message, "Invalid payload")
}

func TestWSListWorkflowsSurfacesRepositoryFailure(t *testing.T) {
	repo := workflowFixture()
	repo.listErr = errors.New("database unavailable")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowList, map[string]any{"workspace_id": "ws-1"})

	resp, err := h.wsListWorkflows(context.Background(), msg)

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to list workflows", payload.Message)
}

func TestWSCreateWorkflowValidatesRequiredFields(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	for name, tc := range map[string]struct {
		payload any
		wantMsg string
	}{
		"missing name":      {payload: map[string]any{"workspace_id": "ws-1"}, wantMsg: "name is required"},
		"missing workspace": {payload: map[string]any{"name": "Flow"}, wantMsg: "workspace_id is required"},
	} {
		t.Run(name, func(t *testing.T) {
			msg := wsWorkflowRequest(t, ws.ActionWorkflowCreate, tc.payload)

			resp, err := h.wsCreateWorkflow(context.Background(), msg)

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
		})
	}
	require.Empty(t, repo.created)
}

func TestWSCreateWorkflowCreatesWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowCreate, map[string]any{
		"workspace_id": "ws-1", "name": "Flow", "prompt": "p",
	})

	resp, err := h.wsCreateWorkflow(context.Background(), msg)

	require.NoError(t, err)
	var created dto.WorkflowDTO
	wsWorkflowResponse(t, resp, &created)
	require.Len(t, repo.created, 1)
	require.Equal(t, repo.created[0].ID, created.ID)
	require.Equal(t, "Flow", created.Name)
	require.Equal(t, "ws-1", created.WorkspaceID)
	require.NotNil(t, created.Prompt)
	require.Equal(t, "p", *created.Prompt)
}

func TestWSCreateWorkflowRejectsImproveKandevWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowCreate, map[string]any{
		"workspace_id": "ws-improve", "name": "Flow",
	})

	resp, err := h.wsCreateWorkflow(context.Background(), msg)

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeConflict), payload.Code)
	require.Equal(t, workspaceReadOnlyMsg, payload.Message)
	require.Empty(t, repo.created, "the read-only workspace must not gain a workflow")
}

func TestWSCreateWorkflowReturnsNotFoundForUnknownWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowCreate, map[string]any{
		"workspace_id": "ws-missing", "name": "Flow",
	})

	resp, err := h.wsCreateWorkflow(context.Background(), msg)

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeNotFound), payload.Code)
	require.Equal(t, "Workspace not found", payload.Message)
	require.Empty(t, repo.created)
}

func TestWSGetWorkflowReturnsWorkflowOrErrors(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsGetWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowGet, map[string]any{"id": "wf-1"}))
	require.NoError(t, err)
	var got dto.WorkflowDTO
	wsWorkflowResponse(t, resp, &got)
	require.Equal(t, "wf-1", got.ID)

	missing, err := h.wsGetWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowGet, map[string]any{"id": "nope"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeNotFound), wsWorkflowError(t, missing).Code)

	empty, err := h.wsGetWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowGet, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)

	malformed, err := h.wsGetWorkflow(context.Background(), wsMalformedRequest(ws.ActionWorkflowGet))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, malformed).Code)
}

func TestWSUpdateWorkflowUpdatesFields(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	msg := wsWorkflowRequest(t, ws.ActionWorkflowUpdate, map[string]any{
		"id": "wf-1", "name": "Renamed", "description": "next",
	})

	resp, err := h.wsUpdateWorkflow(context.Background(), msg)

	require.NoError(t, err)
	var updated dto.WorkflowDTO
	wsWorkflowResponse(t, resp, &updated)
	require.Equal(t, "Renamed", updated.Name)
	require.Len(t, repo.updated, 1)
	require.Equal(t, "next", repo.updated[0].Description)
}

func TestWSUpdateWorkflowRequiresID(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsUpdateWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowUpdate, map[string]any{"name": "Renamed"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Equal(t, "id is required", payload.Message)
	require.Empty(t, repo.updated)
}

func TestWSUpdateWorkflowSurfacesRepositoryFailure(t *testing.T) {
	repo := workflowFixture()
	repo.updateErr = errors.New("disk full")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsUpdateWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowUpdate, map[string]any{"id": "wf-1", "name": "Renamed"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to update workflow", payload.Message)
}

// TestWSWorkflowMutationsRejectReadOnlyWorkflows is the WS twin of the HTTP
// read-only test: both reasons, both mutating actions, and no write recorded.
func TestWSWorkflowMutationsRejectReadOnlyWorkflows(t *testing.T) {
	for name, tc := range map[string]struct {
		workflowID string
		wantMsg    string
	}{
		"github synced":     {workflowID: "wf-synced", wantMsg: workflowReadOnlyMsg},
		"improve workspace": {workflowID: "wf-improve", wantMsg: workspaceReadOnlyMsg},
	} {
		t.Run(name, func(t *testing.T) {
			repo := workflowFixture()
			h := newWorkflowHandlers(t, repo, &stubStepLister{})

			update, err := h.wsUpdateWorkflow(context.Background(),
				wsWorkflowRequest(t, ws.ActionWorkflowUpdate, map[string]any{"id": tc.workflowID, "name": "x"}))
			require.NoError(t, err)
			updatePayload := wsWorkflowError(t, update)
			require.Equal(t, string(ws.ErrorCodeConflict), updatePayload.Code)
			require.Equal(t, tc.wantMsg, updatePayload.Message)
			require.Empty(t, repo.updated)

			del, err := h.wsDeleteWorkflow(context.Background(),
				wsWorkflowRequest(t, ws.ActionWorkflowDelete, map[string]any{"id": tc.workflowID}))
			require.NoError(t, err)
			deletePayload := wsWorkflowError(t, del)
			require.Equal(t, string(ws.ErrorCodeConflict), deletePayload.Code)
			require.Equal(t, tc.wantMsg, deletePayload.Message)
			require.Empty(t, repo.deleted)
		})
	}
}

func TestWSDeleteWorkflowDeletesEditableWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsDeleteWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowDelete, map[string]any{"id": "wf-1"}))

	require.NoError(t, err)
	var success dto.SuccessResponse
	wsWorkflowResponse(t, resp, &success)
	require.True(t, success.Success)
	require.Equal(t, []string{"wf-1"}, repo.deleted)
}

func TestWSDeleteWorkflowRequiresID(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsDeleteWorkflow(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowDelete, map[string]any{}))

	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, resp).Message)
	require.Empty(t, repo.deleted)
}

func TestWSReorderWorkflowsValidatesRequest(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	for name, tc := range map[string]struct {
		payload any
		wantMsg string
	}{
		"missing workspace": {
			payload: map[string]any{"workflow_ids": []string{"wf-1"}},
			wantMsg: "workspace_id is required",
		},
		"missing ids": {
			payload: map[string]any{"workspace_id": "ws-1"},
			wantMsg: "workflow_ids is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := h.wsReorderWorkflows(context.Background(),
				wsWorkflowRequest(t, ws.ActionWorkflowReorder, tc.payload))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
		})
	}
	require.Nil(t, repo.reordered)
}

func TestWSReorderWorkflowsRejectsImproveKandevWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsReorderWorkflows(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowReorder, map[string]any{
			"workspace_id": "ws-improve", "workflow_ids": []string{"wf-improve"},
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeConflict), payload.Code)
	require.Equal(t, workspaceReadOnlyMsg, payload.Message)
	require.Nil(t, repo.reordered)
}

func TestWSReorderWorkflowsPersistsOrder(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsReorderWorkflows(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowReorder, map[string]any{
			"workspace_id": "ws-1", "workflow_ids": []string{"wf-office", "wf-1"},
		}))

	require.NoError(t, err)
	var success dto.SuccessResponse
	wsWorkflowResponse(t, resp, &success)
	require.True(t, success.Success)
	require.Equal(t, "ws-1", repo.reorderedWorkspace)
	require.Equal(t, []string{"wf-office", "wf-1"}, repo.reordered)
}

func TestWSReorderWorkflowsSurfacesRepositoryFailure(t *testing.T) {
	repo := workflowFixture()
	repo.reorderErr = errors.New("constraint violation")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	resp, err := h.wsReorderWorkflows(context.Background(),
		wsWorkflowRequest(t, ws.ActionWorkflowReorder, map[string]any{
			"workspace_id": "ws-1", "workflow_ids": []string{"wf-1"},
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to reorder workflows", payload.Message)
}
