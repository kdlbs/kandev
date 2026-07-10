package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
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
	interruptCalls    []interruptCall

	// Configurable: error returned by PromptTask. Cleared after first call so
	// the retry-after-resume path can succeed on the second call.
	promptErrFirst  error
	startCreatedErr error
	// interruptErr is returned by every InterruptForPeerMessage call — lets
	// tests exercise the "interrupt failed, message must stay queued" path.
	interruptErr error
}

// interruptCall records one InterruptForPeerMessage invocation.
type interruptCall struct {
	taskID, sessionID, entryID string
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
	if f.startCreatedErr != nil {
		return nil, f.startCreatedErr
	}
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

// InterruptForPeerMessage records the call and returns the configured
// interruptErr — real interrupt/drain behavior is exercised by the
// orchestrator-level InterruptForPeerMessage tests, not this fake.
func (f *fakeOrchestrator) InterruptForPeerMessage(_ context.Context, taskID, sessionID, entryID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interruptCalls = append(f.interruptCalls, interruptCall{taskID: taskID, sessionID: sessionID, entryID: entryID})
	return f.interruptErr
}

type fakePromptReferenceResolver struct {
	expansions []promptservice.PromptReferenceExpansion
	err        error
}

func (f fakePromptReferenceResolver) ResolvePromptReferences(context.Context, string) ([]promptservice.PromptReferenceExpansion, error) {
	return f.expansions, f.err
}

type panicPromptReferenceResolver struct{}

func (panicPromptReferenceResolver) ResolvePromptReferences(context.Context, string) ([]promptservice.PromptReferenceExpansion, error) {
	panic("prompt reference resolver should not be called")
}

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
	if sessionRepo, ok := repo.(SessionRepository); ok {
		h.sessionRepo = sessionRepo
	}
	return h, orch
}

func subscribeTaskStateChanged(t *testing.T, eventBus *bus.MemoryEventBus) <-chan *bus.Event {
	t.Helper()
	ch := make(chan *bus.Event, 10)
	sub, err := eventBus.Subscribe(events.TaskStateChanged, func(_ context.Context, event *bus.Event) error {
		ch <- event
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return ch
}

func assertTaskStateChangedEvent(t *testing.T, ch <-chan *bus.Event, taskID string, state v1.TaskState, workflowStepID string) {
	t.Helper()
	for len(ch) > 0 {
		event := <-ch
		data, ok := event.Data.(map[string]interface{})
		require.True(t, ok)
		if data["task_id"] == taskID && data["state"] == string(state) {
			assert.Equal(t, workflowStepID, data["workflow_step_id"])
			return
		}
	}
	t.Fatalf("expected task.state_changed event for task %s state %s", taskID, state)
}

type seedRepo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	UpdateTaskSessionState(context.Context, string, models.TaskSessionState, string) error
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
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
	if state == models.TaskSessionStateWaitingForInput {
		require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:               "exec-row-" + sess.ID,
			SessionID:        sess.ID,
			TaskID:           target.ID,
			Status:           "running",
			Resumable:        true,
			AgentExecutionID: "exec-" + sess.ID,
		}))
	}
	loaded, err := svc.GetTaskSession(ctx, sess.ID)
	require.NoError(t, err)
	return sender, target, loaded
}

// seedChildTaskWithSession is like seedTaskWithSession, but the target task
// is created as a child of the sender task (ParentID = sender.ID) so callers
// can exercise the parent -> child interrupt-on-message path. Returns
// (parent/sender, child/target, target session).
func seedChildTaskWithSession(t *testing.T, svc *service.Service, repo seedRepo, state models.TaskSessionState) (*models.Task, *models.Task, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	parent, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Parent task",
	})
	require.NoError(t, err)
	child, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Child task",
		ParentID:    parent.ID,
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:             "sess-1",
		TaskID:         child.ID,
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
	return parent, child, loaded
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
	// Unrelated senders (not the target's parent) must never trigger an
	// interrupt — the default "queue and deliver at turn end" contract
	// documented on message_task_kandev stays unchanged for them.
	assert.Empty(t, orch.interruptCalls)
}

