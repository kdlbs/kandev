package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// countingMessageCreator records how many bundles the handler asked it to
// create; the retry tests assert it stays untouched when durable messages
// already exist for the retry identity.
type countingMessageCreator struct {
	calls atomic.Int32
}

func (c *countingMessageCreator) CreateClarificationRequestMessages(context.Context, string, string, string, []clarification.Question, string) ([]string, error) {
	c.calls.Add(1)
	return []string{"m-created"}, nil
}

const retryTestKey = "conn-a/int64:7"

var retryTestQuestions = []map[string]interface{}{{
	"id":     "q1",
	"prompt": "Which color?",
	"options": []map[string]interface{}{
		{"label": "Red", "description": "R"},
		{"label": "Blue", "description": "B"},
	},
}}

// seedRetrySession creates a task and running session and returns the retry
// identity an exact retry of retryTestKey would derive for that session.
func seedRetrySession(t *testing.T, ctx context.Context, svc *service.Service, repo *sqliterepo.Repository, suffix string) (taskID, sessionID, pendingID string) {
	t.Helper()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + suffix, Name: "WS"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + suffix, WorkspaceID: "ws-" + suffix, Name: "Board"}))
	taskResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{WorkspaceID: "ws-" + suffix, WorkflowID: "wf-" + suffix, Title: "Task"})
	require.NoError(t, err)
	sess := &models.TaskSession{ID: "sess-" + suffix, TaskID: taskResult.Task.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))
	return taskResult.Task.ID, sess.ID, clarification.PendingIDForRequest(sess.ID, retryTestKey)
}

// seedRetryMessage commits the durable question message an interrupted
// ask_user_question call would already have created, in the recorded status.
func seedRetryMessage(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, sessionID, pendingID, status string, response map[string]interface{}) {
	t.Helper()
	meta := map[string]interface{}{
		"pending_id":     pendingID,
		"question_id":    "q1",
		"question_index": 0,
		"status":         status,
		"question": map[string]interface{}{
			"id": "q1", "title": "Color", "prompt": "Which color?",
			"options": []interface{}{
				map[string]interface{}{"option_id": "opt-red", "label": "Red", "description": "R"},
				map[string]interface{}{"option_id": "opt-blue", "label": "Blue", "description": "B"},
			},
		},
	}
	if response != nil {
		meta["response"] = response
	}
	turn := &models.Turn{ID: "turn-" + sessionID, TaskSessionID: sessionID, TaskID: taskID}
	require.NoError(t, repo.CreateTurn(ctx, turn))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turn.ID,
		AuthorType:    "agent",
		Type:          "clarification_request",
		Content:       "Which color?",
		Metadata:      meta,
	}))
}

func retryAskPayload(sessionID, taskID string) map[string]interface{} {
	return map[string]interface{}{
		"session_id": sessionID,
		"task_id":    taskID,
		"retry_key":  retryTestKey,
		"questions":  retryTestQuestions,
	}
}

