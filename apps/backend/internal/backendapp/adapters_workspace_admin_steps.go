package backendapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func (a *workspaceAdminAdapter) manageStepConfiguration(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	if cmd.Action == adminCreate {
		return a.createWorkspaceStep(ctx, workspace, cmd)
	}
	if cmd.Action == adminReorder {
		return a.reorderWorkspaceSteps(ctx, workspace, cmd)
	}
	step, err := a.workflows.GetStep(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := a.requireEditableWorkflow(ctx, workspace, step.WorkflowID); err != nil {
		return nil, err
	}
	ctrl := controller.NewController(a.workflows)
	switch cmd.Action {
	case adminUpdate:
		return a.updateWorkspaceStep(ctx, workspace, cmd)
	case adminDelete:
		if err := ctrl.DeleteStep(ctx, cmd.ID); err != nil {
			return nil, err
		}
		a.stepEvents.Publish(ctx, events.WorkflowStepDeleted, step)
		// Native deletion clears references from remaining steps.
		remaining, err := a.workflows.ListStepsByWorkflow(ctx, step.WorkflowID)
		if err != nil {
			return nil, err
		}
		a.stepEvents.PublishAll(ctx, events.WorkflowStepUpdated, remaining)
		return nil, nil
	default:
		return nil, fmt.Errorf("step action must be create, update, delete or reorder")
	}
}

func (a *workspaceAdminAdapter) createWorkspaceStep(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	var req controller.CreateStepRequest
	if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
		return nil, err
	}
	if err := a.requireEditableWorkflow(ctx, workspace, req.WorkflowID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := a.validateWorkspaceStepConfiguration(ctx, workspace, req.AgentProfileID, req.Events); err != nil {
		return nil, err
	}
	result, err := controller.NewController(a.workflows).CreateStep(ctx, req)
	if err != nil {
		return nil, err
	}
	a.stepEvents.PublishAll(ctx, events.WorkflowStepUpdated, result.DemotedStartSteps)
	a.stepEvents.Publish(ctx, events.WorkflowStepCreated, result.Step)
	return result.Step, nil
}

func (a *workspaceAdminAdapter) updateWorkspaceStep(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	var req controller.UpdateStepRequest
	if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
		return nil, err
	}
	if req.ID != "" {
		return nil, fmt.Errorf("use id outside configuration")
	}
	req.ID = cmd.ID
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	if err := a.validateWorkspaceStepConfiguration(ctx, workspace, req.AgentProfileID, req.Events); err != nil {
		return nil, err
	}
	result, err := controller.NewController(a.workflows).UpdateStep(ctx, req)
	if err != nil {
		return nil, err
	}
	a.stepEvents.PublishAll(ctx, events.WorkflowStepUpdated, result.DemotedStartSteps)
	a.stepEvents.Publish(ctx, events.WorkflowStepUpdated, result.Step)
	return result.Step, nil
}

func (a *workspaceAdminAdapter) validateWorkspaceStepConfiguration(ctx context.Context, workspace string, profile *string, stepEvents *wfmodels.StepEvents) error {
	if err := a.validateWorkspaceProfile(ctx, workspace, profile); err != nil {
		return err
	}
	if stepEvents == nil {
		return nil
	}
	for _, action := range stepEvents.OnEnter {
		if action.Type == wfmodels.OnEnterRunCodeReview {
			id, _ := action.Config[wfmodels.ReviewAgentProfileConfigKey].(string)
			if err := a.validateWorkspaceProfile(ctx, workspace, &id); err != nil {
				return err
			}
		}
	}
	for _, id := range wfmodels.CollectStepEventReferences(*stepEvents).TaskIDs {
		task, err := a.taskSvc.GetTask(ctx, id)
		if err != nil || task.WorkspaceID != workspace || task.IsEphemeral || task.IsFromOffice {
			return fmt.Errorf("event task must be a delivery task in this workspace")
		}
	}
	return nil
}

func (a *workspaceAdminAdapter) reorderWorkspaceSteps(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	if err := a.requireEditableWorkflow(ctx, workspace, cmd.WorkflowID); err != nil {
		return nil, err
	}
	req := controller.ReorderStepsRequest{WorkflowID: cmd.WorkflowID, StepIDs: cmd.IDs}
	if err := controller.NewController(a.workflows).ReorderSteps(ctx, req); err != nil {
		return nil, err
	}
	steps, err := a.workflows.ListStepsByWorkflow(ctx, cmd.WorkflowID)
	if err != nil {
		return nil, err
	}
	a.stepEvents.PublishAll(ctx, events.WorkflowStepUpdated, steps)
	return steps, nil
}
