package backendapp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (a *taskCreatorAdapter) ValidateMaintenanceScope(ctx context.Context, workspace string, scope models.MaintenanceScope) (string, error) {
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, authz.ScopeTaskWrite); err != nil {
		return "", err
	}
	repo, err := a.taskSvc.GetRepository(ctx, scope.RepositoryID)
	if err != nil || repo.WorkspaceID != workspace || !filepath.IsAbs(repo.LocalPath) {
		return "", fmt.Errorf("maintenance requires a local repository in this workspace")
	}
	if err = a.validateMaintenanceWorkflow(ctx, workspace, scope); err != nil {
		return "", err
	}
	return repo.LocalPath, nil
}

func (a *taskCreatorAdapter) validateMaintenanceWorkflow(ctx context.Context, workspace string, scope models.MaintenanceScope) error {
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspace, false)
	if err != nil {
		return err
	}
	eligible := false
	for _, workflow := range workflows {
		if workflow.ID == scope.WorkflowID && !workflow.Hidden && (workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID == "") {
			eligible = true
		}
	}
	if !eligible || a.workflow == nil {
		return fmt.Errorf("maintenance requires an ordinary workflow without contribution preparation")
	}
	step, err := a.workflow.GetStep(ctx, scope.WorkflowStepID)
	if err != nil || step.WorkflowID != scope.WorkflowID || (!step.IsStartStep && !step.AllowManualMove) || len(step.Events.OnEnter) > 0 {
		return fmt.Errorf("maintenance requires an accessible entry step without automatic actions")
	}
	return nil
}
