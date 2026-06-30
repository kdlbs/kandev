package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOrchestrator records calls to the SessionLauncher methods exercised by
// handleMessageTask. PromptTask returns a configurable error so the auto-resume
// path can be tested.
type fakeOrchestrator struct {
	mu sync.Mutex

	queue *messagequeue.Service

	promptCalls       []promptCall
	startCreatedCalls []startCreatedCall
	resumeCalls       int
	turnStartCalls    []turnStartCall
	onTurnStart       func(context.Context, string, string) error

	// Configurable: error returned by PromptTask. Cleared after first call so
	// the retry-after-resume path can succeed on the second call.
	promptErrFirst error
}

type promptCall struct {
	taskID, sessionID, prompt string
	dispatchOnly              bool
}
type startCreatedCall struct {
	taskID, sessionID, agentProfileID, prompt string
	skipMessageRecord                         bool
}
type turnStartCall struct {
	taskID, sessionID string
}

func (f *fakeOrchestrator) LaunchSession(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	return nil, nil
}

func (f *fakeOrchestrator) PromptTask(_ context.Context, taskID, sessionID, prompt, _ string, _ bool, _ []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promptCalls = append(f.promptCalls, promptCall{taskID: taskID, sessionID: sessionID, prompt: prompt, dispatchOnly: dispatchOnly})
	if f.promptErrFirst != nil {
		err := f.promptErrFirst
		f.promptErrFirst = nil
		return nil, err
	}
	return &orchestrator.PromptResult{}, nil
}

func (f *fakeOrchestrator) StartCreatedSession(_ context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, _, _ bool, _ []v1.MessageAttachment) (*executor.TaskExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCreatedCalls = append(f.startCreatedCalls, startCreatedCall{
		taskID:            taskID,
		sessionID:         sessionID,
		agentProfileID:    agentProfileID,
		prompt:            prompt,
		skipMessageRecord: skipMessageRecord,
	})
	return &executor.TaskExecution{SessionID: sessionID}, nil
}

func (f *fakeOrchestrator) ResumeTaskSession(_ context.Context, _, _ string) (*executor.TaskExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumeCalls++
	return &executor.TaskExecution{}, nil
}

func (f *fakeOrchestrator) ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) error {
	f.mu.Lock()
	f.turnStartCalls = append(f.turnStartCalls, turnStartCall{taskID: taskID, sessionID: sessionID})
	fn := f.onTurnStart
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, taskID, sessionID)
	}
	return nil
}

func (f *fakeOrchestrator) GetMessageQueue() *messagequeue.Service { return f.queue }

func newMessageTaskHandler(t *testing.T, svc *service.Service, taskRepo ...TaskRepository) (*Handlers, *fakeOrchestrator) {
	t.Helper()
	log := testLogger(t)
	orch := &fakeOrchestrator{queue: messagequeue.NewServiceMemory(log)}
	var repo TaskRepository
	if len(taskRepo) > 0 {
		repo = taskRepo[0]
	}
	h := &Handlers{
		taskSvc:         svc,
		taskRepo:        repo,
		sessionLauncher: orch,
		logger:          log.WithFields(),
	}
	return h, orch
}

type seedRepo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	UpdateTaskSessionState(context.Context, string, models.TaskSessionState, string) error
}

// seedTaskWithSession creates a workspace, workflow, target task with a primary
// session in the given state, and a separate sender task to attribute messages
// to. Returns (sender task, target task, target session). Most tests just need
// the sender ID for the sender_task_id payload field.
func seedTaskWithSession(t *testing.T, svc *service.Service, repo seedRepo, state models.TaskSessionState) (*models.Task, *models.Task, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	target, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Target task",
	})
	require.NoError(t, err)
	sender, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sender task",
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:             "sess-1",
		TaskID:         target.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      true,
		State:          models.TaskSessionStateCreated,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))
	if state != models.TaskSessionStateCreated {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, sess.ID, state, ""))
	}
	loaded, err := svc.GetTaskSession(ctx, sess.ID)
	require.NoError(t, err)
	return sender, target, loaded
}