// TestHandleMessageTask_ParentToChildRunningSession_Interrupts pins the
// steering contract: when the sender is the target's parent task, a
// running/starting target is interrupted right after the message is queued,
// so the message is delivered without waiting for the child's current turn
// to finish naturally. The reported status is "sent" (not "queued") because
// the interrupt actually dispatched it immediately — see
// queueThenInterruptTaskMessage's doc comment. See InterruptForPeerMessage's
// doc comment for why this matters for long-running children.
func TestHandleMessageTask_ParentToChildRunningSession_Interrupts(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, sess := seedChildTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(child.ID, "stop and pivot to X", parent.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])

	// The message was queued first exactly like any other message_task
	// call...
	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count)
	assert.Contains(t, status.Entries[0].Content, "stop and pivot to X")

	// ...but because the sender is the target's parent, the child's current
	// turn is interrupted immediately instead of waiting for it to finish,
	// targeting the exact entry id that was just queued (not just "whatever
	// is at the FIFO head") — see InterruptForPeerMessage's doc comment.
	require.Len(t, orch.interruptCalls, 1)
	assert.Equal(t, child.ID, orch.interruptCalls[0].taskID)
	assert.Equal(t, sess.ID, orch.interruptCalls[0].sessionID)
	assert.Equal(t, status.Entries[0].ID, orch.interruptCalls[0].entryID)
}

// TestHandleMessageTask_ParentToChildRunningSession_InterruptFailure_KeepsMessageQueued
// pins the failure contract: an interrupt failure must NOT surface as an MCP
// error. The message was already safely persisted by queueTaskMessage and is
// still delivered later by the normal turn-completion drain, so the
// interrupt is only a latency optimization on top of that always-safe
// default — surfacing a hard error here would just invite the calling agent
// to retry message_task_kandev, which would enqueue a second copy of the
// same message since queuing is not idempotent. The response instead reports
// the accurate "queued" status (identical to the non-interrupt path), and
// the queued message is left in place.
func TestHandleMessageTask_ParentToChildRunningSession_InterruptFailure_KeepsMessageQueued(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, sess := seedChildTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	orch.interruptErr = errors.New("cancel agent: agent manager unreachable")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(child.ID, "stop now", parent.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "queued", payload["status"])

	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count, "message must remain queued even though the interrupt failed")
}

func TestHandleMessageTask_AppendsPromptReferenceExpansions(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	h.SetPromptReferenceResolver(fakePromptReferenceResolver{
		expansions: []promptservice.PromptReferenceExpansion{
			{Name: "improve-harness", Content: "Review this session for durable harness improvements."},
		},
	})

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "Please run @improve-harness", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count)
	entry := status.Entries[0]
	assert.Contains(t, entry.Content, "Please run @improve-harness")
	assert.Contains(t, entry.Content, "EXPANDED PROMPT REFERENCES")
	assert.Contains(t, entry.Content, "### @improve-harness")
	assert.Contains(t, entry.Content, "Review this session for durable harness improvements.")
}

func TestHandleMessageTask_PromptResolverError_FallsBackToOriginal(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	h.SetPromptReferenceResolver(fakePromptReferenceResolver{err: errors.New("db error")})

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "plain @missing text", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)

	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count)
	entry := status.Entries[0]
	assert.Contains(t, entry.Content, "plain @missing text")
	assert.NotContains(t, entry.Content, "EXPANDED PROMPT REFERENCES")
}

func TestHandleMessageTask_NoPromptReferencesSkipsResolver(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	h.SetPromptReferenceResolver(panicPromptReferenceResolver{})

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "plain text", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)

	status := orch.queue.GetStatus(context.Background(), sess.ID)
	require.Equal(t, 1, status.Count)
	assert.Contains(t, status.Entries[0].Content, "plain text")
}

