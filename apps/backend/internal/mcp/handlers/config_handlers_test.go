package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return log
}

func makeWSMessage(t *testing.T, action string, payload interface{}) *ws.Message {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  action,
		Payload: data,
	}
}

func assertWSError(t *testing.T, resp *ws.Message, expectedCode string) {
	t.Helper()
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	assert.Equal(t, expectedCode, ep.Code)
}

// --- Workflow handler tests ---

func TestHandleCreateWorkflowStep_MissingWorkflowID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"name": "Test Step",
	})

	resp, err := h.handleCreateWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateWorkflowStep_MissingName(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": "wf-123",
	})

	resp, err := h.handleCreateWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateWorkflowStep_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPCreateWorkflowStep,
		Payload: json.RawMessage(`{invalid`),
	}

	resp, err := h.handleCreateWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleUpdateWorkflowStep_MissingStepID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateWorkflowStep, map[string]interface{}{
		"name": "New Name",
	})

	resp, err := h.handleUpdateWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateWorkflowStep_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPUpdateWorkflowStep,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleUpdateWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleDeleteWorkflowStep_MissingStepID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPDeleteWorkflowStep, map[string]string{})

	resp, err := h.handleDeleteWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleDeleteWorkflowStep_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPDeleteWorkflowStep,
		Payload: json.RawMessage(`badjson`),
	}

	resp, err := h.handleDeleteWorkflowStep(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleReorderWorkflowSteps_MissingWorkflowID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPReorderWorkflowStep, map[string]interface{}{
		"step_ids": []string{"s1", "s2"},
	})

	resp, err := h.handleReorderWorkflowSteps(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleReorderWorkflowSteps_MissingStepIDs(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPReorderWorkflowStep, map[string]interface{}{
		"workflow_id": "wf-123",
		"step_ids":    []string{},
	})

	resp, err := h.handleReorderWorkflowSteps(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleReorderWorkflowSteps_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPReorderWorkflowStep,
		Payload: json.RawMessage(`{bad}`),
	}

	resp, err := h.handleReorderWorkflowSteps(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- Agent handler tests ---

func TestHandleUpdateAgent_MissingAgentID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateAgent, map[string]interface{}{
		"supports_mcp": true,
	})

	resp, err := h.handleUpdateAgent(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateAgent_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPUpdateAgent,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleUpdateAgent(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleListAgentProfiles_MissingAgentID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPListAgentProfiles, map[string]string{})

	resp, err := h.handleListAgentProfiles(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleListAgentProfiles_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPListAgentProfiles,
		Payload: json.RawMessage(`badpayload`),
	}

	resp, err := h.handleListAgentProfiles(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleUpdateAgentProfile_MissingProfileID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"name": "New Name",
	})

	resp, err := h.handleUpdateAgentProfile(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateAgentProfile_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPUpdateAgentProfile,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleUpdateAgentProfile(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- MCP Config handler tests ---

func TestHandleGetMcpConfig_MissingProfileID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPGetMcpConfig, map[string]string{})

	resp, err := h.handleGetMcpConfig(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleGetMcpConfig_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPGetMcpConfig,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleGetMcpConfig(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleUpdateMcpConfig_MissingProfileID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateMcpConfig, map[string]interface{}{
		"enabled": true,
	})

	resp, err := h.handleUpdateMcpConfig(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateMcpConfig_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPUpdateMcpConfig,
		Payload: json.RawMessage(`invalid`),
	}

	resp, err := h.handleUpdateMcpConfig(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- Task handler tests ---

func TestHandleMoveTask_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"workflow_id":      "wf-123",
		"workflow_step_id": "step-1",
	})

	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMoveTask_MissingWorkflowID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "task-1",
		"workflow_step_id": "step-1",
	})

	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMoveTask_MissingStepID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":     "task-1",
		"workflow_id": "wf-123",
	})

	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMoveTask_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPMoveTask,
		Payload: json.RawMessage(`invalid`),
	}

	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// TestLookupSession_NoPrimarySession_ReturnsNilNil pins the contract that a
// task with no primary session resolves to (nil, nil) — a legitimate "empty"
// state that lets handleMoveTask fall through to the idle-move path — rather
// than a backend error. Regression guard: lookupSession originally detected
// this case by substring-matching the repository's error message; when the
// repository switched to a wrapped taskrepo.ErrNoPrimarySession sentinel the
// substring stopped matching, so sessionless task moves were wrongly rejected
// as internal errors. Classifying via errors.Is keeps the two decoupled.
func TestLookupSession_NoPrimarySession_ReturnsNilNil(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	// Task created without an agent → no primary session row.
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sessionless task",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	session, lookupErr := h.lookupSession(ctx, task.ID)
	require.NoError(t, lookupErr, "no-primary-session must not surface as a backend error")
	assert.Nil(t, session, "no-primary-session must resolve to a nil session, not a value")
}

