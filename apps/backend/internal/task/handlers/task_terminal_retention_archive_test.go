package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type heldArchiveRepo struct{ archiveStampRepo }

func (r *heldArchiveRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-1", Metadata: map[string]interface{}{
		models.MetaKeyTerminalRetention: true,
	}}, nil
}

func TestHTTPArchiveHeldTaskReturnsConflict(t *testing.T) {
	repo := &heldArchiveRepo{}
	h := &TaskHandlers{handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil), logger: newTestLogger(t)}
	c, rec := taskRequestAs(t, "", http.MethodPost, "/api/v1/tasks/task-1/archive", "task-1")
	h.httpArchiveTask(c)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Empty(t, repo.stamped)
}

func TestWSArchiveHeldTaskReturnsConflict(t *testing.T) {
	repo := &heldArchiveRepo{}
	h := &TaskHandlers{handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil), logger: newTestLogger(t)}
	msg := &ws.Message{ID: "msg-1", Action: ws.ActionTaskArchive, Payload: json.RawMessage(`{"id":"task-1"}`)}
	resp, err := h.wsArchiveTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var payload struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, string(ws.ErrorCodeConflict), payload.Code)
	require.Empty(t, repo.stamped)
}
