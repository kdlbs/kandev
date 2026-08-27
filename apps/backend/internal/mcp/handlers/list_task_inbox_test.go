package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func taskInboxContext(taskID, sessionID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CallerTaskID:    taskID,
		CallerSessionID: sessionID,
	})
}

// @covers AC-1, AC-4: inbox aggregation is task-bound and includes all of
// the task's delivered inbound prompts without exposing another task.
func TestHandleListTaskInbox_AggregatesDeliveredMessagesForOwnTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	sibling := &models.TaskSession{ID: "inbox-sibling", TaskID: task.ID, IsPrimary: false, State: models.TaskSessionStateWaitingForInput}
	require.NoError(t, repo.CreateTaskSession(context.Background(), sibling))
	for _, session := range []*models.TaskSession{primary, sibling} {
		metadata := map[string]interface{}(nil)
		content := session.ID
		if session.ID == primary.ID {
			metadata = map[string]interface{}{
				"queue_entry_id":     "queue-primary",
				inboxTransitionIDKey: "transition-primary",
				"attachments":        []messagequeue.MessageAttachment{{Type: "image", Data: "secret", MimeType: "image/png", Name: "delivered.png", SizeBytes: 12}},
			}
			content = "visible\n\n<kandev-system>hidden</kandev-system>"
		}
		_, err := svc.CreateMessage(context.Background(), &service.CreateMessageRequest{TaskSessionID: session.ID, TaskID: task.ID, AuthorType: "user", Content: content, Metadata: metadata})
		require.NoError(t, err)
	}
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	ctx := taskInboxContext(task.ID, primary.ID)
	resp, err := h.handleListTaskInbox(ctx, makeWSMessage(t, ws.ActionMCPListTaskInbox, map[string]interface{}{"task_id": task.ID, "caller_task_id": "spoofed"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload struct {
		Items []inboxItem `json:"items"`
		Total int         `json:"total"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Len(t, payload.Items, 2)
	assert.Equal(t, 2, payload.Total)
	assert.Equal(t, "transition-primary", payload.Items[0].TransitionID)
	assert.True(t, payload.Items[0].IsCurrent)
	assert.Equal(t, "visible", payload.Items[0].Content)
	require.Len(t, payload.Items[0].Attachments, 1)
	assert.Equal(t, "delivered.png", payload.Items[0].Attachments[0].Name)
	assert.NotContains(t, string(resp.Payload), "secret")
}

func TestHandleListTaskInbox_PaginatesDeliveredMessagesWithCursor(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, task, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	sibling := &models.TaskSession{ID: "inbox-page-sibling", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput}
	require.NoError(t, repo.CreateTaskSession(ctx, sibling))
	first, err := svc.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: primary.ID,
		TaskID:        task.ID,
		AuthorType:    "user",
		Content:       "first",
	})
	require.NoError(t, err)
	second, err := svc.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: sibling.ID,
		TaskID:        task.ID,
		AuthorType:    "user",
		Content:       "second",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	request := func(cursor string) *ws.Message {
		return makeWSMessage(t, ws.ActionMCPListTaskInbox, map[string]interface{}{
			"task_id":        task.ID,
			"caller_task_id": task.ID,
			"limit":          1,
			"cursor":         cursor,
		})
	}
	var firstPayload struct {
		Items   []inboxItem `json:"items"`
		Total   int         `json:"total"`
		Cursor  string      `json:"cursor"`
		HasMore bool        `json:"has_more"`
	}
	resp, err := h.handleListTaskInbox(taskInboxContext(task.ID, primary.ID), request(""))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(resp.Payload, &firstPayload))
	require.Len(t, firstPayload.Items, 1)
	assert.Equal(t, first.ID, firstPayload.Items[0].ID)
	assert.Equal(t, 2, firstPayload.Total)
	assert.True(t, firstPayload.HasMore)
	assert.NotEmpty(t, firstPayload.Cursor)

	var secondPayload struct {
		Items   []inboxItem `json:"items"`
		Total   int         `json:"total"`
		Cursor  string      `json:"cursor"`
		HasMore bool        `json:"has_more"`
	}
	resp, err = h.handleListTaskInbox(taskInboxContext(task.ID, primary.ID), request(firstPayload.Cursor))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(resp.Payload, &secondPayload))
	require.Len(t, secondPayload.Items, 1)
	assert.Equal(t, second.ID, secondPayload.Items[0].ID)
	assert.Equal(t, 2, secondPayload.Total)
	assert.False(t, secondPayload.HasMore)
	assert.Empty(t, secondPayload.Cursor)
}

func TestHandleListTaskInbox_RejectsForeignTarget(t *testing.T) {
	h := &Handlers{}
	resp, err := h.handleListTaskInbox(context.Background(), makeWSMessage(t, ws.ActionMCPListTaskInbox, map[string]interface{}{"task_id": "foreign", "caller_task_id": "own"}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// @covers AC-2, AC-7, AC-9: queue visibility is a non-mutating snapshot and
// never includes a lifecycle row already reserved for delivery.
func TestHandleListTaskInbox_ListsSafeQueuedPromptsWithoutMutatingQueue(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, task, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc)
	require.NoError(t, orch.queue.SetAutoRun(ctx, session.ID, false))
	orch.queue.SetPendingMove(ctx, session.ID, &messagequeue.PendingMove{MoveID: "move-1", TaskID: task.ID})
	queued, err := orch.queue.QueueMessage(ctx, session.ID, task.ID, "queued prompt", "", "peer", false, []messagequeue.MessageAttachment{{AttachmentID: "att-1", Type: "image", Data: "secret-bytes", MimeType: "image/png", Name: "diagram.png", SizeBytes: 42}})
	require.NoError(t, err)

	resp, err := h.handleListTaskInbox(taskInboxContext(task.ID, session.ID), makeWSMessage(t, ws.ActionMCPListTaskInbox, map[string]interface{}{"task_id": task.ID, "caller_task_id": task.ID, "current_session_id": "spoofed"}))
	require.NoError(t, err)
	var payload struct {
		Items []inboxItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Items, 1)
	assert.Equal(t, queued.ID, payload.Items[0].ID)
	assert.Equal(t, queued.ID, payload.Items[0].TransitionID)
	require.Len(t, payload.Items[0].Attachments, 1)
	assert.Equal(t, "diagram.png", payload.Items[0].Attachments[0].Name)
	assert.NotContains(t, string(resp.Payload), "secret-bytes")
	status := orch.queue.GetStatus(ctx, session.ID)
	assert.False(t, status.AutoRun)
	require.Len(t, status.Entries, 1)
	assert.Equal(t, queued.ID, status.Entries[0].ID)
	move, ok := orch.queue.GetPendingMove(ctx, session.ID)
	require.True(t, ok)
	assert.Equal(t, "move-1", move.MoveID)
}

// @covers AC-3, AC-6: transition identity lets a caller recognize a queue
// drain as the same work, while the cursor orders equal timestamps by ID.
func TestTaskInboxCursorAndTransitionIdentity(t *testing.T) {
	stamp := time.Now().UTC()
	first := inboxItem{ID: "a", TransitionID: "queue-a", Timestamp: stamp}
	second := inboxItem{ID: "b", TransitionID: "queue-a", Timestamp: stamp}
	assert.True(t, inboxBefore(first, second))
	cursor, err := decodeInboxCursor(encodeInboxCursor(first))
	require.NoError(t, err)
	assert.True(t, inboxAfter(second, *cursor))
	assert.False(t, inboxAfter(first, *cursor))
}

func TestInboxLimitCapsOversizedPages(t *testing.T) {
	assert.Equal(t, inboxDefaultLimit, inboxLimit(0))
	assert.Equal(t, 7, inboxLimit(7))
	assert.Equal(t, inboxMaxLimit, inboxLimit(inboxMaxLimit+1))
}

func TestHandleListTaskInbox_AllowsAutomationDispatch(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID:    "automation-1",
		WorkspaceID:     task.WorkspaceID,
		CallerTaskID:    task.ID,
		CallerSessionID: session.ID,
		Surface:         mcpprofile.SurfaceAutomation,
	})
	resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListTaskInbox, map[string]interface{}{"task_id": task.ID}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)
}