// senderPayload returns the standard payload shape sent by the MCP server
// (agentctl injects sender_task_id and sender_session_id). Helper keeps test
// bodies focused on the behaviour under test.
func senderPayload(targetTaskID, prompt, senderTaskID string) map[string]interface{} {
	return map[string]interface{}{
		"task_id":           targetTaskID,
		"prompt":            prompt,
		"sender_task_id":    senderTaskID,
		"sender_session_id": "sender-sess-1",
	}
}

func TestHandleMessageTask_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"prompt": "hello",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMessageTask_MissingPrompt(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": "task-1",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMessageTask_BadPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPMessageTask,
		Payload: json.RawMessage(`{not-json`),
	}
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleMessageTask_RunningSession_Queues(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "follow-up message", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "queued", payload["status"])
	assert.Equal(t, sess.ID, payload["session_id"])

	// Message landed in the queue, with the <kandev-system> attribution wrapper
	// and structured sender metadata so the drain path can write a Message row
	// the UI can render with a sender badge.
	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count)
	entry := status.Entries[0]
	assert.Contains(t, entry.Content, "follow-up message")
	assert.Contains(t, entry.Content, "<kandev-system>")
	assert.Contains(t, entry.Content, "Sender task")
	assert.Equal(t, sender.ID, entry.Metadata["sender_task_id"])
	assert.Equal(t, "Sender task", entry.Metadata["sender_task_title"])
	assert.Equal(t, "sender-sess-1", entry.Metadata["sender_session_id"])
	assert.Empty(t, orch.promptCalls)
	assert.Empty(t, orch.startCreatedCalls)
}

func TestHandleMessageTask_QueueFull_ReturnsStructuredError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	// Saturate the receiver's queue.
	for i := 0; i < messagequeue.DefaultMaxPerSession; i++ {
		_, err := orch.queue.QueueMessageWithMetadata(context.Background(), sess.ID, target.ID,
			"prefill", "", "agent", false, nil, nil)
		require.NoError(t, err)
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "overflow message", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeError, resp.Type)

	var errResp ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &errResp))
	assert.Equal(t, "queue_full", errResp.Code)
	assert.Equal(t, "queue_full", errResp.Details["error"])
	assert.EqualValues(t, messagequeue.DefaultMaxPerSession, errResp.Details["queue_size"])
	assert.EqualValues(t, messagequeue.DefaultMaxPerSession, errResp.Details["max"])
	assert.Equal(t, "next_turn", errResp.Details["retry_after"])
	queued, ok := errResp.Details["queued_messages"].([]interface{})
	require.True(t, ok, "queued_messages should be an array")
	assert.Len(t, queued, messagequeue.DefaultMaxPerSession)
}

func TestHandleMessageTask_WaitingForInput_PromptsAgent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "next instruction", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, target.ID, orch.promptCalls[0].taskID)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
	// The prompt sent to the agent is wrapped with the attribution block so the
	// agent can identify the sender on this turn (and on resume).
	assert.Contains(t, orch.promptCalls[0].prompt, "next instruction")
	assert.Contains(t, orch.promptCalls[0].prompt, "<kandev-system>")
	// MCP message_task uses dispatch-only mode so the tool returns once the
	// prompt is accepted instead of blocking for the entire target turn.
	assert.True(t, orch.promptCalls[0].dispatchOnly, "MCP path must use dispatch-only mode")
	assert.Zero(t, orch.resumeCalls)

	// Prompt is recorded as a user message so it shows in the receiving task's chat.
	messages, err := svc.ListMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "next instruction")
	assert.Equal(t, models.MessageAuthorUser, messages[0].AuthorType)
	// Sender metadata persists on the recorded row.
	assert.Equal(t, sender.ID, messages[0].Metadata["sender_task_id"])
	assert.Equal(t, "Sender task", messages[0].Metadata["sender_task_title"])
}

