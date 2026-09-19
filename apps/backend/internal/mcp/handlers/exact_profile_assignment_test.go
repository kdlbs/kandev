package handlers

import (
	"context"
	"encoding/json"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleAssignExactTaskProfileForwardsGuardedCoordinatorRequest(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow", WorkspaceID: "workspace", Name: "Workflow"}))
	for _, step := range []*wfmodels.WorkflowStep{
		{ID: "step-done", WorkflowID: "workflow", Name: "Done"},
		{ID: "step-work", WorkflowID: "workflow", Name: "Work"},
	} {
		require.NoError(t, workflowRepo.CreateStep(ctx, step))
	}
	for _, task := range []*models.Task{
		{ID: "coordinator", WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step-done", State: v1.TaskStateCompleted},
		{ID: "target", WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step-done", State: v1.TaskStateCompleted},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	assigner := &recordingExactTaskProfileAssigner{}
	h := NewHandlers(taskSvc, workflowCtrl, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetExactTaskProfileAssigner(assigner)
	payload, err := json.Marshal(map[string]interface{}{
		"task_id": "target", "agent_profile_id": "profile-sol", "expected_model": "gpt-5.6-sol",
		"expected_task_state": v1.TaskStateCompleted, "expected_workflow_step_id": "step-done",
		"target_workflow_step_id": "step-work", "expected_assignment_generation": 0,
		"sender_task_id": "coordinator", "sender_session_id": "coordinator-session",
	})
	require.NoError(t, err)
	principal := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "automation", WorkspaceID: "workspace", CallerTaskID: "coordinator",
		CallerSessionID: "coordinator-session", Surface: mcpprofile.SurfaceAutomation,
	})

	response, err := h.handleAssignExactTaskProfile(principal, &ws.Message{
		ID: "request-1", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload,
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, orchestrator.ExactTaskProfileAssignmentRequest{
		TaskID: "target", AgentProfileID: "profile-sol", ExpectedModel: "gpt-5.6-sol", Generation: 1,
		ExpectedWorkflowID: "workflow", ExpectedWorkflowStepID: "step-done", ExpectedTaskState: v1.TaskStateCompleted,
	}, assigner.request)
}

type recordingExactTaskProfileAssigner struct {
	request orchestrator.ExactTaskProfileAssignmentRequest
}

func (a *recordingExactTaskProfileAssigner) AssignExactTaskProfile(
	_ context.Context, request orchestrator.ExactTaskProfileAssignmentRequest,
) (*orchestrator.ExactProfileLaunchDecision, error) {
	a.request = request
	return &orchestrator.ExactProfileLaunchDecision{
		AgentProfileID: request.AgentProfileID, Generation: request.Generation,
		Revision: 42, Model: request.ExpectedModel, Changed: true,
	}, nil
}

func TestHandleAssignExactTaskProfileRejectsOrdinaryCrossTaskCaller(t *testing.T) {
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetExactTaskProfileAssigner(&recordingExactTaskProfileAssigner{})
	payload, err := json.Marshal(map[string]interface{}{
		"task_id": "task-foreign", "agent_profile_id": "profile-sol", "expected_model": "gpt-5.6-sol",
		"expected_task_state": v1.TaskStateCompleted, "expected_workflow_step_id": "step-done",
		"target_workflow_step_id": "step-work", "expected_assignment_generation": 0,
		"sender_task_id": "ordinary", "sender_session_id": "ordinary-session",
	})
	require.NoError(t, err)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace", CallerTaskID: "ordinary", CallerSessionID: "ordinary-session",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	response, err := h.handleAssignExactTaskProfile(ctx, &ws.Message{
		ID: "request-foreign", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload,
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
}
