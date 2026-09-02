package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// handoffSourceMetadata builds the handoff_source metadata block a real
// handoff would have written on the delivery task, so a manually pre-created
// task looks indistinguishable from one this handler actually produced.
func handoffSourceMetadata(f *handoffFixture, handedOffAt string) map[string]interface{} {
	return map[string]interface{}{
		models.MetaKeyAgentProfileID:    f.targetAgentProfileID,
		models.MetaKeyExecutorProfileID: f.targetExecProfileID,
		models.MetaKeyHandoffSource: map[string]interface{}{
			handoffSourceTaskIDKey:    f.sourceTaskID,
			"source_workspace_id":     f.sourceWorkspaceID,
			"source_session_id":       f.sourceSessionID,
			"source_agent_profile_id": f.callerAgentProfileID,
			handoffHandedOffAtKey:     handedOffAt,
		},
	}
}

// --- AC-24b: a replay observes an existing-but-unsettled delivery task ---

func TestHandleHandoffTask_FoundUnsettledOutcomeReportsCreationCompleteFalse(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const externalID = "ext-unsettled-race"
	handedOffAt := formatHandoffTimestamp(time.Now().UTC())

	preCreated, err := f.svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID:    f.targetWorkspaceID,
		WorkflowID:     f.targetWorkflowID,
		WorkflowStepID: f.targetStepID,
		Title:          "Deliver the thing",
		Description:    "Do the work",
		ExternalID:     externalID,
		Metadata:       handoffSourceMetadata(f, handedOffAt),
	})
	require.NoError(t, err)
	require.Equal(t, service.CreateTaskOutcomeCreated, preCreated.Outcome)
	// Deliberately never settled: SettleExternalID is the handler's job after
	// its own create, which is exactly the in-flight window AC-24b describes.

	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: externalID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeFoundUnsettled, result.Outcome)
	assert.Equal(t, preCreated.Task.ID, result.TaskID)
	assert.False(t, result.CreationComplete, "AC-24b: an unsettled find must not report creation_complete")
	assert.False(t, result.Started)
	assert.Contains(t, result.Message, "another create may still be finishing")
	assert.True(t, result.ReverseLinkRecorded, "the reverse link is still repaired even when unsettled")
}

// --- AC-24c/AC-32: created_identity_lost skips the launch with no start_error ---

func TestHandleHandoffCreatedOutcome_IdentityLostSkipsStartAndReportsMessage(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)
	const externalID = "ext-identity-lost"
	handedOffAt := formatHandoffTimestamp(time.Now().UTC())

	created, err := f.svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID:    f.targetWorkspaceID,
		WorkflowID:     f.targetWorkflowID,
		WorkflowStepID: f.targetStepID,
		Title:          "Deliver the thing",
		Description:    "Do the work",
		ExternalID:     externalID,
		Metadata:       handoffSourceMetadata(f, handedOffAt),
	})
	require.NoError(t, err)
	task := created.Task
	require.Equal(t, externalID, task.ExternalID, "precondition: the in-memory task still names the identity that is about to be released")

	// Simulate the release race SettleExternalID's doc comment describes: the
	// identity is freed (e.g. by an operator) between this handler's own
	// create and its settlement call, which is the only way to reach
	// created_identity_lost deterministically without racing goroutines
	// against an internal synchronous call sequence.
	releasedTask, err := f.repo.ReleaseTaskExternalID(context.Background(), f.targetWorkspaceID, externalID)
	require.NoError(t, err)
	require.NotNil(t, releasedTask)

	principal := mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	}
	session, err := f.repo.GetTaskSession(context.Background(), f.sourceSessionID)
	require.NoError(t, err)
	args := &handoffTaskArgs{
		TargetWorkspaceID: f.targetWorkspaceID,
		WorkflowID:        f.targetWorkflowID,
		Title:             "Deliver the thing",
		Prompt:            "Do the work",
		AgentProfileID:    f.targetAgentProfileID,
		ExecutorProfileID: f.targetExecProfileID,
		StartAgent:        true,
		ExternalID:        externalID,
	}
	resolved := &handoffResolvedResources{WorkflowStepID: f.targetStepID}
	msg := makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil))

	resp, err := f.h.handleHandoffCreatedOutcome(f.ctx(), msg, principal, session, args, resolved, task, handedOffAt)
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreatedIdentityLost, result.Outcome)
	assert.True(t, result.CreationComplete, "the task itself was created; only the identity was lost")
	assert.False(t, result.Started, "AC-32: identity_lost must never dispatch the launch")
	assert.Empty(t, result.StartError, "AC-32: skipping the launch is not itself a start failure")
	assert.Nil(t, launcher.getRequest(), "LaunchSession must never be called")
	assert.Contains(t, result.Message, externalID)
	assert.Contains(t, result.Message, "record task_id rather than replaying")
}

// --- AC-24d: created_identity_lost reports the settlement survivor, not the stale created row ---

