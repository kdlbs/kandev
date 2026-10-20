package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
)

func (o DeleteTaskOptions) validate() error {
	if o.AllowUnverifiedWorkspaceChanges && (o.retainsWorkspace() || !o.DiscardWorktreeChanges) {
		return fmt.Errorf("%w: unverified workspace changes require delete mode and discard consent", ErrTaskDeletePreflightInvalid)
	}
	switch o.WorkspaceCleanup.Normalized() {
	case models.WorkspaceCleanupDelete:
		return nil
	case models.WorkspaceCleanupRetain:
		if !o.DiscardWorktreeChanges {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid workspace cleanup options", ErrTaskDeletePreflightInvalid)
}

func (o DeleteTaskOptions) retainsWorkspace() bool {
	return o.WorkspaceCleanup == models.WorkspaceCleanupRetain
}

// DeleteTaskWithLifecycleOptions preserves the canonical ownership coordinator.
func (s *Service) DeleteTaskWithLifecycleOptions(ctx context.Context, id string, options DeleteTaskOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	if s.taskLifecycleCoordinator == nil {
		return s.DeleteTaskWithOptions(ctx, id, options)
	}
	if coordinator, ok := s.taskLifecycleCoordinator.(interface {
		DeleteTaskTreeWithOptions(context.Context, string, bool, DeleteTaskOptions) (*CascadeOutcome, error)
	}); ok {
		_, err := coordinator.DeleteTaskTreeWithOptions(ctx, id, false, options)
		var committed *CascadePostCommitError
		if errors.As(err, &committed) {
			return nil
		}
		return err
	}
	if options.retainsWorkspace() || options.AllowUnverifiedWorkspaceChanges {
		return fmt.Errorf("task lifecycle coordinator does not support workspace cleanup options")
	}
	return s.DeleteTaskWithLifecycle(ctx, id)
}

func (s *HandoffService) validateDeleteCascadeAdmission(ctx context.Context, ids []string, options DeleteTaskOptions) error {
	if options.retainsWorkspace() {
		return nil
	}
	if checker, ok := s.resourceCleaner.(interface {
		ValidateTaskDeleteWorktreesWithOptions(context.Context, []string, DeleteTaskOptions) error
	}); ok {
		return checker.ValidateTaskDeleteWorktreesWithOptions(ctx, ids, options)
	}
	if options.AllowUnverifiedWorkspaceChanges {
		return errors.New("task cleaner does not support unverified workspace changes")
	}
	if checker, ok := s.resourceCleaner.(taskDeleteWorktreeAdmissionChecker); ok {
		return checker.ValidateTaskDeleteWorktrees(ctx, ids, options.DiscardWorktreeChanges)
	}
	return nil
}
