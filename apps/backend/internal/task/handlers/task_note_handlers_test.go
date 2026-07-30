package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func makeTaskNoteWSMessage(t *testing.T, action string, payload interface{}) *ws.Message {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return &ws.Message{ID: "test-id", Type: ws.MessageTypeRequest, Action: action, Payload: data}
}

func newTestNoteHandlers(t *testing.T) (*TaskHandlers, *sqliterepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "task-note-handlers.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := taskrepository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlxDB.Close()
		_ = cleanup()
	})
	log := newTestLogger(t)
	noteService := service.NewNoteService(repo, nil, log)
	return &TaskHandlers{noteService: noteService, logger: log}, repo
}

func seedTaskForNoteHandler(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-note", Name: "Notes"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-note", WorkspaceID: "ws-note", Name: "WF"})
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-note",
		WorkflowID:  "wf-note",
		Title:       "Task",
		State:       v1.TaskStateCreated,
		Priority:    "medium",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
}

func decodeTaskHandlerError(t *testing.T, msg *ws.Message) ws.ErrorPayload {
	t.Helper()
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	return payload
}

func TestTaskNoteHandlers_GetMissingReturnsNull(t *testing.T) {
	h, repo := newTestNoteHandlers(t)
	seedTaskForNoteHandler(t, repo, "task-1")
	msg := makeTaskNoteWSMessage(t, ws.ActionTaskNoteGet, map[string]interface{}{"task_id": "task-1"})

	resp, err := h.wsGetTaskNote(context.Background(), msg)
	require.NoError(t, err)
	if string(resp.Payload) != "null" {
		t.Fatalf("expected null payload, got %s", string(resp.Payload))
	}
}

func TestTaskNoteHandlers_UpsertAndDelete(t *testing.T) {
	h, repo := newTestNoteHandlers(t)
	seedTaskForNoteHandler(t, repo, "task-1")

	resp, err := h.wsUpsertTaskNote(context.Background(), makeTaskNoteWSMessage(t, ws.ActionTaskNoteUpdate, map[string]interface{}{
		"task_id": "task-1",
		"content": "hello",
	}))
	require.NoError(t, err)
	var note map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &note))
	if note["content"] != "hello" || note["updated_by"] != "user" {
		t.Fatalf("unexpected note payload: %+v", note)
	}

	resp, err = h.wsDeleteTaskNote(context.Background(), makeTaskNoteWSMessage(t, ws.ActionTaskNoteDelete, map[string]interface{}{"task_id": "task-1"}))
	require.NoError(t, err)
	var deleted map[string]bool
	require.NoError(t, json.Unmarshal(resp.Payload, &deleted))
	if !deleted[responseKeySuccess] {
		t.Fatalf("expected success response, got %+v", deleted)
	}
}

func TestTaskNoteHandlers_DeleteMissingMapsNotFound(t *testing.T) {
	h, repo := newTestNoteHandlers(t)
	seedTaskForNoteHandler(t, repo, "task-1")

	resp, err := h.wsDeleteTaskNote(context.Background(), makeTaskNoteWSMessage(t, ws.ActionTaskNoteDelete, map[string]interface{}{"task_id": "task-1"}))
	require.NoError(t, err)
	payload := decodeTaskHandlerError(t, resp)
	if payload.Code != ws.ErrorCodeNotFound {
		t.Fatalf("expected not_found, got %+v", payload)
	}
}
