package backendapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/events"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func (a *workspaceAdminAdapter) requireEditableWorkflow(ctx context.Context, workspace, id string) error {
	if id == "" {
		return fmt.Errorf("workflow id is required")
	}
	if _, err := a.workspaceDeliveryWorkflow(ctx, workspace, id); err != nil {
		return err
	}
	return a.workflows.EnsureWorkflowMutable(ctx, id)
}

func (a *workspaceAdminAdapter) manageWorkflowConfiguration(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	if cmd.Action == adminCreate {
		return a.createWorkspaceWorkflow(ctx, workspace, cmd)
	}
	if cmd.Action == adminReorder {
		for _, id := range cmd.IDs {
			if err := a.requireEditableWorkflow(ctx, workspace, id); err != nil {
				return nil, err
			}
		}
		return nil, a.taskSvc.ReorderWorkflows(ctx, workspace, cmd.IDs)
	}
	if err := a.requireEditableWorkflow(ctx, workspace, cmd.ID); err != nil {
		return nil, err
	}
	switch cmd.Action {
	case adminUpdate:
		var req taskservice.UpdateWorkflowRequest
		if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
			return nil, err
		}
		if err := a.validateWorkspaceProfile(ctx, workspace, req.AgentProfileID); err != nil {
			return nil, err
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
			return nil, fmt.Errorf("name must not be empty")
		}
		return a.taskSvc.UpdateWorkflow(ctx, cmd.ID, &req)
	case adminDelete:
		return nil, a.taskSvc.DeleteWorkflow(ctx, cmd.ID)
	default:
		return nil, fmt.Errorf("workflow action must be create, update, delete or reorder")
	}
}

func (a *workspaceAdminAdapter) createWorkspaceWorkflow(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	var req taskservice.CreateWorkflowRequest
	if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
		return nil, err
	}
	if req.WorkspaceID != "" || req.Hidden {
		return nil, fmt.Errorf("workspace_id and hidden cannot be overridden")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.WorkflowTemplateID != nil && *req.WorkflowTemplateID != "" {
		if _, err := a.workflows.GetTemplate(ctx, *req.WorkflowTemplateID); err != nil {
			return nil, err
		}
	}
	req.WorkspaceID = workspace
	wf, err := a.taskSvc.CreateWorkflow(ctx, &req)
	if err != nil {
		return nil, err
	}
	// The task service creates template steps; publish them through the same
	// publisher used by UI mutations so open boards see the new columns.
	steps, err := a.workflows.ListStepsByWorkflow(ctx, wf.ID)
	if err != nil {
		return nil, err
	}
	a.stepEvents.PublishAll(ctx, events.WorkflowStepCreated, steps)
	return wf, nil
}

func (a *workspaceAdminAdapter) manageRepositoryConfiguration(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	if cmd.Action == adminCreate {
		var req taskservice.CreateRepositoryRequest
		if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
			return nil, err
		}
		if req.WorkspaceID != "" {
			return nil, fmt.Errorf("workspace_id cannot be overridden")
		}
		if strings.TrimSpace(req.Name) == "" {
			return nil, fmt.Errorf("name is required")
		}
		req.WorkspaceID = workspace
		return a.taskSvc.CreateRepository(ctx, &req)
	}
	repo, err := a.taskSvc.GetRepository(ctx, cmd.ID)
	if err != nil || repo.WorkspaceID != workspace {
		return nil, fmt.Errorf("select a repository in this workspace")
	}
	switch cmd.Action {
	case adminUpdate:
		var req taskservice.UpdateRepositoryRequest
		if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
			return nil, err
		}
		return a.taskSvc.UpdateRepository(ctx, cmd.ID, &req)
	case adminDelete:
		return nil, a.taskSvc.DeleteRepository(ctx, cmd.ID)
	default:
		return nil, fmt.Errorf("repository action must be create, update or delete")
	}
}
