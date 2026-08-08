package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTaskWithSessions creates a task carrying one primary session and the
// given number of extra spawned sessions, each started later than the last so
// the newest-first ordering is unambiguous.
func seedTaskWithSessions(t *testing.T, svc *service.Service, repo seedRepo, extra int) (*models.Task, *models.TaskSession, []*models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Multi-session task",
	})
	require.NoError(t, err)

	base := time.Now().Add(-time.Hour)
	primary := &models.TaskSession{
		ID:             "sess-primary",
		TaskID:         task.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      true,
		State:          models.TaskSessionStateWaitingForInput,
		StartedAt:      base,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, primary))

	spawned := make([]*models.TaskSession, 0, extra)
	for i := 0; i < extra; i++ {
		session := &models.TaskSession{
			ID:             fmt.Sprintf("sess-spawned-%d", i),
			TaskID:         task.ID,
			Name:           "reviewer",
			AgentProfileID: "agent-profile-2",
			IsPrimary:      false,
			State:          models.TaskSessionStateRunning,
			StartedAt:      base.Add(time.Duration(i+1) * time.Minute),
		}
		require.NoError(t, repo.CreateTaskSession(ctx, session))
		spawned = append(spawned, session)
	}
	return task, primary, spawned
}

func listSessionsPayload(t *testing.T, resp *ws.Message) map[string]interface{} {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	return payload
}

func sessionEntries(t *testing.T, payload map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := payload["sessions"].([]interface{})
	require.True(t, ok, "sessions must be an array")
	entries := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]interface{})
		require.True(t, ok, "each session must be an object")
		entries = append(entries, entry)
	}
	return entries
}

func TestHandleListTaskSessions_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{})
	resp, err := h.handleListTaskSessions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleListTaskSessions_ReturnsEverySessionNewestFirst(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, primary, spawned := seedTaskWithSessions(t, svc, repo, 2)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": task.ID,
	})

	resp, err := h.handleListTaskSessions(context.Background(), msg)
	require.NoError(t, err)
	payload := listSessionsPayload(t, resp)

	assert.Equal(t, task.ID, payload["task_id"])
	assert.Equal(t, float64(3), payload["total"])

	entries := sessionEntries(t, payload)
	require.Len(t, entries, 3)
	assert.Equal(t, []interface{}{spawned[1].ID, spawned[0].ID, primary.ID},
		[]interface{}{entries[0]["session_id"], entries[1]["session_id"], entries[2]["session_id"]},
		"sessions must be ordered newest-started first")

	assert.Equal(t, true, entries[2]["is_primary"], "the primary session must be flagged")
	assert.Equal(t, false, entries[0]["is_primary"])
	assert.Equal(t, "reviewer", entries[0]["name"])
	assert.Equal(t, string(models.TaskSessionStateWaitingForInput), entries[2]["state"])
	assert.Equal(t, "agent-profile-2", entries[0]["agent_profile_id"])
	assert.Contains(t, entries[0], "started_at")
}

// The caller's own session is marked so an agent listing siblings can tell
// which entry is itself before messaging one of them.
func TestHandleListTaskSessions_MarksCurrentSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, primary, spawned := seedTaskWithSessions(t, svc, repo, 1)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id":            task.ID,
		"current_session_id": primary.ID,
	})

	resp, err := h.handleListTaskSessions(context.Background(), msg)
	require.NoError(t, err)
	entries := sessionEntries(t, listSessionsPayload(t, resp))
	require.Len(t, entries, 2)

	assert.Equal(t, spawned[0].ID, entries[0]["session_id"])
	assert.Equal(t, false, entries[0]["is_current"])
	assert.Equal(t, primary.ID, entries[1]["session_id"])
	assert.Equal(t, true, entries[1]["is_current"])
}

// An empty list must mean "this task has no sessions yet", never "no such
// task" — otherwise a typo'd task ID reads as a task that was never started.
func TestHandleListTaskSessions_UnknownTaskIsNotFound(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedTaskWithSessions(t, svc, repo, 0)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": "task-that-does-not-exist",
	})

	resp, err := h.handleListTaskSessions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// Infrastructure failing during the existence check must not be flattened into
// "task not found" — that would report a database outage as a bad task ID.
func TestHandleListTaskSessions_LookupFailureIsInternalError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, _, _ := seedTaskWithSessions(t, svc, repo, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": task.ID,
	})

	resp, err := h.handleListTaskSessions(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleListTaskSessions_TaskWithoutSessionsReturnsEmptyList(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Never started",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": task.ID,
	})

	resp, err := h.handleListTaskSessions(ctx, msg)
	require.NoError(t, err)
	payload := listSessionsPayload(t, resp)
	assert.Equal(t, float64(0), payload["total"])
	assert.Equal(t, []interface{}{}, payload["sessions"], "sessions must serialize as [] rather than null")
}
