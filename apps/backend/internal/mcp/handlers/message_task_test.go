package handlers

import (
	"context"
	"encoding/json"
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

	// Configurable: error returned by PromptTask. Cleared after first call so
	// the retry-after-resume path can succeed on the second call.
	promptErrFirst error
}

type promptCall struct {
	taskID, sessionID, prompt string
}
type startCreatedCall struct {
	taskID, sessionID, agentProfileID, prompt string
	skipMessageRecord                         bool
}

func (f *fakeOrchestrator) LaunchSession(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	return nil, nil
}

func (f *fakeOrchestrator) PromptTask(_ context.Context, taskID, sessionID, prompt, _ string, _ bool, _ []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promptCalls = append(f.promptCalls, promptCall{taskID, sessionID, prompt})
	if f.promptErrFirst != nil {
		err := f.promptErrFirst
		f.promptErrFirst = nil
		return nil, err
	}
	return &orchestrator.PromptResult{}, nil
}

func (f *fakeOrchestrator) StartCreatedSession(_ context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, _ bool, _ []v1.MessageAttachment) (*executor.TaskExecution, error) {
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

func (f *fakeOrchestrator) GetMessageQueue() *messagequeue.Service { return f.queue }

func newMessageTaskHandler(t *testing.T, svc *service.Service) (*Handlers, *fakeOrchestrator) {
	t.Helper()
	log := testLogger(t)
	orch := &fakeOrchestrator{queue: messagequeue.NewService(log)}
	h := &Handlers{
		taskSvc:         svc,
		sessionLauncher: orch,
		logger:          log.WithFields(),
	}
	return h, orch
}

// seedTaskWithSession creates a workspace, workflow, task, and primary session
// in the given state. Returns the task and session models.
func seedTaskWithSession(t *testing.T, svc *service.Service, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	UpdateTaskSessionState(context.Context, string, models.TaskSessionState, string) error
}, state models.TaskSessionState) (*models.Task, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Target task",
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:             "sess-1",
		TaskID:         task.ID,
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
	return task, loaded
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
	task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "follow-up message",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "queued", payload["status"])
	assert.Equal(t, sess.ID, payload["session_id"])

	// Message landed in the queue.
	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.True(t, status.IsQueued)
	assert.Equal(t, "follow-up message", status.Message.Content)
	assert.Empty(t, orch.promptCalls)
	assert.Empty(t, orch.startCreatedCalls)
}

func TestHandleMessageTask_WaitingForInput_PromptsAgent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "next instruction",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, task.ID, orch.promptCalls[0].taskID)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
	assert.Equal(t, "next instruction", orch.promptCalls[0].prompt)
	assert.Zero(t, orch.resumeCalls)

	// Prompt is recorded as a user message so it shows in the receiving task's chat.
	messages, err := svc.ListMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "next instruction", messages[0].Content)
	assert.Equal(t, models.MessageAuthorUser, messages[0].AuthorType)
}

func TestHandleMessageTask_PromptFailsWithExecutionNotFound_AutoResumes(t *testing.T) {
	// Wrapped in synctest so the WaitForSessionReady poll's time.After advances
	// virtually instead of blocking the test for ~1s of real time. Matches
	// CLAUDE.md guidance to prefer synctest over time.Sleep-based waits.
	synctest.Test(t, func(t *testing.T) {
		svc, repo := newTestTaskService(t)
		task, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

		h, orch := newMessageTaskHandler(t, svc)
		orch.promptErrFirst = executor.ErrExecutionNotFound

		msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
			"task_id": task.ID,
			"prompt":  "retry me",
		})
		resp, err := h.handleMessageTask(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		assert.Len(t, orch.promptCalls, 2, "should retry prompt after resume")
		assert.Equal(t, 1, orch.resumeCalls)
	})
}

func TestHandleMessageTask_CreatedSession_StartsAgent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "kick off the work",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "started", payload["status"])

	require.Len(t, orch.startCreatedCalls, 1)
	c := orch.startCreatedCalls[0]
	assert.Equal(t, task.ID, c.taskID)
	assert.Equal(t, sess.ID, c.sessionID)
	assert.Equal(t, "agent-profile-1", c.agentProfileID)
	assert.Equal(t, "kick off the work", c.prompt)
	// skipMessageRecord must be false so postLaunchCreated → recordInitialMessage
	// writes the prompt to the receiving task's chat.
	assert.False(t, c.skipMessageRecord, "skipMessageRecord must be false so the prompt is recorded in chat")
}

func TestHandleMessageTask_FailedSession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateFailed)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "hello",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleMessageTask_CancelledSession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCancelled)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "hello",
	})
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleMessageTask_NoPrimarySession_Rejects(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Sessionless task",
	})
	require.NoError(t, err)

	h, _ := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID,
		"prompt":  "hello",
	})
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}