func TestHandleMessageTask_WaitingForInput_FiresTurnStart(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, taskID, sessionID string) error {
		assert.Equal(t, target.ID, taskID)
		assert.Equal(t, sess.ID, sessionID)
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
		updatedTask.WorkflowStepID = "step-in-progress"
		return repo.UpdateTask(ctx, updatedTask)
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "review follow-up", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.turnStartCalls, 1)
	assert.Equal(t, target.ID, orch.turnStartCalls[0].taskID)
	assert.Equal(t, sess.ID, orch.turnStartCalls[0].sessionID)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
	assert.Equal(t, "step-in-progress", updatedTask.WorkflowStepID)

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
}

func TestHandleMessageTask_WaitingForInput_UsesSessionSelectedByTurnStart(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	replacement := &models.TaskSession{
		ID:             "sess-2",
		TaskID:         target.ID,
		AgentProfileID: "agent-profile-2",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      false,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, replacement))

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.State = models.TaskSessionStateCompleted
		oldSession.IsPrimary = false
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacement.ID))
		return nil
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "handoff after switch", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	assert.Equal(t, replacement.ID, payload["session_id"])

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, replacement.ID, orch.promptCalls[0].sessionID)

	oldMessages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, oldMessages)
	newMessages, err := svc.ListMessages(ctx, replacement.ID)
	require.NoError(t, err)
	require.Len(t, newMessages, 1)
	assert.Contains(t, newMessages[0].Content, "handoff after switch")
}

func TestHandleMessageTask_CompletedSessionWithoutSwitch_PromptsSameSession(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCompleted)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "follow up completed", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	assert.Equal(t, sess.ID, payload["session_id"])

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)

	messages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "follow up completed")
	assert.Equal(t, sender.ID, messages[0].Metadata["sender_task_id"])
}

func TestHandleMessageTask_CompletedSession_UsesSessionSelectedByTurnStart(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCompleted)

	replacement := &models.TaskSession{
		ID:             "sess-2",
		TaskID:         target.ID,
		AgentProfileID: "agent-profile-2",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      false,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, replacement))

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.IsPrimary = false
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacement.ID))
		return nil
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "handoff from completed", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	assert.Equal(t, replacement.ID, payload["session_id"])

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, replacement.ID, orch.promptCalls[0].sessionID)

	oldMessages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, oldMessages)
	newMessages, err := svc.ListMessages(ctx, replacement.ID)
	require.NoError(t, err)
	require.Len(t, newMessages, 1)
	assert.Contains(t, newMessages[0].Content, "handoff from completed")
	assert.Equal(t, sender.ID, newMessages[0].Metadata["sender_task_id"])
}

func TestHandleMessageTask_WaitingForInputCompletedWithoutPrimarySwitchRejects(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.State = models.TaskSessionStateCompleted
		oldSession.IsPrimary = true
		return repo.UpdateTaskSession(ctx, oldSession)
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "no handoff", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assert.Contains(t, string(resp.Payload), "marked completed")

	assert.Empty(t, orch.promptCalls)
	messages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestHandleMessageTask_CreatedSessionStartsAfterTurnStartChangesState(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, _, sessionID string) error {
		require.Equal(t, sess.ID, sessionID)
		return repo.UpdateTaskSessionState(ctx, sess.ID, models.TaskSessionStateWaitingForInput, "")
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "start after trigger", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "started", payload["status"])
	assert.Equal(t, sess.ID, payload["session_id"])

	require.Len(t, orch.startCreatedCalls, 1)
	assert.Equal(t, sess.ID, orch.startCreatedCalls[0].sessionID)
	assert.Empty(t, orch.promptCalls)
}

