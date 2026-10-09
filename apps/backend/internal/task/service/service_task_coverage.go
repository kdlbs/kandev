package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// WorkflowTaskCoverage describes the collection already read by the authorized caller.
func (s *Service) WorkflowTaskCoverage(workflow *models.Workflow, total, returned int) *models.TaskCoverage {
	profile := "server_only"
	if provider, ok := s.tasks.(interface{ SidebarTaskOrderingProfile() string }); ok {
		profile = provider.SidebarTaskOrderingProfile()
	}
	return &models.TaskCoverage{
		WorkspaceID: workflow.WorkspaceID, WorkflowID: workflow.ID,
		Membership: "active", Total: total, Complete: total == returned, OrderingProfile: profile,
	}
}

// TaskWorkflowCoverage names all scopes that must be present for workspace-wide reuse.
func (s *Service) TaskWorkflowCoverage(ctx context.Context, workspaceID string) (*models.TaskWorkflowCoverage, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	provider, ok := s.tasks.(interface {
		SidebarTaskWorkflowIDs(context.Context, string) ([]string, error)
	})
	if !ok || workspaceID == "" {
		return nil, nil
	}
	ids, err := provider.SidebarTaskWorkflowIDs(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &models.TaskWorkflowCoverage{WorkspaceID: workspaceID, WorkflowIDs: ids, Complete: true}, nil
}