// recordingMessageQueuer captures QueueMessage calls for assertion.
type recordingMessageQueuer struct {
	calls []messagequeue.QueuedMessage
}

func (r *recordingMessageQueuer) QueueMessage(_ context.Context, sessionID, taskID, content, model, userID string, planMode bool, _ []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error) {
	msg := messagequeue.QueuedMessage{
		SessionID: sessionID,
		TaskID:    taskID,
		Content:   content,
		Model:     model,
		PlanMode:  planMode,
		QueuedBy:  userID,
	}
	r.calls = append(r.calls, msg)
	return &msg, nil
}

type pendingMoveRecordingQueuer struct {
	recordingMessageQueuer
	pendingSessionID string
	pendingMoves     []messagequeue.PendingMove
}

func (r *pendingMoveRecordingQueuer) SetPendingMove(_ context.Context, sessionID string, move *messagequeue.PendingMove) {
	r.pendingSessionID = sessionID
	if move != nil {
		r.pendingMoves = append(r.pendingMoves, *move)
	}
}

func (r *recordingMessageQueuer) SetPendingMove(_ context.Context, _ string, _ *messagequeue.PendingMove) {
}

// TakeQueued is a no-op stub — the unit tests below don't exercise rollback,
// they just exercise QueueMessage. Returning (nil, false) is consistent with
// "nothing to take", which is what the rollback path checks before logging.
func (r *recordingMessageQueuer) TakeQueued(_ context.Context, _ string) (*messagequeue.QueuedMessage, bool) {
	return nil, false
}

// TestQueueMoveTaskPrompt_NilQueueReturnsError ensures the call is safe (no panic)
// and surfaces a descriptive error so callers can fail fast instead of silently
// dropping the user-supplied prompt.
func TestQueueMoveTaskPrompt_NilQueueReturnsError(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}

	err := h.queueMoveTaskPrompt(context.Background(), "task-1", "session-1", "fix issues")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message queue")
}

// TestQueueMoveTaskPrompt_EmptySessionIDReturnsError ensures a missing session ID
// surfaces an error rather than silently no-op'ing — without a session there's
// nowhere to deliver the prompt.
func TestQueueMoveTaskPrompt_EmptySessionIDReturnsError(t *testing.T) {
	queue := &recordingMessageQueuer{}
	h := &Handlers{
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}

	err := h.queueMoveTaskPrompt(context.Background(), "task-1", "", "fix issues")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary session")
	assert.Empty(t, queue.calls, "queue must not be invoked without a session ID")
}

// TestQueueMoveTaskPrompt_QueuesWithExpectedFields verifies the happy-path
// invocation: the prompt is queued on the resolved session with the expected
// metadata (sender = "mcp-move-task", plan mode disabled, no model override).
func TestQueueMoveTaskPrompt_QueuesWithExpectedFields(t *testing.T) {
	queue := &recordingMessageQueuer{}
	h := &Handlers{
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}

	err := h.queueMoveTaskPrompt(context.Background(), "task-1", "session-99", "Please fix the failing test in foo_test.go")
	require.NoError(t, err)

	require.Len(t, queue.calls, 1)
	got := queue.calls[0]
	assert.Equal(t, "session-99", got.SessionID)
	assert.Equal(t, "task-1", got.TaskID)
	assert.Equal(t, "Please fix the failing test in foo_test.go", got.Content)
	assert.Equal(t, "mcp-move-task", got.QueuedBy)
	assert.False(t, got.PlanMode)
	assert.Equal(t, "", got.Model)
}

func TestHandleDeleteTask_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPDeleteTask, map[string]string{})

	resp, err := h.handleDeleteTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleDeleteTask_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPDeleteTask,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleDeleteTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleArchiveTask_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]string{})

	resp, err := h.handleArchiveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleArchiveTask_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPArchiveTask,
		Payload: json.RawMessage(`bad`),
	}

	resp, err := h.handleArchiveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleUpdateTaskState_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateTaskState, map[string]interface{}{
		"state": "in_progress",
	})

	resp, err := h.handleUpdateTaskState(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateTaskState_MissingState(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPUpdateTaskState, map[string]interface{}{
		"task_id": "task-1",
	})

	resp, err := h.handleUpdateTaskState(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateTask_PersistsState(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-update-state", Name: "Update State", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-update-state", WorkspaceID: "ws-update-state", Name: "Board", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-update-state", WorkspaceID: "ws-update-state", WorkflowID: "wf-update-state",
		Title: "Stateful", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id": "task-update-state",
		"state":   "COMPLETED",
	})

	resp, err := h.handleUpdateTask(ctx, msg)
	require.NoError(t, err)
	assert.NotEqual(t, ws.MessageTypeError, resp.Type)

	task, err := svc.GetTask(ctx, "task-update-state")
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateCompleted, task.State)
}

