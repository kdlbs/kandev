package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func spawnPayload(taskID, prompt, senderTaskID, senderSessionID string) map[string]interface{} {
	return map[string]interface{}{
		"task_id":           taskID,
		"prompt":            prompt,
		"sender_task_id":    senderTaskID,
		"sender_session_id": senderSessionID,
	}
}

func TestHandleSpawnSession_MissingFields(t *testing.T) {
	h := &Handlers{}
	for name, payload := range map[string]map[string]interface{}{
		"missing task_id": {"prompt": "do things"},
		"missing prompt":  {"task_id": "task-1"},
	} {
		msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
		resp, err := h.handleSpawnSession(context.Background(), msg)
		require.NoError(t, err, name)
		assertWSError(t, resp, ws.ErrorCodeValidation)
	}
}

func TestHandleSpawnSession_TaskNotFound(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h, _ := newMessageTaskHandler(t, svc)
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload("no-such-task", "do things", "no-such-task", "sess-x"))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// Spawning on the caller's own task inherits the caller session's agent
// profile when none is given, launches via IntentStart, wraps the prompt with
// spawner attribution, and applies the optional session name.
func TestHandleSpawnSession_SameTask_DefaultsToSenderProfile(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	payload := spawnPayload(target.ID, "review the diff please", target.ID, sess.ID)
	payload["name"] = "reviewer"
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, target.ID, out["task_id"])
	assert.Equal(t, "spawned-sess-1", out["session_id"])
	assert.Equal(t, "agent-profile-1", out["agent_profile_id"])

	require.Len(t, orch.launchCalls, 1)
	launched := orch.launchCalls[0]
	assert.Equal(t, target.ID, launched.TaskID)
	assert.Equal(t, orchestrator.IntentStart, launched.Intent)
	assert.Equal(t, "agent-profile-1", launched.AgentProfileID)
	// Prompt carries the spawner attribution block plus the original body.
	assert.Contains(t, launched.Prompt, "review the diff please")
	assert.Contains(t, launched.Prompt, "<kandev-system>")
	assert.Contains(t, launched.Prompt, sess.ID)

	require.Len(t, orch.renameCalls, 1)
	assert.Equal(t, renameCall{sessionID: "spawned-sess-1", name: "reviewer"}, orch.renameCalls[0])
}

// An explicit agent_profile_id wins over the sender session's profile, and no
// rename happens when name is omitted.
func TestHandleSpawnSession_ExplicitProfile(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	payload := spawnPayload(target.ID, "work on the docs", target.ID, sess.ID)
	payload["agent_profile_id"] = "agent-profile-2"
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	assert.Equal(t, "agent-profile-2", orch.launchCalls[0].AgentProfileID)
	assert.Empty(t, orch.renameCalls)
}

// Cross-task spawns (sender session belongs to another task) fall back to the
// target task's primary-session profile.
func TestHandleSpawnSession_CrossTask_DefaultsToTargetPrimaryProfile(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "help out", sender.ID, "sender-sess-1"))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	assert.Equal(t, "agent-profile-1", orch.launchCalls[0].AgentProfileID)
}

// A LaunchSession failure surfaces as an internal error to the caller instead
// of a half-reported success.
func TestHandleSpawnSession_LaunchFailure(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	orch.launchErr = errors.New("executor unavailable")

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do things", target.ID, sess.ID))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assert.Empty(t, orch.renameCalls, "no rename after a failed launch")
}

// Spawning on a task in another workspace is rejected — unlike message_task,
// spawn consumes executor resources and must stay workspace-scoped.
func TestHandleSpawnSession_CrossWorkspace_Forbidden(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, _, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2", Name: "Board"}))
	other, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-2",
		WorkflowID:  "wf-2",
		Title:       "Other-workspace task",
	})
	require.NoError(t, err)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(other.ID, "help out", sender.ID, "sender-sess-1"))
	resp, err := h.handleSpawnSession(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	assert.Empty(t, orch.launchCalls, "cross-workspace spawn must not launch")
}

func TestWrapSpawnedSessionPrompt(t *testing.T) {
	wrapped := wrapSpawnedSessionPrompt("do the thing", "task-1", "sess-1")
	assert.Contains(t, wrapped, "do the thing")
	assert.Contains(t, wrapped, "<kandev-system>")
	assert.Contains(t, wrapped, `task_id="task-1"`)
	assert.Contains(t, wrapped, `session_id="sess-1"`)

	// No sender session (e.g. external mode) → prompt passes through unwrapped.
	assert.Equal(t, "plain", wrapSpawnedSessionPrompt("plain", "task-1", ""))
}
