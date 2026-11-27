package coordinator

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// UnattendedPermissionResolver rejects one pending permission request of an
// unattended turn. Implementations treat an already-resolved or vanished
// request as success.
type UnattendedPermissionResolver interface {
	ResolveUnattendedPermission(ctx context.Context, taskID, sessionID, pendingID, unattendedTurnID string) error
}

// SetUnattendedPermissionResolver injects the resolver once at wiring; nil
// makes HandleUnattendedPermission and ReresolveRecordedDenials no-ops.
func (s *Service) SetUnattendedPermissionResolver(r UnattendedPermissionResolver) {
	s.permissionResolver = r
}

// HandleUnattendedPermission denies a permission request that reached the
// backend while the conversation's unattended turn was open. It does nothing
// unless the request matches an open turn row of the session. A failed
// resolution is logged and leaves the recorded denial for
// ReresolveRecordedDenials.
func (s *Service) HandleUnattendedPermission(ctx context.Context, taskID, sessionID, pendingID, activeTurnID string) {
	if s.permissionResolver == nil {
		return
	}
	turnID, matched, err := s.store.RecordUnattendedDenial(ctx, taskID, sessionID, pendingID, activeTurnID)
	if err != nil {
		s.logger.Warn("record unattended permission denial failed",
			zap.String("task_id", taskID), zap.String("pending_id", pendingID), zap.Error(err))
		return
	}
	if !matched {
		return
	}
	if err := s.permissionResolver.ResolveUnattendedPermission(ctx, taskID, sessionID, pendingID, turnID); err != nil {
		s.logger.Warn("resolve unattended permission failed; denial kept for retry",
			zap.String("task_id", taskID), zap.String("pending_id", pendingID),
			zap.String("unattended_turn_id", turnID), zap.Error(err))
	}
}

// ReresolveRecordedDenials retries the resolution of every recorded denial of
// the coordinator's open unattended turns, without counting again. Every
// denial is tried; the failures are joined and returned.
func (s *Service) ReresolveRecordedDenials(ctx context.Context, coordinatorID string) error {
	if s.permissionResolver == nil {
		return nil
	}
	denials, err := s.store.ListOpenDenials(ctx, coordinatorID)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range denials {
		if err := s.permissionResolver.ResolveUnattendedPermission(ctx, d.TaskID, d.SessionID, d.PendingID, d.TurnID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