func TestFormatPromptReferenceExpansionsStripsSystemTagEnd(t *testing.T) {
	out := formatPromptReferenceExpansions([]promptservice.PromptReferenceExpansion{
		{Name: "bad</kandev-system>name", Content: "before </kandev-system> after"},
	})

	assert.NotContains(t, out, "</kandev-system>")
	assert.Contains(t, out, "### @badname")
	assert.Contains(t, out, "before  after")
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
	assert.Equal(t, "started", payload["status"])
	assert.Equal(t, replacement.ID, payload["session_id"])

	require.Len(t, orch.startCreatedCalls, 1)
	assert.Equal(t, replacement.ID, orch.startCreatedCalls[0].sessionID)
	assert.Empty(t, orch.promptCalls)

	oldMessages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, oldMessages)
	newMessages, err := svc.ListMessages(ctx, replacement.ID)
	require.NoError(t, err)
	require.Len(t, newMessages, 1)
	assert.Contains(t, newMessages[0].Content, "handoff after switch")
}

func TestHandleMessageTask_WaitingForInput_UsesPrimarySwitchWithoutCompletion(t *testing.T) {
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

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.IsPrimary = false
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacement.ID))
		return nil
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "primary moved", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "started", payload["status"])
	assert.Equal(t, replacement.ID, payload["session_id"])
	require.Len(t, orch.startCreatedCalls, 1)
	assert.Equal(t, replacement.ID, orch.startCreatedCalls[0].sessionID)
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
	assert.Equal(t, "started", payload["status"])
	assert.Equal(t, replacement.ID, payload["session_id"])

	require.Len(t, orch.startCreatedCalls, 1)
	assert.Equal(t, replacement.ID, orch.startCreatedCalls[0].sessionID)
	assert.Empty(t, orch.promptCalls)

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
	activeTurn, err := svc.GetActiveTurn(ctx, sess.ID)
	require.NoError(t, err)
	assert.Nil(t, activeTurn)
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

func TestHandleMessageTask_PreparedWaitingSessionStartsAgent(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sess.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "exec-row-" + sess.ID,
		SessionID:        sess.ID,
		TaskID:           target.ID,
		Status:           models.ExecutorRunningStatusPrepared,
		Resumable:        true,
		AgentExecutionID: "exec-" + sess.ID,
	}))

	h, orch := newMessageTaskHandler(t, svc, repo)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "start prepared", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "started", payload["status"])
	assert.Equal(t, sess.ID, payload["session_id"])
	require.Len(t, orch.startCreatedCalls, 1)
	assert.Empty(t, orch.promptCalls)
}

