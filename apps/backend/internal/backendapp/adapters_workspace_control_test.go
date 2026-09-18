package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceOrchestratorNativeTaskLifecycle(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for i, id := range []string{"backlog", "progress", "done"} {
		require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: id, WorkflowID: wf.ID, Name: id, Position: i, AllowManualMove: true}))
	}
	id, err := a.CreateWorkspaceTask(ctx, shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", ChiefID: "chief", WorkflowID: wf.ID, Title: "Synthetic task", Description: "Initial description", ExecutionMode: "execute", DirectProfile: true})
	require.NoError(t, err)
	command := func(body string) error {
		var command shared.WorkspaceTaskCommand
		require.NoError(t, json.Unmarshal([]byte(body), &command))
		command.WorkspaceID, command.ChiefID, command.TaskID, command.DirectProfile = "ws-1", "chief", id, true
		return a.ManageWorkspaceTask(ctx, command)
	}
	require.NoError(t, command(`{"action":"edit","title":"Updated synthetic task","description":"Revised description","priority":"high"}`))
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Updated synthetic task", task.Title)
	require.Equal(t, "Revised description", task.Description)
	require.Equal(t, "high", task.Priority)
	require.NoError(t, command(`{"action":"move","workflow_step_id":"progress"}`))
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "progress", task.WorkflowStepID)
	require.NoError(t, command(`{"action":"archive"}`))
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, task.ArchivedAt)
	require.NoError(t, command(`{"action":"delete"}`))
	_, err = svc.GetTask(ctx, id)
	require.Error(t, err)
}

func TestWorkspaceTaskMutationsRejectForeignAndConversationTasks(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	for _, task := range []*models.Task{
		{ID: "foreign-control", WorkspaceID: "other", Title: "Foreign"},
		{ID: "chat-control", WorkspaceID: "ws-1", Title: "Conversation", IsEphemeral: true},
	} {
		require.NoError(t, a.taskRepo.CreateTask(ctx, task))
		for _, action := range []string{"edit", "move", "archive", "delete"} {
			require.Error(t, a.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", TaskID: task.ID, Action: action, DirectProfile: true}))
		}
		_, err := a.taskRepo.GetTask(ctx, task.ID)
		require.NoError(t, err)
	}
}

func TestWorkspaceMoveHonorsReviewAndTargetPolicy(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for i, id := range []string{"review", "done", "automatic"} {
		require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: id, WorkflowID: wf.ID, Name: id, Position: i, AllowManualMove: id != "automatic"}))
	}
	task := &models.Task{ID: "review-control", WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "review", Title: "Synthetic review", State: "REVIEW"}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	participant := &wfmodels.WorkflowStepParticipant{StepID: "review", Role: wfmodels.ParticipantRoleReviewer, AgentProfileID: "reviewer", DecisionRequired: true}
	require.NoError(t, a.workflow.UpsertStepParticipant(ctx, participant))
	cmd := shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", TaskID: task.ID, Action: "move", WorkflowStepID: "done", DirectProfile: true}
	require.ErrorContains(t, a.ManageWorkspaceTask(ctx, cmd), "pending")
	current, err := svc.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "review", current.WorkflowStepID)
	require.NoError(t, a.workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: task.ID, StepID: "review", ParticipantID: participant.ID, Decision: "approved"}))
	cmd.WorkflowStepID = "automatic"
	require.ErrorContains(t, a.ManageWorkspaceTask(ctx, cmd), "manual moves")
	cmd.WorkflowStepID = "done"
	require.NoError(t, a.ManageWorkspaceTask(ctx, cmd))
}