// TestHandleMoveTask_ActiveSessionWithoutPrompt_DefersMove pins the production
// bug where an agent on Work called move_task_kandev → Done without a prompt
// mid-turn. The immediate path hit validateMoveSessions (RUNNING session) and
// returned INTERNAL_ERROR. Active-session moves must always defer.
func TestHandleMoveTask_ActiveSessionWithoutPrompt_DefersMove(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-move", Name: "Move", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-move", WorkspaceID: "ws-move", Name: "Board", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-move", WorkspaceID: "ws-move", WorkflowID: "wf-move",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-move", TaskID: "task-move", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{
		taskSvc:      svc,
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}

	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "task-move",
		"workflow_id":      "wf-move",
		"workflow_step_id": "step-done",
		"position":         0,
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	assert.NotEqual(t, ws.MessageTypeError, resp.Type, "active-session move without prompt must not fail")

	require.Len(t, queue.pendingMoves, 1)
	assert.Equal(t, "step-done", queue.pendingMoves[0].WorkflowStepID)
	assert.Equal(t, "sess-move", queue.pendingSessionID)
	assert.Empty(t, queue.calls, "no hand-off prompt should be queued when prompt omitted")

	task, err := svc.GetTask(ctx, "task-move")
	require.NoError(t, err)
	assert.Equal(t, "step-work", task.WorkflowStepID, "deferred move must not apply immediately")
}

func TestMoveTaskErrorMessage_SanitizesClassifiedErrors(t *testing.T) {
	assert.Equal(t,
		"Move task conflicts with the current task or workflow state",
		moveTaskErrorMessage(fmt.Errorf("WIP limit exceeded for workflow step secret-step")),
	)
	assert.Equal(t,
		"Invalid move_task request",
		moveTaskErrorMessage(fmt.Errorf("workflow_step_id is required")),
	)
	assert.Equal(t,
		"Failed to move task",
		moveTaskErrorMessage(fmt.Errorf("database path /tmp/private failed")),
	)
}

func TestNormalizeTaskState_AcceptsCommonAliases(t *testing.T) {
	assert.Equal(t, v1.TaskStateCompleted, normalizeTaskState("complete"))
	assert.Equal(t, v1.TaskStateCompleted, normalizeTaskState("DONE"))
	assert.Equal(t, v1.TaskStateInProgress, normalizeTaskState("in_progress"))
	assert.Equal(t, v1.TaskStateTODO, normalizeTaskState("open"))
}

func TestHandleUpdateTaskState_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPUpdateTaskState,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleUpdateTaskState(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- Registration tests ---

func TestRegisterHandlers_NilDeps_DoesNotPanic(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log}
	d := ws.NewDispatcher()

	// Should not panic with nil config/task deps — handlers simply not registered.
	assert.NotPanics(t, func() { h.RegisterHandlers(d) })
}

func TestRegisterHandlers_ListExecutorsActionDispatches(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)

	msg := makeWSMessage(t, ws.ActionMCPListExecutors, map[string]interface{}{})
	resp, err := d.Dispatch(context.Background(), msg)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, ws.ActionMCPListExecutors, resp.Action)

	var payload dto.ListExecutorsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, len(payload.Executors), payload.Total)
	assert.Contains(t, executorIDs(payload.Executors), models.ExecutorIDLocal)
}

func executorIDs(executors []dto.ExecutorDTO) []string {
	ids := make([]string, 0, len(executors))
	for _, executor := range executors {
		ids = append(ids, executor.ID)
	}
	return ids
}

// --- Helper function tests ---

func TestUnmarshalStringField(t *testing.T) {
	t.Run("valid field", func(t *testing.T) {
		payload := json.RawMessage(`{"task_id":"abc-123"}`)
		val, err := unmarshalStringField(payload, "task_id")
		assert.NoError(t, err)
		assert.Equal(t, "abc-123", val)
	})

	t.Run("missing field returns empty", func(t *testing.T) {
		payload := json.RawMessage(`{"other":"value"}`)
		val, err := unmarshalStringField(payload, "task_id")
		assert.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("invalid json", func(t *testing.T) {
		payload := json.RawMessage(`not json`)
		_, err := unmarshalStringField(payload, "task_id")
		assert.Error(t, err)
	})

	t.Run("empty payload", func(t *testing.T) {
		payload := json.RawMessage(`{}`)
		val, err := unmarshalStringField(payload, "task_id")
		assert.NoError(t, err)
		assert.Equal(t, "", val)
	})
}
