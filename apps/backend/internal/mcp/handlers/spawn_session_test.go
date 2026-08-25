package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
// profile when none is given, launches via IntentStart, carries the spawner
// origin, and applies the optional session name.
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
	// The prompt travels unwrapped; spawner attribution rides along as
	// structured origin so the launch site can build a trusted system block
	// that survives first-turn canonicalization.
	assert.Equal(t, "review the diff please", launched.Prompt)
	require.NotNil(t, launched.SpawnOrigin)
	assert.Equal(t, target.ID, launched.SpawnOrigin.TaskID)
	assert.Equal(t, sess.ID, launched.SpawnOrigin.SessionID)

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

func TestHandleSpawnSessionReportsEffectiveAgentProfile(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	orch.launchResponseProfileID = "workflow-default-profile"

	payload := spawnPayload(target.ID, "work with the workflow profile", target.ID, sess.ID)
	payload["agent_profile_id"] = "requested-profile"
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "workflow-default-profile", out["agent_profile_id"])
	assert.Equal(t, "requested-profile", orch.launchCalls[0].AgentProfileID)
}

func TestHandleSpawnSessionReportsStepPinnedAgentProfile(t *testing.T) {
	svc, repo, _, workflowRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	workspace, workflow := defaultWorkspaceAndWorkflow(t, ctx, svc)
	workflowProfileID := "workflow-default-profile"
	_, err := svc.UpdateWorkflow(ctx, workflow.ID, &service.UpdateWorkflowRequest{
		AgentProfileID: &workflowProfileID,
	})
	require.NoError(t, err)
	step := seedWorkflowStep(t, ctx, workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID:      workflow.ID,
		Name:            "Pinned",
		Position:        0,
		IsStartStep:     true,
		AgentProfileID:  "step-pinned-profile",
		AllowManualMove: true,
	})
	now := time.Now().UTC()
	target := &models.Task{
		ID:             "task-pinned-target",
		WorkspaceID:    workspace.ID,
		WorkflowID:     workflow.ID,
		WorkflowStepID: step.ID,
		Title:          "Target task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	sender := &models.Task{
		ID:          "task-pinned-sender",
		WorkspaceID: workspace.ID,
		WorkflowID:  workflow.ID,
		Title:       "Sender task",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, target))
	require.NoError(t, repo.CreateTask(ctx, sender))
	session := &models.TaskSession{
		ID:             "sess-pinned",
		TaskID:         target.ID,
		AgentProfileID: "sender-profile",
		IsPrimary:      true,
		State:          models.TaskSessionStateRunning,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.launchResponseProfileID = step.AgentProfileID
	payload := spawnPayload(target.ID, "use the pinned profile", sender.ID, session.ID)
	payload["agent_profile_id"] = "explicit-profile"
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
	resp, err := h.handleSpawnSession(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "step-pinned-profile", out["agent_profile_id"])
	assert.Equal(t, "explicit-profile", orch.launchCalls[0].AgentProfileID)
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

func TestHandleSpawnSession_WorkspacePreparingReturnsRecoverableConflict(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc)
	orch.launchErr = models.ErrWorkspacePreparing

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do things", target.ID, sess.ID))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "workspace_preparing", payload.Details["reason"])
	assert.Equal(t, true, payload.Details["recoverable"])
	assert.Empty(t, orch.renameCalls)
}

func TestHandleSpawnSession_WorkspaceUnsafeReturnsRecoverableConflict(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc)
	orch.launchErr = models.ErrWorkspaceReuseUnsafe

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do things", target.ID, sess.ID))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "workspace_reuse_unsafe", payload.Details["reason"])
	assert.Empty(t, orch.renameCalls)
}

// A rename failure after a successful launch must not fail the spawn — the
// session is already running; the label is best-effort.
func TestHandleSpawnSession_RenameFailure_DoesNotFailSpawn(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)
	orch.renameErr = errors.New("rename write failed")

	payload := spawnPayload(target.ID, "do things", target.ID, sess.ID)
	payload["name"] = "reviewer"
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, payload)
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "spawned-sess-1", out["session_id"])
	require.Len(t, orch.renameCalls, 1, "rename was attempted")
}

// Spawning on a task in another workspace is rejected — unlike message_task,
// spawn consumes executor resources and must stay workspace-scoped.
func TestHandleSpawnSession_CrossWorkspace_Forbidden(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, _, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2", Name: "Board"}))
	otherResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-2",
		WorkflowID:  "wf-2",
		Title:       "Other-workspace task",
	})
	other := otherResult.Task
	require.NoError(t, err)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(other.ID, "help out", sender.ID, "sender-sess-1"))
	resp, err := h.handleSpawnSession(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	assert.Empty(t, orch.launchCalls, "cross-workspace spawn must not launch")
}

// The spawner's session name (tab label) rides along so the new agent can refer
// to its spawner by name rather than by bare UUID.
func TestHandleSpawnSession_OriginCarriesSpawnerSessionName(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	spawner := &models.TaskSession{
		ID:             "sess-planner",
		TaskID:         target.ID,
		AgentProfileID: "agent-profile-1",
		Name:           "planner",
		State:          models.TaskSessionStateRunning,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, spawner))

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do the thing", target.ID, spawner.ID))
	resp, err := h.handleSpawnSession(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	require.NotNil(t, orch.launchCalls[0].SpawnOrigin)
	assert.Equal(t, "planner", orch.launchCalls[0].SpawnOrigin.SessionName)
}

// Without a sender session (external MCP callers) there is nothing to attribute,
// so the launch carries no origin and the prompt is untouched.
func TestHandleSpawnSession_NoSenderSession_NoOrigin(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "plain", target.ID, ""))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	assert.Nil(t, orch.launchCalls[0].SpawnOrigin)
	assert.Equal(t, "plain", orch.launchCalls[0].Prompt)
}

// A caller-supplied sender_session_id that does not belong to the claimed sender
// task must not become launch attribution: the spawned agent would be told to
// report to a session that never asked for it. Falling back to the target's
// primary profile keeps the spawn working without the forged origin.
func TestHandleSpawnSession_ForgedSenderSession_YieldsNoOrigin(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, victim := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	// victim belongs to the target task, but the caller claims it is a session of
	// the sender task.
	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do things", sender.ID, victim.ID))
	resp, err := h.handleSpawnSession(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	assert.Nil(t, orch.launchCalls[0].SpawnOrigin)
	assert.Equal(t, "agent-profile-1", orch.launchCalls[0].AgentProfileID)
}

// An unknown sender_session_id is likewise not attributable.
func TestHandleSpawnSession_UnknownSenderSession_YieldsNoOrigin(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	h, orch := newMessageTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPSpawnSession, spawnPayload(target.ID, "do things", target.ID, "sess-ghost"))
	resp, err := h.handleSpawnSession(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.launchCalls, 1)
	assert.Nil(t, orch.launchCalls[0].SpawnOrigin)
}
