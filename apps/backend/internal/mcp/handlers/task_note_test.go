package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func seedMCPTaskNoteTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-note", Name: "Notes", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-note", WorkspaceID: "ws-note", Name: "WF"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-note", WorkflowID: "wf-note", Title: "Task", State: v1.TaskStateCreated, Priority: "medium", CreatedAt: now, UpdatedAt: now}))
}

func TestHandleGetTaskNote_MissingReturnsEmptyObject(t *testing.T) {
	svc, repo := newTestTaskService(t)
	noteService := service.NewNoteService(repo, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetNoteService(noteService)
	seedMCPTaskNoteTask(t, repo, "task-1")

	resp, err := h.handleGetTaskNote(context.Background(), makeWSMessage(t, ws.ActionMCPGetTaskNote, map[string]interface{}{"task_id": "task-1"}))
	require.NoError(t, err)
	if string(resp.Payload) != "{}" {
		t.Fatalf("expected empty object payload, got %s", string(resp.Payload))
	}
}

func TestHandleUpdateTaskNote_ForwardsAgentDefault(t *testing.T) {
	svc, repo := newTestTaskService(t)
	noteService := service.NewNoteService(repo, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetNoteService(noteService)
	seedMCPTaskNoteTask(t, repo, "task-1")

	resp, err := h.handleUpdateTaskNote(context.Background(), makeWSMessage(t, ws.ActionMCPUpdateTaskNote, map[string]interface{}{
		"task_id": "task-1",
		"content": "note content",
	}))
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	if payload["content"] != "note content" || payload["updated_by"] != "agent" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
