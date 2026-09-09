package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// sessionAge orders fallback candidates. The resolver walks sessions
// newest-started-first, and CreateTaskSession persists whatever StartedAt the
// caller sets — it stamps nothing — so two sessions left at the zero time sort
// arbitrarily and any "newest" assertion built on call order is a coin flip.
// Every session these tests create gets an explicit age.
type sessionAge int

func (a sessionAge) startedAt() time.Time {
	return time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC).Add(time.Duration(a) * time.Minute)
}

// addSession attaches another session to a task in the given state, started at
// the given age. Higher age = more recently started = earlier in the resolver's
// walk. seedTaskWithSession's primary is left at the zero time, so it is always
// the oldest.
func addSession(
	t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string,
	state models.TaskSessionState, age sessionAge,
) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID,
		AgentProfileID: "agent-profile-2",
		State:          models.TaskSessionStateCreated,
		StartedAt:      age.startedAt(),
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	if state != models.TaskSessionStateCreated {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, state, ""))
	}
	loaded, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	return loaded
}

func cancelSession(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	require.NoError(t, repo.UpdateTaskSessionState(
		context.Background(), sessionID, models.TaskSessionStateCancelled, ""))
}

func errorPayload(t *testing.T, resp *ws.Message) ws.ErrorPayload {
	t.Helper()
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	return payload
}

// The bug 3 regression. Observed on task 25509e92: stop_task_kandev cancelled
// the primary session while a spawned session was still RUNNING, and a
// session-id-less message_task_kandev failed outright instead of reaching the
// agent that was right there. Routing "to the task" must not dead-end on a
// pointer to a session that can never answer.
func TestHandleMessageTask_CancelledPrimaryFallsBackToTheLiveSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	live := addSession(t, repo, target.ID, "sess-live", models.TaskSessionStateRunning, 1)
	cancelSession(t, repo, primary.ID)

	h, orch := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "keep going", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "a task with a live session must stay reachable")

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, live.ID, out["session_id"], "the message must reach the live session")
	assert.Equal(t, 1, orch.queue.GetStatus(context.Background(), live.ID).Count)
	assert.Equal(t, 0, orch.queue.GetStatus(context.Background(), primary.ID).Count)
}

// A live primary keeps the message even when a newer session exists: the
// fallback is a repair for a dead pointer, not a "newest wins" rule that would
// quietly move an ordinary message onto a spawned sibling.
func TestHandleMessageTask_LivePrimaryKeepsPriorityOverNewerSessions(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	newer := addSession(t, repo, target.ID, "sess-newer", models.TaskSessionStateRunning, 1)

	h, orch := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "keep going", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, primary.ID, out["session_id"])
	assert.Equal(t, 0, orch.queue.GetStatus(context.Background(), newer.ID).Count)
}

// When every session really is terminal there is nothing to fall back to, and
// the error has to say what to do next rather than just what failed.
//
// The CODE stays INTERNAL_ERROR. "Target session FAILED/CANCELLED ->
// INTERNAL_ERROR" is written down in
// docs/specs/tasks/requirements/parent-child-message-interrupt.md, so improving the message
// must not quietly reclassify it: a terminal primary with nothing live is
// handed to dispatch exactly as before.
func TestHandleMessageTask_AllSessionsTerminalNamesTheRecoveryTool(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	cancelSession(t, repo, primary.ID)

	h, _ := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "keep going", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)

	payload := errorPayload(t, resp)
	assert.Equal(t, ws.ErrorCodeInternalError, payload.Code,
		"the documented code for a terminal target must survive a message-only fix")
	assert.Contains(t, payload.Message, "spawn_session_kandev",
		"a stopped task's error must name the tool that restarts it")
	assert.Contains(t, payload.Message, primary.ID,
		"the error must name the session it could not use")
}

// The bug 2 regression. A parent that stops a wedged child and then messages it
// used to get a bare "session is CANCELLED — cannot send message", with the
// actual recovery (spawn_session_kandev) documented nowhere.
func TestHandleMessageTask_ExplicitTerminalSessionNamesTheRecoveryTool(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	cancelSession(t, repo, primary.ID)

	h, _ := newMessageTaskHandler(t, svc)
	payload := senderPayload(target.ID, "restart please", sender.ID)
	payload["session_id"] = primary.ID
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, payload)
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)

	errPayload := errorPayload(t, resp)
	assert.Contains(t, errPayload.Message, "CANCELLED")
	assert.Contains(t, errPayload.Message, "spawn_session_kandev")
	assert.Contains(t, errPayload.Message, "list_task_sessions_kandev")
}

