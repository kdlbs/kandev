package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TaskMutationIdentity supplies only the placement/version needed by internal
// mutation adapters. It does not expose a general task read with control rights.
type TaskMutationIdentity struct {
	WorkspaceID     string
	WorkflowID      string
	ResourceVersion string
}

func (s *Service) GetTaskMutationIdentity(ctx context.Context, taskID string) (*TaskMutationIdentity, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, repoerrors.ErrTaskNotFound
	}
	return &TaskMutationIdentity{
		WorkspaceID: task.WorkspaceID, WorkflowID: task.WorkflowID,
		ResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

// GetTaskManagementClaimForMutation retains the normal claim fence, without
// requiring the independent collaboration read grant for an internal preflight.
func (s *Service) GetTaskManagementClaimForMutation(ctx context.Context, taskID string) (*models.TaskManagementClaim, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if err := s.requireManagementClaimTask(ctx, taskID); err != nil {
		return nil, err
	}
	claims, ok := s.tasks.(repository.TaskManagementClaimRepository)
	if !ok {
		return nil, errTaskManagementClaimsUnavailable
	}
	return claims.GetTaskManagementClaim(ctx, taskID)
}
