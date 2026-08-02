package azuredevops

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) AssociateTaskWorkItem(ctx context.Context, workspaceID, taskID, projectID string, workItemID int) (*TaskWorkItem, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(projectID) == "" || workItemID <= 0 {
		return nil, fmt.Errorf("%w: task, project, and positive work item ID are required", ErrInvalidTaskPRAssociation)
	}
	owned, err := s.store.TaskBelongsToWorkspace(ctx, taskID, workspaceID)
	if err != nil || !owned {
		return nil, fmt.Errorf("%w: task belongs to another workspace", ErrInvalidTaskPRAssociation)
	}
	detail, err := s.GetWorkItemDetailForWorkspace(ctx, workspaceID, projectID, workItemID)
	if err != nil {
		return nil, fmt.Errorf("validate Azure work item: %w", err)
	}
	if detail.Project != "" && detail.Project != projectID {
		return nil, fmt.Errorf("%w: work item belongs to another project", ErrInvalidTaskPRAssociation)
	}
	row := &TaskWorkItem{TaskID: taskID, WorkspaceID: workspaceID, ProjectID: projectID, WorkItemID: detail.ID, WorkItemURL: detail.WebURL, Title: detail.Title, State: detail.State, Type: detail.Type}
	if err := s.store.UpsertTaskWorkItem(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert Azure task work item: %w", err)
	}
	return row, nil
}

func (s *Service) ListTaskWorkItemsByWorkspace(ctx context.Context, workspaceID string) (map[string][]*TaskWorkItem, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListTaskWorkItemsByWorkspace(ctx, workspaceID)
}
