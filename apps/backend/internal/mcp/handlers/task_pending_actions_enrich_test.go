package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestEnrichTasksWithPendingActions_NoTasksIsNoop(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	dtos := []dto.TaskDTO{}
	h.enrichTasksWithPendingActions(context.Background(), nil, dtos)
	assert.Empty(t, dtos)
}

// seedTaskWithClarificationSession creates a task (in the given, already-
// created workspace/workflow) with a primary RUNNING session carrying a
// pending clarification_request message — the fixture spec T1 calls "a
// blocked task in a workflow". sessionID must be unique across the test.
func seedTaskWithClarificationSession(t *testing.T, svc *service.Service, repo interface {
	CreateTaskSession(ctx context.Context, s *models.TaskSession) error
	CreateTurn(ctx context.Context, tn *models.Turn) error
	CreateMessage(ctx context.Context, m *models.Message) error
}, wsID, wfID, title, sessionID string) *models.Task {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	taskResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{WorkspaceID: wsID, WorkflowID: wfID, Title: title})
	require.NoError(t, err)
	task := taskResult.Task

	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: task.ID, IsPrimary: true, State: models.TaskSessionStateRunning,
	}))
	turnID := "turn-" + sessionID
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: task.ID, StartedAt: now}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "clarify-" + sessionID, TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest,
		Content: "Which approach?", CreatedAt: now,
		Metadata: map[string]interface{}{"pending_id": "pending-" + sessionID, "status": "pending"},
	}))
	return task
}

func TestEnrichTasksWithPendingActions_ClarificationBlocksTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-clar", Name: "Clar", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-clar", WorkspaceID: "ws-clar", Name: "Clar", CreatedAt: now, UpdatedAt: now}))
	task := seedTaskWithClarificationSession(t, svc, repo, "ws-clar", "wf-clar", "Blocked task", "sess-clar")

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	dtos := []dto.TaskDTO{dto.FromTask(task)}
	h.enrichTasksWithPendingActions(ctx, []*models.Task{task}, dtos)

	require.NotNil(t, dtos[0].TaskPendingAction)
	assert.Equal(t, "clarification", *dtos[0].TaskPendingAction)
	require.NotNil(t, dtos[0].PrimarySessionPendingAction)
	assert.Equal(t, "clarification", *dtos[0].PrimarySessionPendingAction)
}

func TestEnrichTasksWithPendingActions_TaskWithNoBlockedSessionGetsNullFields(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-idle", Name: "Idle", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-idle", WorkspaceID: "ws-idle", Name: "Idle", CreatedAt: now, UpdatedAt: now}))
	taskResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{WorkspaceID: "ws-idle", WorkflowID: "wf-idle", Title: "Idle task"})
	require.NoError(t, err)
	task := taskResult.Task

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	dtos := []dto.TaskDTO{dto.FromTask(task)}
	h.enrichTasksWithPendingActions(ctx, []*models.Task{task}, dtos)

	assert.Nil(t, dtos[0].TaskPendingAction, "a task with no session must carry JSON null, not an empty string")
	assert.Nil(t, dtos[0].PrimarySessionPendingAction, "a task with no session must carry JSON null, not an empty string")
}

func TestHandleListTasks_IncludesPendingActionFields(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pending", Name: "Pending", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-pending", WorkspaceID: "ws-pending", Name: "Pending", CreatedAt: now, UpdatedAt: now}))
	seedTaskWithClarificationSession(t, svc, repo, "ws-pending", "wf-pending", "Blocked task", "sess-blocked")

	_, err := svc.CreateTask(ctx, &service.CreateTaskRequest{WorkspaceID: "ws-pending", WorkflowID: "wf-pending", Title: "Idle task"})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]any{"workflow_id": "wf-pending"})
	resp, err := h.handleListTasks(ctx, msg)
	require.NoError(t, err)

	// T2: assert the raw JSON keys are present with an explicit null, not
	// merely absent (which omitempty would also produce for the zero value).
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Payload, &raw))
	var rawTasks []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["tasks"], &rawTasks))
	require.Len(t, rawTasks, 2)

	byTitle := map[string]map[string]json.RawMessage{}
	for _, rt := range rawTasks {
		var title string
		require.NoError(t, json.Unmarshal(rt["title"], &title))
		byTitle[title] = rt
	}

	blocked, ok := byTitle["Blocked task"]
	require.True(t, ok)
	require.Contains(t, blocked, "task_pending_action")
	require.Contains(t, blocked, "primary_session_pending_action")
	assert.JSONEq(t, `"clarification"`, string(blocked["task_pending_action"]))
	assert.JSONEq(t, `"clarification"`, string(blocked["primary_session_pending_action"]))

	idle, ok := byTitle["Idle task"]
	require.True(t, ok)
	require.Contains(t, idle, "task_pending_action", "T2: key present even with no blocked session")
	require.Contains(t, idle, "primary_session_pending_action", "T2: key present even with no blocked session")
	assert.JSONEq(t, `null`, string(idle["task_pending_action"]))
	assert.JSONEq(t, `null`, string(idle["primary_session_pending_action"]))

	var payload dto.ListTasksResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Tasks, 2)
}