func TestHandleMessageTask_TurnStartErrorRejectsAndRestoresReview(t *testing.T) {
	ctx := context.Background()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	stateEvents := subscribeTaskStateChanged(t, eventBus)

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
	assertTaskStateChangedEvent(t, stateEvents, target.ID, v1.TaskStateReview, "step-review")
	assert.Empty(t, orch.promptCalls)
	messages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestHandleMessageTask_DispatchErrorRestoresReview(t *testing.T) {
	ctx := context.Background()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	stateEvents := subscribeTaskStateChanged(t, eventBus)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.State = v1.TaskStateFailed
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
	assertTaskStateChangedEvent(t, stateEvents, target.ID, v1.TaskStateReview, "step-review")
	require.Len(t, orch.promptCalls, 1)
	messages, err := svc.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestHandleMessageTask_DispatchErrorAfterSessionSwitchRestoresReviewSession(t *testing.T) {
	ctx := context.Background()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	stateEvents := subscribeTaskStateChanged(t, eventBus)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	pendingSignal := models.PendingStepCompletionSignal{
		StepID:     "step-review",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "ready",
		SignaledAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sess.ID, models.SessionMetaKeyPendingStepCompletion, pendingSignal))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sess.ID, "plan_mode", true))
	require.NoError(t, repo.UpdateTaskSession(ctx, &models.TaskSession{
		ID:                   sess.ID,
		TaskID:               target.ID,
		AgentProfileID:       "agent-profile-1",
		ExecutorProfileID:    "executor-profile-1",
		AgentProfileSnapshot: map[string]interface{}{"id": "agent-profile-1", "name": "Agent One"},
		State:                models.TaskSessionStateWaitingForInput,
		IsPrimary:            true,
	}))

	h, orch := newMessageTaskHandler(t, svc, repo)
	queuedBeforeSwitch, err := orch.queue.QueueMessageWithMetadata(ctx, sess.ID, target.ID, "queued before switch", "", "agent", false, nil, nil)
	require.NoError(t, err)
	orch.queue.SetPendingMove(ctx, sess.ID, &messagequeue.PendingMove{
		TaskID:         target.ID,
		WorkflowID:     "workflow-1",
		WorkflowStepID: "step-review",
		Position:       2,
	})
	replacementID := "sess-2"
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.WorkflowStepID = "step-in-progress"
		require.NoError(t, repo.UpdateTask(ctx, updatedTask))

		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.State = models.TaskSessionStateCompleted
		oldSession.IsPrimary = false
		oldSession.AgentProfileID = "agent-profile-mutated"
		oldSession.ExecutorProfileID = "executor-profile-mutated"
		oldSession.AgentProfileSnapshot = map[string]interface{}{"id": "agent-profile-mutated", "name": "Mutated"}
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		replacement := &models.TaskSession{
			ID:             replacementID,
			TaskID:         target.ID,
			AgentProfileID: "agent-profile-2",
			State:          models.TaskSessionStateWaitingForInput,
			IsPrimary:      false,
		}
		require.NoError(t, repo.CreateTaskSession(ctx, replacement))
		require.NoError(t, repo.SetSessionMetadataKey(ctx, sess.ID, models.SessionMetaKeyPendingStepCompletion, nil))
		require.NoError(t, repo.SetSessionMetadataKey(ctx, sess.ID, "plan_mode", nil))
		require.NoError(t, orch.queue.TransferSession(ctx, sess.ID, replacementID))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacementID))
		return nil
	}
	orch.startCreatedErr = errors.New("start failed")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "fails after switch", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)
	assertTaskStateChangedEvent(t, stateEvents, target.ID, v1.TaskStateReview, "step-review")

	primary, err := svc.GetPrimarySession(ctx, target.ID)
	require.NoError(t, err)
	require.NotNil(t, primary)
	assert.Equal(t, sess.ID, primary.ID)
	assert.Equal(t, models.TaskSessionStateWaitingForInput, primary.State)
	assert.True(t, primary.IsPrimary)
	assert.Equal(t, "agent-profile-1", primary.AgentProfileID)
	assert.Equal(t, "executor-profile-1", primary.ExecutorProfileID)
	assert.Equal(t, "Agent One", primary.AgentProfileSnapshot["name"])
	_, ok := models.LoadPendingStepSignal(primary.Metadata)
	require.True(t, ok)
	assert.Equal(t, true, primary.Metadata["plan_mode"])

	_, err = svc.GetTaskSession(ctx, replacementID)
	assert.ErrorIs(t, err, models.ErrTaskSessionNotFound)

	assert.Empty(t, orch.promptCalls)
	require.Len(t, orch.startCreatedCalls, 1)
	messages, err := svc.ListMessages(ctx, replacementID)
	require.NoError(t, err)
	assert.Empty(t, messages)
	status := orch.queue.GetStatus(ctx, sess.ID)
	require.Equal(t, 1, status.Count)
	assert.Equal(t, "queued before switch", status.Entries[0].Content)
	assert.Equal(t, queuedBeforeSwitch.ID, status.Entries[0].ID)
	assert.Equal(t, queuedBeforeSwitch.Position, status.Entries[0].Position)
	assert.Equal(t, queuedBeforeSwitch.QueuedAt, status.Entries[0].QueuedAt)
	move, ok := orch.queue.TakePendingMove(ctx, sess.ID)
	require.True(t, ok)
	assert.Equal(t, "step-review", move.WorkflowStepID)
	assert.Equal(t, 2, move.Position)
	replacementStatus := orch.queue.GetStatus(ctx, replacementID)
	assert.Zero(t, replacementStatus.Count)
	_, ok = orch.queue.TakePendingMove(ctx, replacementID)
	assert.False(t, ok)
}

