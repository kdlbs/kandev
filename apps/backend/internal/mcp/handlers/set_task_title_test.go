package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSetTaskTitle_UpdatesPendingTitleOnce(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-title", Name: "Titles", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "task-title",
		WorkspaceID: "ws-title",
		Title:       "Build the useful feature now",
		Description: "Build the useful feature now",
		State:       v1.TaskStateInProgress,
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title",
		"title":   "Useful Feature",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{
		"accepted": true,
		"task_id":  "task-title",
		"title":    "Useful Feature",
	})

	updated, err := svc.GetTask(ctx, "task-title")
	require.NoError(t, err)
	assert.Equal(t, "Useful Feature", updated.Title)
	assert.False(t, models.IsAgentTitlePending(updated.Metadata))

	resp, err = h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title",
		"title":   "Late Agent Title",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{
		"accepted": false,
		"task_id":  "task-title",
		"title":    "Useful Feature",
		"reason":   "title_not_pending",
	})
}

func assertTaskTitleResponse(t *testing.T, resp *ws.Message, want map[string]interface{}) {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &got))
	for key, value := range want {
		assert.Equal(t, value, got[key], "response field %s", key)
	}
}

func TestHandleSetTaskTitle_ValidatesInput(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	resp, err := h.handleSetTaskTitle(context.Background(), makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title",
		"title":   "  ",
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleSetTaskTitle_RejectsOverlongTitle(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-title-long", Name: "Titles", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "task-title-long",
		WorkspaceID: "ws-title-long",
		Title:       "Temporary title",
		Description: "Prompt",
		State:       v1.TaskStateInProgress,
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title-long",
		"title":   strings.Repeat("x", 501),
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
