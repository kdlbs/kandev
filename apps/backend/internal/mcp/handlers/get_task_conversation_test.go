package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGetTaskConversation_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{})
	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleGetTaskConversation_UsesPrimarySession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	_, err := svc.CreateMessage(context.Background(), &service.CreateMessageRequest{
		TaskSessionID: sess.ID,
		TaskID:        task.ID,
		AuthorType:    "user",
		Content:       "hello from task",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": task.ID,
		"limit":   10,
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, task.ID, payload["task_id"])
	assert.Equal(t, sess.ID, payload["session_id"])
	assert.Equal(t, float64(1), payload["total"])
}

func TestHandleGetTaskConversation_SessionMustBelongToTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	taskA, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	// Create another task/session in the same workflow to validate cross-task mismatch.
	taskB, err := svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Other task",
	})
	require.NoError(t, err)
	sessB := &models.TaskSession{
		ID:             "sess-2",
		TaskID:         taskB.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      true,
		State:          models.TaskSessionStateWaitingForInput,
	}
	require.NoError(t, repo.CreateTaskSession(context.Background(), sessB))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id":    taskA.ID,
		"session_id": "sess-2",
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
