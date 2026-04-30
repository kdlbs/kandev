package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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

func (r *recordingMessageQueuer) SetPendingMove(_ context.Context, _ string, _ *messagequeue.PendingMove) {
}

// TestQueueMoveTaskPrompt_NilQueueIsSafe ensures that providing a prompt with no
// configured message queue is handled gracefully (logged, no panic).
func TestQueueMoveTaskPrompt_NilQueueIsSafe(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}

	// Should not panic — the nil-queue branch short-circuits.
	h.queueMoveTaskPrompt(context.Background(), "task-1", "session-1", "fix issues")
}

// TestQueueMoveTaskPrompt_EmptySessionIDIsSafe ensures that a missing primary
// session is handled gracefully (logged, no panic, no queue call).
func TestQueueMoveTaskPrompt_EmptySessionIDIsSafe(t *testing.T) {
	queue := &recordingMessageQueuer{}
	h := &Handlers{
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}

	h.queueMoveTaskPrompt(context.Background(), "task-1", "", "fix issues")

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

	h.queueMoveTaskPrompt(context.Background(), "task-1", "session-99", "Please fix the failing test in foo_test.go")

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
