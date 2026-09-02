package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
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
	seedHandoffWorkspace(t, f.repo, thirdWorkspaceID, "")
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

// --- AC-12/AC-12b: under real per-user workspace scoping, a workflow that
// exists but whose workspace the caller's owner cannot see must be refused
// identically (code and message) to a workflow that does not exist at all ---

func TestHandleHandoffTask_WorkflowInInvisibleWorkspaceGivesSameResponseAsMissing(t *testing.T) {
	const ownerID = "owner-handoff-scoped"
	const foreignOwnerID = "owner-handoff-foreign"
	f := newHandoffFixtureWithOwner(t, agentsettingsmodels.AgentRoleCEO, "", nil, ownerID)
	ctx := authn.WithIdentity(f.ctx(), authn.Identity{UserID: ownerID})

	payloadMissing := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: "wf-does-not-exist"})
	respMissing, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payloadMissing))
	require.NoError(t, err)
	assertWSError(t, respMissing, ws.ErrorCodeValidation)

	const foreignWorkspaceID = "ws-handoff-foreign-owner"
	const foreignWorkflowID = "wf-handoff-foreign-owner"
	seedHandoffWorkspace(t, f.repo, foreignWorkspaceID, foreignOwnerID)
	require.NoError(t, f.repo.CreateWorkflow(context.Background(), &models.Workflow{
		ID: foreignWorkflowID, WorkspaceID: foreignWorkspaceID, Name: "Foreign",
	}))

	payloadInvisible := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: foreignWorkflowID})
	respInvisible, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payloadInvisible))
	require.NoError(t, err)
	assertWSError(t, respInvisible, ws.ErrorCodeValidation)

	var epMissing, epInvisible ws.ErrorPayload
	require.NoError(t, json.Unmarshal(respMissing.Payload, &epMissing))
	require.NoError(t, json.Unmarshal(respInvisible.Payload, &epInvisible))
	assert.Equal(t, epMissing.Message, epInvisible.Message,
		"a workflow in a workspace the caller's owner cannot see must be indistinguishable from a nonexistent one")
	assert.NotContains(t, epInvisible.Message, foreignWorkspaceID)
	assert.NotContains(t, epInvisible.Message, foreignOwnerID)
}

// --- AC-12c: workflow_id naming the target workspace's own office workflow
// is refused, distinctly from an ordinary not-a-member workflow ---

func TestHandleHandoffTask_OwnOfficeWorkflowRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	officeWorkflowID, err := f.repo.EnsureOfficeWorkflow(context.Background(), f.targetWorkspaceID)
	require.NoError(t, err)

	payload := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: officeWorkflowID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, "office workflow")
}

// --- AC-15b: a workflow with no resolvable destination step is a
// Validation refusal, while a step-listing failure is an InternalError ---

func TestHandleHandoffTask_NoResolvableDestinationStepIsValidation(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const emptyWorkflowID = "wf-handoff-no-steps"
	require.NoError(t, f.repo.CreateWorkflow(context.Background(), &models.Workflow{
		ID: emptyWorkflowID, WorkspaceID: f.targetWorkspaceID, Name: "No Steps",
	}))

	payload := f.validPayload(map[string]interface{}{handoffFieldWorkflowID: emptyWorkflowID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, "no resolvable destination step")
}

func TestHandleHandoffTask_StepListingFailureIsInternalError(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	f.h.workflowCtrl = nil

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assertWSErrorContains(t, resp, "workflow steps are not available")
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
	require.Equal(t, f.targetWorkflowID, result1.WorkflowID)

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

// TestHandleHandoffTask_ReplayAgainstTaskMovedToDifferentWorkflowReportsCurrentWorkflow
// is the AC-33 sibling that actually changes workflow_id, not just
// workflow_step_id within the same workflow: the two fields are read off the
// stored task row independently, so a same-workflow step move alone cannot
// prove workflow_id is reported live rather than echoed from the create-time
// request args.
func TestHandleHandoffTask_ReplayAgainstTaskMovedToDifferentWorkflowReportsCurrentWorkflow(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	ctx := context.Background()
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-moved-workflow"})

	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)
	require.Equal(t, f.targetWorkflowID, result1.WorkflowID)
	require.Equal(t, f.targetStepID, result1.WorkflowStepID)

	const secondWorkflowID = "wf-handoff-second"
	require.NoError(t, f.repo.CreateWorkflow(ctx, &models.Workflow{
		ID: secondWorkflowID, WorkspaceID: f.targetWorkspaceID, Name: "Second Delivery Workflow",
	}))
	secondStep := seedWorkflowStep(t, ctx, f.workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID: secondWorkflowID, Name: "Second Start", Position: 1, IsStartStep: true, AllowManualMove: true,
	})
	originalStep, err := f.workflowRepo.GetStep(ctx, f.targetStepID)
	require.NoError(t, err)
	f.svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{
		originalStep.ID: originalStep,
		secondStep.ID:   secondStep,
	}})

	_, err = f.svc.MoveTask(ctx, result1.TaskID, secondWorkflowID, secondStep.ID, 0)
	require.NoError(t, err)

	resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result2 := decodeHandoffResult(t, resp2)

	assert.Equal(t, handoffOutcomeFoundSettled, result2.Outcome)
	assert.Equal(t, result1.TaskID, result2.TaskID)
	assert.Equal(t, secondWorkflowID, result2.WorkflowID, "must report the task's current workflow_id, not the create-time workflow")
	assert.Equal(t, secondStep.ID, result2.WorkflowStepID)
	assert.NotEqual(t, result1.WorkflowID, result2.WorkflowID)
}