func TestHandleAskUserQuestion_RetryReusesDurableBundleWithoutRecreatingMessages(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-pending")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "pending", nil)

	store := clarification.NewStore(time.Minute)
	creator := &countingMessageCreator{}
	h := NewHandlers(svc, nil, store, nil, creator, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	var wg sync.WaitGroup
	var resp *ws.Message
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		resp, err = h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
		require.NoError(t, err)
	}()

	require.Eventually(t, func() bool { return len(store.ListPending()) == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, pendingID, store.ListPending()[0].PendingID, "the store entry must adopt the durable identity so the visible bundle's answer reaches this waiter")
	assert.Equal(t, int32(0), creator.calls.Load(), "no second visible question bundle may be published")

	messages, err := repo.FindMessagesByPendingID(ctx, pendingID)
	require.NoError(t, err)
	assert.Len(t, messages, 1)

	answer := &clarification.Response{PendingID: pendingID, Answers: []clarification.Answer{{QuestionID: "q1", SelectedOptions: []string{"opt-blue"}}}}
	require.NoError(t, store.Respond(pendingID, answer))
	wg.Wait()

	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var body clarification.Response
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	assert.Equal(t, pendingID, body.PendingID)
	require.Len(t, body.Answers, 1)
	assert.Equal(t, []string{"opt-blue"}, body.Answers[0].SelectedOptions)
}

func TestHandleAskUserQuestion_RetryReturnsRecordedAnswerWithoutWaiting(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-answered")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "answered", map[string]interface{}{
		"question_id": "q1", "selected_options": []interface{}{"opt-red"}, "custom_text": "because",
	})

	store := clarification.NewStore(time.Minute)
	creator := &countingMessageCreator{}
	h := NewHandlers(svc, nil, store, nil, creator, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	done := make(chan *ws.Message, 1)
	go func() {
		resp, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
		require.NoError(t, err)
		done <- resp
	}()

	var resp *ws.Message
	select {
	case resp = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retry of an answered bundle must return immediately, not wait for a new answer")
	}
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var body clarification.Response
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	assert.Equal(t, pendingID, body.PendingID)
	assert.False(t, body.Rejected)
	require.Len(t, body.Answers, 1)
	assert.Equal(t, "q1", body.Answers[0].QuestionID)
	assert.Equal(t, []string{"opt-red"}, body.Answers[0].SelectedOptions)
	assert.Equal(t, "because", body.Answers[0].CustomText)

	assert.Empty(t, store.ListPending(), "a recorded outcome must not open a new in-memory wait")
	assert.Equal(t, int32(0), creator.calls.Load())
}

func TestHandleAskUserQuestion_RetryReturnsRecordedRejection(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-rejected")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "rejected", nil)

	store := clarification.NewStore(time.Minute)
	h := NewHandlers(svc, nil, store, nil, &countingMessageCreator{}, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	resp, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var body clarification.Response
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	assert.True(t, body.Rejected)
	assert.Empty(t, store.ListPending())
}

func TestHandleAskUserQuestion_RetryOfCancelledBundleReturnsCancelledError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-cancelled")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "cancelled", nil)

	store := clarification.NewStore(time.Minute)
	creator := &countingMessageCreator{}
	h := NewHandlers(svc, nil, store, nil, creator, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	resp, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assert.Empty(t, store.ListPending(), "a cancelled bundle must not be re-opened by a retry")
	assert.Equal(t, int32(0), creator.calls.Load(), "a retry must not re-ask a cancelled question")
}

func TestHandleAskUserQuestion_RetryIgnoresBundleOwnedByAnotherSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-owner")
	other := &models.TaskSession{ID: "sess-other", TaskID: taskID, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(ctx, other))
	// A bundle under this identity but owned by another session must never be
	// adopted, even though the identity already binds the session.
	seedRetryMessage(t, ctx, repo, taskID, other.ID, pendingID, "answered", map[string]interface{}{"selected_options": []interface{}{"opt-red"}})

	store := clarification.NewStore(time.Minute)
	creator := &countingMessageCreator{}
	h := NewHandlers(svc, nil, store, nil, creator, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool { return len(store.ListPending()) == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, int32(1), creator.calls.Load(), "the foreign bundle must not suppress this session's own question")
	store.CancelSession(sessionID)
	wg.Wait()
}

func TestHandleAskUserQuestion_WithoutRetryKeyUsesRandomIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID, derived := seedRetrySession(t, ctx, svc, repo, "retry-none")

	store := clarification.NewStore(time.Minute)
	h := NewHandlers(svc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	payload := retryAskPayload(sessionID, taskID)
	delete(payload, "retry_key")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload))
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool { return len(store.ListPending()) == 1 }, time.Second, 5*time.Millisecond)
	got := store.ListPending()[0].PendingID
	assert.NotEmpty(t, got)
	assert.NotEqual(t, derived, got, "without a transport retry key the identity must stay random")
	assert.NotEqual(t, clarification.PendingIDForRequest(sessionID, "test-id"), got, "the backend's own ws message id is not a retry identity")
	store.CancelSession(sessionID)
	wg.Wait()
}
