package backendapp

import (
	"context"
	"fmt"
	"strings"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func (a *taskCreatorAdapter) editWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if command.Title != nil && strings.TrimSpace(*command.Title) == "" {
		return fmt.Errorf("title must not be empty")
	}
	if command.Title == nil && command.Description == nil && command.Priority == nil && command.ParentID == nil {
		return fmt.Errorf("edit requires title, description, priority or parent_id")
	}
	if command.ParentID != nil && *command.ParentID != "" {
		parent, err := a.taskSvc.GetTask(ctx, *command.ParentID)
		if err != nil || parent.WorkspaceID != task.WorkspaceID || parent.IsEphemeral || parent.IsFromOffice {
			return fmt.Errorf("parent must be a delivery task in this workspace")
		}
	}
	_, err := a.taskSvc.UpdateTask(ctx, task.ID, &taskservice.UpdateTaskRequest{
		Title: command.Title, Description: command.Description, Priority: command.Priority, ParentID: command.ParentID,
	})
	return err
}

func (a *taskCreatorAdapter) moveWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.workflow == nil || command.WorkflowStepID == "" {
		return fmt.Errorf("move requires a workflow_step_id")
	}
	workflowID := command.WorkflowID
	opts := taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorAgent, StepHistoryTrigger: wfmodels.StepTransitionTriggerPluginMove}
	if workflowID == "" {
		workflowID = task.WorkflowID
		opts.ExpectedWorkflowID = &task.WorkflowID
	}
	if _, err := a.workspaceDeliveryWorkflow(ctx, task.WorkspaceID, workflowID); err != nil {
		return err
	}
	step, err := a.workflow.GetStep(ctx, command.WorkflowStepID)
	if err != nil || step.WorkflowID != workflowID || !step.AllowManualMove {
		return fmt.Errorf("select a step in the delivery workflow that allows manual moves")
	}
	if task.WorkflowStepID != step.ID {
		if err := validateOrchestratedCompletion(ctx, &Repositories{Workflow: a.workflow}, task.WorkflowStepID, task.ID); err != nil {
			return err
		}
	}
	position := 0
	if command.Position != nil {
		position = *command.Position
	}
	if position < 0 {
		return fmt.Errorf("position must not be negative")
	}
	_, err = a.taskSvc.MoveTaskWithOptions(ctx, task.ID, workflowID, step.ID, position, opts)
	return err
}