func TestHandleHandoffCreatedOutcome_IdentityLostReportsSurvivorWorkflowStep(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)
	const externalID = "ext-identity-lost-survivor"
	handedOffAt := formatHandoffTimestamp(time.Now().UTC())

	created, err := f.svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID:    f.targetWorkspaceID,
		WorkflowID:     f.targetWorkflowID,
		WorkflowStepID: f.targetStepID,
		Title:          "Deliver the thing",
		Description:    "Do the work",
		ExternalID:     externalID,
		Metadata:       handoffSourceMetadata(f, handedOffAt),
	})
	require.NoError(t, err)
	// Captured before the step move below: this is the stale in-memory row
	// the handler must NOT trust for WorkflowStepID once settlement reports
	// a different survivor.
	task := created.Task

	releasedTask, err := f.repo.ReleaseTaskExternalID(context.Background(), f.targetWorkspaceID, externalID)
	require.NoError(t, err)
	require.NotNil(t, releasedTask)

	// Move the task to a different step in the DB between this handler's own
	// create and its settlement call, simulating AC-24d's "different rows"
	// case: the settlement reload must observe the move even though `task`
	// (captured above) still names the original step.
	movedStep := seedWorkflowStep(t, context.Background(), f.workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID: f.targetWorkflowID, Name: "Moved", Position: 2, AllowManualMove: true,
	})
	moved, err := f.repo.GetTask(context.Background(), task.ID)
	require.NoError(t, err)
	moved.WorkflowStepID = movedStep.ID
	require.NoError(t, f.repo.UpdateTask(context.Background(), moved))
	require.NotEqual(t, task.WorkflowStepID, movedStep.ID, "precondition: the captured task and the DB row must now disagree")

	principal := mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	}
	session, err := f.repo.GetTaskSession(context.Background(), f.sourceSessionID)
	require.NoError(t, err)
	args := &handoffTaskArgs{
		TargetWorkspaceID: f.targetWorkspaceID,
		WorkflowID:        f.targetWorkflowID,
		Title:             "Deliver the thing",
		Prompt:            "Do the work",
		AgentProfileID:    f.targetAgentProfileID,
		ExecutorProfileID: f.targetExecProfileID,
		StartAgent:        true,
		ExternalID:        externalID,
	}
	resolved := &handoffResolvedResources{WorkflowStepID: f.targetStepID}
	msg := makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil))

	resp, err := f.h.handleHandoffCreatedOutcome(f.ctx(), msg, principal, session, args, resolved, task, handedOffAt)
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreatedIdentityLost, result.Outcome)
	assert.Equal(t, task.ID, result.TaskID, "the task id itself is unaffected by the step move")
	assert.Equal(t, movedStep.ID, result.WorkflowStepID,
		"AC-24d: response must report the settlement survivor's step, not the stale created-row's step")
}

// --- AC-24d/R-F39: a genuine SettleExternalID failure halts D3b entirely ---

func TestHandleHandoffCreatedOutcome_SettlementErrorHaltsWithNoReverseLinkActivityOrLaunch(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)
	dashSvc, activityRepo, _ := newHandoffDashboardService(t, "")
	f.h.SetDashboardService(dashSvc)
	const externalID = "ext-settle-fails"
	handedOffAt := formatHandoffTimestamp(time.Now().UTC())

	created, err := f.svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID:    f.targetWorkspaceID,
		WorkflowID:     f.targetWorkflowID,
		WorkflowStepID: f.targetStepID,
		Title:          "Deliver the thing",
		Description:    "Do the work",
		ExternalID:     externalID,
		Metadata:       handoffSourceMetadata(f, handedOffAt),
	})
	require.NoError(t, err)
	task := created.Task

	principal := mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	}
	session, err := f.repo.GetTaskSession(context.Background(), f.sourceSessionID)
	require.NoError(t, err)
	args := &handoffTaskArgs{
		TargetWorkspaceID: f.targetWorkspaceID,
		WorkflowID:        f.targetWorkflowID,
		Title:             "Deliver the thing",
		Prompt:            "Do the work",
		AgentProfileID:    f.targetAgentProfileID,
		ExecutorProfileID: f.targetExecProfileID,
		StartAgent:        true,
		ExternalID:        externalID,
	}
	resolved := &handoffResolvedResources{WorkflowStepID: f.targetStepID}
	msg := makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil))

	// The settlement UPDATE (task_external_id.go's SettleTaskExternalID) is
	// the only thing handleHandoffCreatedOutcome does before the R-F39 halt
	// point, so breaking the tasks table here forces a genuine settlement
	// error without needing a fake/mock service seam.
	_, err = f.db.Exec("DROP TABLE tasks")
	require.NoError(t, err, "precondition: must be able to break the tasks table out from under the fixture")

	resp, err := f.h.handleHandoffCreatedOutcome(f.ctx(), msg, principal, session, args, resolved, task, handedOffAt)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assertWSErrorContains(t, resp, task.ID)

	assert.Empty(t, activityRepo.entries, "R-F39: a settlement error must skip activity logging entirely")
	assert.Nil(t, launcher.getRequest(), "R-F39: a settlement error must skip the launch entirely")
}
