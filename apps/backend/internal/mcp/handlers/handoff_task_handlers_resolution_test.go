package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// --- AC-12/AC-12a: a nonexistent workflow and a workflow that belongs to a
// third workspace must be indistinguishable to the caller ---

func TestHandleHandoffTask_WorkflowNotFoundAndWorkflowInWrongWorkspaceGiveIdenticalMessage(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)

	payloadMissing := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: "wf-does-not-exist"})
	respMissing, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payloadMissing))
	require.NoError(t, err)
	assertWSError(t, respMissing, ws.ErrorCodeValidation)

	const thirdWorkspaceID = "ws-handoff-third"
	const thirdWorkflowID = "wf-handoff-third"
	seedHandoffWorkspace(t, f.repo, thirdWorkspaceID)
	require.NoError(t, f.repo.CreateWorkflow(context.Background(), &models.Workflow{
		ID: thirdWorkflowID, WorkspaceID: thirdWorkspaceID, Name: "Elsewhere",
	}))

	payloadWrongWorkspace := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: thirdWorkflowID})
	respWrongWorkspace, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payloadWrongWorkspace))
	require.NoError(t, err)
	assertWSError(t, respWrongWorkspace, ws.ErrorCodeValidation)

	var epMissing, epWrongWorkspace ws.ErrorPayload
	require.NoError(t, json.Unmarshal(respMissing.Payload, &epMissing))
	require.NoError(t, json.Unmarshal(respWrongWorkspace.Payload, &epWrongWorkspace))
	assert.Equal(t, epMissing.Message, epWrongWorkspace.Message, "must not leak whether the workflow exists in a different workspace")
	assert.NotContains(t, epWrongWorkspace.Message, thirdWorkspaceID)
}

// --- AC-14b: an agent profile with no WorkspaceID (global) is accepted for
// any target workspace ---

func TestHandleHandoffTask_GlobalAgentProfileAcceptedForAnyTargetWorkspace(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const globalProfileID = "profile-global"
	seedHandoffAgentProfile(t, f.agentStore, globalProfileID, agentsettingsmodels.AgentRoleWorker, "", "")

	payload := f.validPayload(map[string]interface{}{handoffFieldAgentProfileID: globalProfileID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)
	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
}

// --- AC-15c: two auto-start-eligible steps sharing a position resolve
// deterministically by (position, id) ascending ---

func TestHandleHandoffTask_AutoStartStepTiebreakIsDeterministicByStepID(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	ctx := context.Background()
	autoStartEvents := workflowmodels.StepEvents{OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}}}

	stepHigh := seedWorkflowStep(t, ctx, f.workflowRepo, &workflowmodels.WorkflowStep{
		ID: "step-zzz-higher", WorkflowID: f.targetWorkflowID, Name: "High", Position: 5,
		Events: autoStartEvents, AllowManualMove: true,
	})
	stepLow := seedWorkflowStep(t, ctx, f.workflowRepo, &workflowmodels.WorkflowStep{
		ID: "step-aaa-lower", WorkflowID: f.targetWorkflowID, Name: "Low", Position: 5,
		Events: autoStartEvents, AllowManualMove: true,
	})
	originalStep, err := f.workflowRepo.GetStep(ctx, f.targetStepID)
	require.NoError(t, err)
	f.svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{
		originalStep.ID: originalStep,
		stepHigh.ID:     stepHigh,
		stepLow.ID:      stepLow,
	}})

	payload := f.validPayload(map[string]interface{}{handoffFieldStartAgent: true})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)
	assert.Equal(t, stepLow.ID, result.WorkflowStepID,
		"tiebreak on equal position must resolve to the lexicographically-smaller step id")
}

// --- AC-32a: start_agent:false is honored even when the resolved step
// itself carries an auto_start_agent on_enter action ---

func TestHandleHandoffTask_StartAgentFalseHonoredEvenWhenStepAutoStarts(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)
	ctx := context.Background()

	autoStartStep := seedWorkflowStep(t, ctx, f.workflowRepo, &workflowmodels.WorkflowStep{
		ID: "step-auto-start", WorkflowID: f.targetWorkflowID, Name: "Auto Start", Position: 1, IsStartStep: true, AllowManualMove: true,
		Events: workflowmodels.StepEvents{OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}}},
	})
	f.svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{autoStartStep.ID: autoStartStep}})

	payload := f.validPayload(map[string]interface{}{handoffFieldStartAgent: false})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, autoStartStep.ID, result.WorkflowStepID)
	assert.False(t, result.Started)

	sessions, err := f.svc.BatchGetSessionsForTasks(ctx, []string{result.TaskID})
	require.NoError(t, err)
	assert.Empty(t, sessions[result.TaskID], "no session may be created when start_agent is explicitly false")
	assert.Nil(t, launcher.getRequest(), "LaunchSession must never be called")
}

// --- AC-33: a replay against a since-moved delivery task reports its
// current workflow_step_id, not the step it was created in ---

func TestHandleHandoffTask_ReplayAgainstMovedDeliveryTaskReportsCurrentStep(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	ctx := context.Background()
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-moved-task"})

	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)
	require.Equal(t, f.targetStepID, result1.WorkflowStepID)

	movedStep := seedWorkflowStep(t, ctx, f.workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID: f.targetWorkflowID, Name: "In Review", Position: 2, AllowManualMove: true,
	})
	originalStep, err := f.workflowRepo.GetStep(ctx, f.targetStepID)
	require.NoError(t, err)
	f.svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{
		originalStep.ID: originalStep,
		movedStep.ID:    movedStep,
	}})

	_, err = f.svc.MoveTask(ctx, result1.TaskID, f.targetWorkflowID, movedStep.ID, 0)
	require.NoError(t, err)

	resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result2 := decodeHandoffResult(t, resp2)

	assert.Equal(t, handoffOutcomeFoundSettled, result2.Outcome)
	assert.Equal(t, result1.TaskID, result2.TaskID)
	assert.Equal(t, movedStep.ID, result2.WorkflowStepID, "must report the task's current step, not the original create-time step")
	assert.NotEqual(t, result1.WorkflowStepID, result2.WorkflowStepID)
}
