package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var errTaskManagementClaimsUnavailable = errors.New("task management claims are unavailable")

// GetTaskManagementClaim returns the current owner projection. A task with no
// prior claim has no claim row and returns nil.
func (s *Service) GetTaskManagementClaim(ctx context.Context, taskID string) (*models.TaskManagementClaim, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
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

// ListTaskManagementClaimHistory returns the bounded audit history for native
// task-detail recovery controls.
func (s *Service) ListTaskManagementClaimHistory(ctx context.Context, taskID string) ([]*models.TaskManagementClaimHistory, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if err := s.requireManagementClaimTask(ctx, taskID); err != nil {
		return nil, err
	}
	claims, ok := s.tasks.(repository.TaskManagementClaimRepository)
	if !ok {
		return nil, errTaskManagementClaimsUnavailable
	}
	return claims.ListTaskManagementClaimHistory(ctx, taskID)
}

func (s *Service) requireManagementClaimTask(ctx context.Context, taskID string) error {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

// ChangeTaskManagementClaim is shared by the exact Host methods and native
// human takeover API. The repository performs task-version and claim-version
// comparisons together with the write, so no caller preflight can authorize a
// claim after it has changed.
func (s *Service) ChangeTaskManagementClaim(ctx context.Context, taskID string, change models.TaskManagementClaimChange) (*models.TaskManagementClaim, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	lock := s.managementClaimLocks.lockFor(taskID)
	lock.Lock()
	defer lock.Unlock()
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if change.TaskID != "" && change.TaskID != taskID {
		return nil, repository.ErrTaskManagementClaimConflict
	}
	if change.WorkspaceID == "" {
		change.WorkspaceID = task.WorkspaceID
	}
	if change.WorkspaceID != task.WorkspaceID {
		return nil, repository.ErrTaskVersionConflict
	}
	change.TaskID = taskID
	claims, ok := s.tasks.(repository.TaskManagementClaimRepository)
	if !ok {
		return nil, errTaskManagementClaimsUnavailable
	}
	return claims.ChangeTaskManagementClaim(ctx, change)
}

// WithTaskManagementClaimFence serializes a task-scoped effect against claim
// acquisition, transfer, and release. The callback runs while the task's
// ownership lock is held, so a takeover cannot pass the check and race the
// protected execution effect.
//
//nolint:cyclop // The ownership lock spans the guard and protected effect to serialize takeovers.
func (s *Service) WithTaskManagementClaimFence(
	ctx context.Context,
	taskID, installationID, instanceKey string,
	generation int64,
	effect func() error,
) error {
	if taskID == "" || installationID == "" || effect == nil || generation < 0 ||
		(generation == 0 && instanceKey != "") {
		return repoerrors.ErrTaskManagementClaimConflict
	}
	lock := s.managementClaimLocks.lockFor(taskID)
	lock.Lock()
	defer lock.Unlock()
	claims, ok := s.tasks.(repository.TaskManagementClaimRepository)
	if !ok {
		return errTaskManagementClaimsUnavailable
	}
	claim, err := claims.GetTaskManagementClaim(ctx, taskID)
	if err != nil {
		return err
	}
	switch {
	case claim == nil:
		if instanceKey != "" || generation != 0 {
			return repoerrors.ErrTaskManagementClaimConflict
		}
	case claim.OwnerKind == "":
		if instanceKey != "" || generation != claim.Generation {
			return repoerrors.ErrTaskManagementClaimConflict
		}
	case claim.OwnerKind != "plugin" || claim.InstallationID != installationID ||
		claim.InstanceKey != instanceKey || claim.Generation != generation:
		return repoerrors.ErrTaskManagementClaimConflict
	}
	return effect()
}
