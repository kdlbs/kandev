package azuredevops

import (
	"context"
	"fmt"
	"strings"
)

// WorkItemAssignmentRequest is the intentionally narrow mutation surface for
// the detail view. Azure work-item fields stay provider-owned; Kandev only
// changes the assignee and relies on board mutations for status changes.
type WorkItemAssignmentRequest struct {
	Revision       int     `json:"revision"`
	AssigneeAction *string `json:"assigneeAction,omitempty"`

	resolvedAssignee    string
	hasResolvedAssignee bool
}

type workItemMutationClient interface {
	UpdateWorkItem(ctx context.Context, projectID string, id int, request WorkItemAssignmentRequest) (*WorkItem, error)
}

func (s *Service) UpdateWorkItemForWorkspace(ctx context.Context, workspaceID, projectID string, id int, request WorkItemAssignmentRequest) (*WorkItem, error) {
	if strings.TrimSpace(projectID) == "" || id <= 0 {
		return nil, fmt.Errorf("%w: project and positive work item id required", ErrInvalidConfig)
	}
	if err := validateWorkItemAssignment(request); err != nil {
		return nil, err
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	writer, ok := client.(workItemMutationClient)
	if !ok {
		return nil, fmt.Errorf("%w: work item assignment is unavailable", ErrNotConfigured)
	}
	if request.AssigneeAction == nil {
		return writer.UpdateWorkItem(ctx, projectID, id, request)
	}
	if *request.AssigneeAction != assignCurrentUserAction {
		request.hasResolvedAssignee = true
		return writer.UpdateWorkItem(ctx, projectID, id, request)
	}
	identityReader, available := client.(interface {
		GetCurrentIdentity(context.Context) (*Identity, error)
	})
	if !available {
		return nil, fmt.Errorf("%w: current identity is unavailable", ErrNotConfigured)
	}
	identity, identityErr := identityReader.GetCurrentIdentity(ctx)
	if identityErr != nil {
		return nil, identityErr
	}
	assignee := strings.TrimSpace(identity.UniqueName)
	if assignee == "" {
		assignee = strings.TrimSpace(identity.DisplayName)
	}
	if assignee == "" {
		return nil, fmt.Errorf("%w: current identity has no assignable name", ErrInvalidConfig)
	}
	request.resolvedAssignee = assignee
	request.hasResolvedAssignee = true
	return writer.UpdateWorkItem(ctx, projectID, id, request)
}

func validateWorkItemAssignment(request WorkItemAssignmentRequest) error {
	if request.Revision <= 0 {
		return fmt.Errorf("%w: revision required", ErrInvalidConfig)
	}
	if request.AssigneeAction == nil || (*request.AssigneeAction != assignCurrentUserAction && *request.AssigneeAction != unassignAction) {
		return fmt.Errorf("%w: an assign_current_user or unassign action is required", ErrInvalidConfig)
	}
	return nil
}
