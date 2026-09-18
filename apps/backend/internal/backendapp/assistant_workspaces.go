package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/authz"
	shared "github.com/kandev/kandev/internal/orchestration/models"
)

func (a *taskCreatorAdapter) WorkspaceGrantAccess(ctx context.Context, workspace string, coordinate bool) (shared.WorkspaceGrantOption, error) {
	scope := authz.ScopeWorkspaceRead
	if coordinate {
		scope = authz.ScopeTaskWrite
	}
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, scope); err != nil {
		return shared.WorkspaceGrantOption{}, err
	}
	row, err := a.taskSvc.GetWorkspace(ctx, workspace)
	if err != nil {
		return shared.WorkspaceGrantOption{}, err
	}
	return shared.WorkspaceGrantOption{ID: row.ID, Name: row.Name}, nil
}
func (a *taskCreatorAdapter) WorkspaceGrantOptions(ctx context.Context) ([]shared.WorkspaceGrantOption, error) {
	workspaces, err := a.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	rows := []shared.WorkspaceGrantOption{}
	for _, row := range workspaces {
		rows = append(rows, shared.WorkspaceGrantOption{ID: row.ID, Name: row.Name})
	}
	return rows, nil
}
