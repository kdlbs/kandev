package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// seedStepCompleteTarget seeds a workspace, task (with WorkflowStepID), and
// session in the requested state. Used by every TestHandleStepComplete_* case
// that needs the precondition chain in `resolveStepCompleteTarget` to succeed.
func seedStepCompleteTarget(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, stepID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID:        "ws-step-complete",
		Name:      "Step Complete",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-step-complete",
		WorkflowStepID: stepID,
		Title:          "Step Complete Task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     state,
		StartedAt: now,
		UpdatedAt: now,
	}))
}

func newStepCompleteHandler(t *testing.T, taskSvc *service.Service, repo *sqliterepo.Repository, bus *mcpRecordingEventBus) *Handlers {
	t.Helper()
	return &Handlers{
		taskSvc:     taskSvc,
		sessionRepo: repo,
		eventBus:    bus,
		logger:      testLogger(t).WithFields(),
	}
}

// TestHandleStepComplete_MissingFields covers the input-validation branches
// that fail before any DB lookup. All three return ErrorCodeValidation.
func TestHandleStepComplete_MissingFields(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{
			name:    "missing task_id",
			payload: map[string]interface{}{"session_id": "s1", "summary": "done"},
			want:    ws.ErrorCodeValidation,
		},
		{
			name:    "missing session_id",
			payload: map[string]interface{}{"task_id": "t1", "summary": "done"},
			want:    ws.ErrorCodeValidation,
		},
		{
			name:    "blank summary",
			payload: map[string]interface{}{"task_id": "t1", "session_id": "s1", "summary": "   "},
			want:    ws.ErrorCodeValidation,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{logger: testLogger(t).WithFields()}
			msg := makeWSMessage(t, ws.ActionMCPStepComplete, tc.payload)
			resp, err := h.handleStepComplete(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, tc.want)
		})
	}
}

// TestHandleStepComplete_SessionDoesNotBelongToTask verifies the ownership
// guard rejects requests where the session.TaskID doesn't match the request's
// task_id.
func TestHandleStepComplete_SessionDoesNotBelongToTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-owner", "session-owner", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-OTHER",
		"session_id": "session-owner",
		"summary":    "wrong task",
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Empty(t, bus.events, "ownership rejection must not publish an event")
}

// TestHandleStepComplete_TerminalSessionRejected covers the
// Completed/Failed/Cancelled guard: writing a signal to a terminal session
// would never be consumed (subscriber short-circuits on non-WAITING state,
// no future turn-end fires), so we reject up front instead of returning
// accepted=true followed by silent no-op.
func TestHandleStepComplete_TerminalSessionRejected(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			seedStepCompleteTarget(t, repo, "task-term", "session-term", "step-1", state)
			bus := &mcpRecordingEventBus{}
			h := newStepCompleteHandler(t, svc, repo, bus)

			msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
				"task_id":    "task-term",
				"session_id": "session-term",
				"summary":    "too late",
			})
			resp, err := h.handleStepComplete(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeValidation)
			assert.Empty(t, bus.events, "terminal-session rejection must not publish")
		})
	}
}

// TestHandleStepComplete_FirstCallAccepted covers the happy path: bag is
// written, event is published with the documented payload shape, and the
// response reports accepted=true with the persisted step_id + signaled_at.
func TestHandleStepComplete_FirstCallAccepted(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-first", "session-first", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-first",
		"session_id": "session-first",
		"summary":    "implementation finished",
		"handoff":    "tests next",
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"])
	assert.Equal(t, "step-1", payload["step_id"])
	// signaled_at is part of the documented response contract — pin its
	// presence + RFC3339Nano shape so a future refactor can't silently
	// drop or rename the field.
	signaledAt, ok := payload["signaled_at"].(string)
	require.True(t, ok, "expected signaled_at string in response payload")
	_, parseErr := time.Parse(time.RFC3339Nano, signaledAt)
	require.NoError(t, parseErr, "signaled_at must be RFC3339Nano")

	// Bag written under the canonical key.
	session, err := repo.GetTaskSession(context.Background(), "session-first")
	require.NoError(t, err)
	bag, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok, "expected bag entry to be persisted")
	assert.Equal(t, "step-1", bag.StepID)
	assert.Equal(t, models.StepCompletionSourceAgent, bag.Source)
	assert.Equal(t, "implementation finished", bag.Summary)
	assert.Equal(t, "tests next", bag.Handoff)

	// Bus event published with the public payload shape (no handoff/blockers
	// on the wire — those live in the bag only).
	require.Len(t, bus.events, 1, "expected one bus publish")
	assert.Equal(t, events.WorkflowStepCompletionSignaled, bus.events[0].Type)
	data, ok := bus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-first", data["task_id"])
	assert.Equal(t, "session-first", data["session_id"])
	assert.Equal(t, "step-1", data["step_id"])
	assert.Equal(t, "implementation finished", data["summary"])
	_, hasHandoff := data["handoff"]
	assert.False(t, hasHandoff, "handoff is bag-only, not on the wire")
}

// TestHandleStepComplete_DedupRunningNoRepublish covers the
// `already_signaled` short-circuit while the session is still RUNNING. The
// inline turn-end path will pick up the bag — no re-publish is needed and
// none should fire (avoids a spurious second event for the subscriber).
func TestHandleStepComplete_DedupRunningNoRepublish(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-dup", "session-dup", "step-1", models.TaskSessionStateRunning)
	// Pre-write the bag to simulate "first call already happened".
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-dup", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step-1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "first call",
		SignaledAt: time.Now().UTC(),
	}))
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-dup",
		"session_id": "session-dup",
		"summary":    "second call (same step)",
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, false, payload["accepted"])
	assert.Equal(t, "already_signaled", payload["reason"])

	assert.Empty(t, bus.events, "RUNNING dedup path must not re-publish (inline turn-end will consume the bag)")
}

// TestHandleStepComplete_DedupWaitingRepublishes covers the retry-after-
// publish-failure path. When the session is WAITING_FOR_INPUT, the bag is
// already persisted but the orchestrator subscriber may have never fired
// (e.g., the first call's publish failed after the bag write). A retry
// MUST re-publish so the subscriber gets a chance to drive the transition,
// otherwise the session stays stuck until the user replies.
func TestHandleStepComplete_DedupWaitingRepublishes(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-retry", "session-retry", "step-1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-retry", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step-1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "first call",
		SignaledAt: time.Now().UTC(),
	}))
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-retry",
		"session_id": "session-retry",
		"summary":    "retry after publish failure",
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, false, payload["accepted"])
	assert.Equal(t, "already_signaled", payload["reason"])

	require.Len(t, bus.events, 1, "WAITING dedup must re-publish the bus event so the subscriber can drive the transition")
	assert.Equal(t, events.WorkflowStepCompletionSignaled, bus.events[0].Type)
}