func TestHandleMessageTask_TurnStartErrorRejectsAndRestoresReview(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.WorkflowStepID = "step-in-progress"
		require.NoError(t, repo.UpdateTask(ctx, updatedTask))
		return errors.New("turn start failed")
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "cannot send", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)
	assert.Empty(t, orch.promptCalls)
	messages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestHandleMessageTask_DispatchErrorRestoresReview(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.WorkflowStepID = "step-in-progress"
		return repo.UpdateTask(ctx, updatedTask)
	}
	orch.promptErrFirst = errors.New("send failed")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "fails during dispatch", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)
	require.Len(t, orch.promptCalls, 1)
}

func TestHandleMessageTask_PromptFailsWithExecutionNotFound_AutoResumes(t *testing.T) {
	// Wrapped in synctest so the WaitForSessionReady poll's time.After advances
	// virtually instead of blocking the test for ~1s of real time. Matches
	// CLAUDE.md guidance to prefer synctest over time.Sleep-based waits.
	synctest.Test(t, func(t *testing.T) {
		svc, repo := newTestTaskService(t)
		sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

		h, orch := newMessageTaskHandler(t, svc)
		orch.promptErrFirst = executor.ErrExecutionNotFound

		msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "retry me", sender.ID))
		resp, err := h.handleMessageTask(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		assert.Len(t, orch.promptCalls, 2, "should retry prompt after resume")
		assert.Equal(t, 1, orch.resumeCalls)
	})
}

func TestHandleMessageTask_CreatedSession_StartsAgent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "kick off the work", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "started", payload["status"])

	require.Len(t, orch.startCreatedCalls, 1)
	c := orch.startCreatedCalls[0]
	assert.Equal(t, target.ID, c.taskID)
	assert.Equal(t, sess.ID, c.sessionID)
	assert.Equal(t, "agent-profile-1", c.agentProfileID)
	// The prompt forwarded to the agent is the wrapped string (so the agent
	// sees the attribution block both at start time and on ACP resume).
	assert.Contains(t, c.prompt, "kick off the work")
	assert.Contains(t, c.prompt, "<kandev-system>")
	// We record the user message ourselves with sender metadata, so
	// StartCreatedSession must skip its own initial-message recording —
	// otherwise the chat would gain an unattributed duplicate row.
	assert.True(t, c.skipMessageRecord, "skipMessageRecord must be true so the sender-attributed message is the only one recorded")

	messages, err := svc.ListMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "kick off the work")
	assert.Equal(t, sender.ID, messages[0].Metadata["sender_task_id"])
}

func TestHandleMessageTask_FailedSession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateFailed)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "hello", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleMessageTask_CancelledSession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCancelled)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "hello", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleMessageTask_NoPrimarySession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	target, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sessionless task",
	})
	require.NoError(t, err)
	sender, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sender task",
	})
	require.NoError(t, err)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "hello", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)

	// The task exists but has no session — must report "no active session", not
	// the generic "task not found" from the task-existence check.
	payload := string(resp.Payload)
	assert.Contains(t, payload, "no active session")
	assert.NotContains(t, payload, "task not found")
}

func TestHandleMessageTask_NonexistentTask_ReportsTaskNotFound(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	// Only the sender exists; the target task_id below was never created (mimics
	// passing a truncated UUID prefix).
	sender, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sender task",
	})
	require.NoError(t, err)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload("00000000-0000-0000-0000-000000000000", "hello", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)

	// Must surface the task-not-found error, NOT the misleading no-session error.
	payload := string(resp.Payload)
	assert.Contains(t, payload, "target task not found")
	assert.NotContains(t, payload, "no session")
}

// --- sender attribution validation ---

func TestHandleMessageTask_MissingSenderTaskID_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": target.ID,
		"prompt":  "hello",
		// sender_task_id intentionally omitted — old MCP server, malicious caller, etc.
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMessageTask_SelfMessage_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "hello", target.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	// No message recorded.
	messages, err := svc.ListMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestHandleMessageTask_UnknownSenderTask_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "hello", "00000000-0000-0000-0000-000000000000"))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)

	messages, err := svc.ListMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}