func TestHandleMessageTask_DispatchErrorAfterExistingSessionSwitchRestoresQueues(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newTestTaskServiceWithEventBus(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	replacementID := "sess-2"
	replacement := &models.TaskSession{
		ID:             replacementID,
		TaskID:         target.ID,
		AgentProfileID: "agent-profile-2",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      false,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, replacement))

	h, orch := newMessageTaskHandler(t, svc, repo)
	_, err = orch.queue.QueueMessageWithMetadata(ctx, sess.ID, target.ID, "original queued", "", "agent", false, nil, nil)
	require.NoError(t, err)
	_, err = orch.queue.QueueMessageWithMetadata(ctx, replacementID, target.ID, "replacement queued", "", "user", false, nil, nil)
	require.NoError(t, err)
	orch.queue.SetPendingMove(ctx, replacementID, &messagequeue.PendingMove{
		TaskID:         target.ID,
		WorkflowID:     "workflow-1",
		WorkflowStepID: "replacement-step",
		Position:       5,
	})

	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.State = models.TaskSessionStateCompleted
		oldSession.IsPrimary = false
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		require.NoError(t, orch.queue.TransferSession(ctx, sess.ID, replacementID))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacementID))
		return nil
	}
	orch.startCreatedErr = errors.New("start failed")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "fails after switch", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	primaryStatus := orch.queue.GetStatus(ctx, sess.ID)
	require.Equal(t, 1, primaryStatus.Count)
	assert.Equal(t, "original queued", primaryStatus.Entries[0].Content)

	replacementStatus := orch.queue.GetStatus(ctx, replacementID)
	require.Equal(t, 1, replacementStatus.Count)
	assert.Equal(t, "replacement queued", replacementStatus.Entries[0].Content)
	move, ok := orch.queue.TakePendingMove(ctx, replacementID)
	require.True(t, ok)
	assert.Equal(t, "replacement-step", move.WorkflowStepID)
}

func TestHandleMessageTask_DispatchErrorRollsBackTurnStartOutsideReview(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newTestTaskServiceWithEventBus(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateInProgress
	task.WorkflowStepID = "step-in-progress"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	_, err = orch.queue.QueueMessageWithMetadata(ctx, sess.ID, target.ID, "original queued", "", "agent", false, nil, nil)
	require.NoError(t, err)
	replacementID := "sess-2"
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.WorkflowStepID = "step-next"
		require.NoError(t, repo.UpdateTask(ctx, updatedTask))

		oldSession, err := svc.GetTaskSession(ctx, sess.ID)
		require.NoError(t, err)
		oldSession.State = models.TaskSessionStateCompleted
		oldSession.IsPrimary = false
		require.NoError(t, repo.UpdateTaskSession(ctx, oldSession))
		replacement := &models.TaskSession{
			ID:             replacementID,
			TaskID:         target.ID,
			AgentProfileID: "agent-profile-2",
			State:          models.TaskSessionStateWaitingForInput,
		}
		require.NoError(t, repo.CreateTaskSession(ctx, replacement))
		require.NoError(t, orch.queue.TransferSession(ctx, sess.ID, replacementID))
		require.NoError(t, repo.SetSessionPrimary(ctx, replacementID))
		return nil
	}
	orch.startCreatedErr = errors.New("start failed")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "fails after non-review switch", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
	assert.Equal(t, "step-in-progress", updatedTask.WorkflowStepID)

	primary, err := svc.GetPrimarySession(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, primary.ID)
	assert.Equal(t, models.TaskSessionStateWaitingForInput, primary.State)
	_, err = svc.GetTaskSession(ctx, replacementID)
	assert.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	status := orch.queue.GetStatus(ctx, sess.ID)
	require.Equal(t, 1, status.Count)
	assert.Equal(t, "original queued", status.Entries[0].Content)
}

func TestHandleMessageTask_OfficeReviewDoesNotTransitionTaskState(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	task.AssigneeAgentProfileID = "agent-profile-1"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "office review follow-up", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
}

func TestHandleMessageTask_OfficeDispatchErrorRestoresWorkflowStep(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	task.AssigneeAgentProfileID = "agent-profile-1"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		updatedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		updatedTask.State = v1.TaskStateFailed
		updatedTask.WorkflowStepID = "step-in-progress"
		return repo.UpdateTask(ctx, updatedTask)
	}
	orch.promptErrFirst = errors.New("send failed")

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "office failure", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)
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
