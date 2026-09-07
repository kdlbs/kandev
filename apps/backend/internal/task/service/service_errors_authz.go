package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ErrForbidden reports that the caller can see the resource but lacks the
// scope the action requires.
//
// It is deliberately distinct from the *NotFound sentinels. Those hide
// existence from a caller who has no business knowing the resource is there;
// this one is returned only after workspace.read has already been granted, so
// there is nothing left to hide and a 404 would just be confusing ("the task I
// am looking at does not exist?").
var ErrForbidden = errors.New("insufficient permissions for this action")

// IsForbidden reports whether an error is the scope-denied sentinel.
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// requireWorkspaceManage gates the owner-only workspace actions: rename,
// defaults, visibility, and delete. It applies the 404-vs-403 rule so a caller
// who cannot see the workspace never learns it exists.
func (s *Service) requireWorkspaceManage(ctx context.Context, workspace *models.Workspace) error {
	if _, scoped := callerScope(ctx); !scoped {
		return nil
	}
	decision := s.workspaceDecision(ctx, workspace)
	if !decision.CanRead() {
		return repoerrors.ErrWorkspaceNotFound
	}
	if !decision.Has(authz.ScopeWorkspaceManage) {
		return ErrForbidden
	}
	return nil
}
