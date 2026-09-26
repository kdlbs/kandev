package share

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TaskAccessAuthorizer is the narrow task-service authorization surface used
// by share operations. The task service owns identity and workspace policy;
// the share service only decides when that policy must run.
type TaskAccessAuthorizer interface {
	AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
}

// ErrAuthorization marks an authorization failure that is not an object
// access denial. HTTP callers must not receive the underlying infrastructure
// error or mistake it for a missing backend credential.
var ErrAuthorization = errors.New("share authorization failed")

func (s *Service) authorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error {
	if err := s.authorizer.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return normalizeAuthorizationError(err)
	}
	return nil
}

func (s *Service) authorizeSessionAccess(ctx context.Context, sessionID string) error {
	if err := s.authorizer.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return normalizeAuthorizationError(err)
	}
	return nil
}

func normalizeAuthorizationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) ||
		errors.Is(err, repoerrors.ErrTaskNotFound) ||
		errors.Is(err, repoerrors.ErrWorkspaceNotFound) ||
		errors.Is(err, models.ErrTaskSessionNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%w: %w", ErrAuthorization, err)
}
