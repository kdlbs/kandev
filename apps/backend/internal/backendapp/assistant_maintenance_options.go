package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/authz"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"path/filepath"
)

func (a *taskCreatorAdapter) MaintenanceOptions(ctx context.Context, workspace string) ([]shared.MaintenanceOption, error) {
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	repositories, err := a.taskSvc.ListRepositories(ctx, workspace)
	if err != nil {
		return nil, err
	}
	rows := []shared.MaintenanceOption{}
	for _, r := range repositories {
		if r.WorkspaceID == workspace && filepath.IsAbs(r.LocalPath) {
			rows = append(rows, shared.MaintenanceOption{ID: "repository:" + r.ID, ResourceID: r.ID, Name: r.Name, Kind: "repository"})
		}
	}
	workflowOptions, err := a.maintenanceWorkflowOptions(ctx, workspace)
	return append(rows, workflowOptions...), err
}

func (a *taskCreatorAdapter) maintenanceWorkflowOptions(ctx context.Context, workspace string) ([]shared.MaintenanceOption, error) {
	rows := []shared.MaintenanceOption{}
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspace, false)
	if err != nil {
		return nil, err
	}
	eligible := map[string]bool{}
	for _, w := range workflows {
		if !w.Hidden && (w.WorkflowTemplateID == nil || *w.WorkflowTemplateID == "") {
			eligible[w.ID] = true
			rows = append(rows, shared.MaintenanceOption{ID: "workflow:" + w.ID, ResourceID: w.ID, Name: w.Name, Kind: "workflow"})
		}
	}
	if a.workflow == nil {
		return rows, nil
	}
	steps, err := a.workflow.ListStepsByWorkspaceID(ctx, workspace)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if eligible[step.WorkflowID] && (step.IsStartStep || step.AllowManualMove) && len(step.Events.OnEnter) == 0 {
			rows = append(rows, shared.MaintenanceOption{ID: "step:" + step.ID, ResourceID: step.ID, Name: step.Name, Kind: "step", WorkflowID: step.WorkflowID})
		}
	}
	return rows, nil
}