// An explicit session_id is never redirected. A caller that named a dead
// session must be told so, not silently rerouted to a different agent.
func TestHandleMessageTask_ExplicitTerminalSessionIsNotRedirected(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	live := addSession(t, repo, target.ID, "sess-live", models.TaskSessionStateRunning, 1)
	cancelSession(t, repo, primary.ID)

	h, orch := newMessageTaskHandler(t, svc)
	payload := senderPayload(target.ID, "restart please", sender.ID)
	payload["session_id"] = primary.ID
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, payload)
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeError, resp.Type)
	assert.Equal(t, 0, orch.queue.GetStatus(context.Background(), live.ID).Count,
		"a named session must not be swapped for another one")
}

// A task whose only sessions are spawned siblings has no primary row at all.
// The same fallback keeps it reachable.
func TestHandleMessageTask_NoPrimarySessionUsesTheLiveSpawnedSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	targetResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Target",
	})
	require.NoError(t, err)
	senderResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Sender",
	})
	require.NoError(t, err)
	spawned := addSession(t, repo, targetResult.Task.ID, "sess-spawned", models.TaskSessionStateRunning, 1)

	h, orch := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(targetResult.Task.ID, "hello", senderResult.Task.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, spawned.ID).Count)
}

// A task with no session at all keeps its NOT_FOUND, and now also reports the
// tool that gives it one.
func TestHandleMessageTask_NoSessionAtAllNamesTheRecoveryTool(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	targetResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Target",
	})
	require.NoError(t, err)
	senderResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Sender",
	})
	require.NoError(t, err)

	h, _ := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(targetResult.Task.ID, "hello", senderResult.Task.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)

	payload := errorPayload(t, resp)
	assert.Equal(t, ws.ErrorCodeNotFound, payload.Code)
	assert.Contains(t, payload.Message, "spawn_session_kandev")
}

// The fallback has to survive the idle dispatch path too. That path re-resolves
// an unpinned target to the task's primary session, which here is the cancelled
// one we routed around: an unpinned fallback works for a RUNNING target and
// bounces straight back to the dead primary for an IDLE one. Found by review,
// not by the running-target test above, which never reaches that code.
func TestHandleMessageTask_IdleFallbackSessionIsNotReroutedToTheCancelledPrimary(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	ctx := context.Background()
	live := addSession(t, repo, target.ID, "sess-live-idle", models.TaskSessionStateWaitingForInput, 1)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-row-" + live.ID, SessionID: live.ID, TaskID: target.ID,
		Status: "running", Resumable: true, AgentExecutionID: "exec-" + live.ID,
	}))
	cancelSession(t, repo, primary.ID)

	h, orch := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "pick this up", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "an idle live session must still receive the message")

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, live.ID, out["session_id"])
	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, live.ID, orch.promptCalls[0].sessionID,
		"the prompt must stay on the fallback session, not bounce to the cancelled primary")
}

// Fallback must pick a session the dispatcher can actually start a turn on, not
// merely the newest one that is not cancelled.
//
// A COMPLETED session is retired, not messageable: the idle dispatch path calls
// ProcessOnTurnStart, and the orchestrator's isTerminalSessionState counts
// COMPLETED as terminal. So a newer COMPLETED sibling shadowing an older RUNNING
// one made the call fail with a live session sitting right there. Reported
// independently by two reviewers on #2660; my own fallback tests never had a
// COMPLETED candidate, so none of them could see it.
func TestHandleMessageTask_CompletedSiblingDoesNotShadowTheLiveFallback(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	running := addSession(t, repo, target.ID, "sess-running", models.TaskSessionStateRunning, 1)
	retired := addSession(t, repo, target.ID, "sess-retired", models.TaskSessionStateCompleted, 2)
	cancelSession(t, repo, primary.ID)

	// The fixture only bites if the retired session really does come first in
	// the resolver's walk. Assert the ordering rather than assuming it — the
	// primary is stamped with the current time on its state change, so it
	// outranks both siblings and index 0 is not the interesting slot.
	sessions, err := svc.ListTaskSessions(context.Background(), target.ID)
	require.NoError(t, err)
	require.Less(t, indexOfSession(t, sessions, retired.ID), indexOfSession(t, sessions, running.ID),
		"fixture must present the completed session ahead of the running one")

	h, orch := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "keep going", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, running.ID, out["session_id"],
		"a retired completed session must not shadow the running one")
	assert.Equal(t, 1, orch.queue.GetStatus(context.Background(), running.ID).Count)
	assert.Equal(t, 0, orch.queue.GetStatus(context.Background(), retired.ID).Count)
}

// indexOfSession reports where a session lands in the resolver's walk order.
func indexOfSession(t *testing.T, sessions []*models.TaskSession, sessionID string) int {
	t.Helper()
	for i, session := range sessions {
		if session.ID == sessionID {
			return i
		}
	}
	t.Fatalf("session %s missing from the task's session list", sessionID)
	return -1
}
