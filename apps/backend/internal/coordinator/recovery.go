package coordinator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

// StartupRecoveryPass recovers every "approving" proposal claimed before t0
// (docs/specs/coordinator/system-design/proposals.md#recovery). Its
// signature matches the registration hook backendapp/coordinator.go's
// startCoordinatorBackgroundPass runs, so task-07's decisions registration
// function can return this method directly. t0 is the time recorded before
// the coordinator routes register: any row still "approving" with
// claimed_at before it was claimed by a process that has since stopped.
//
// Discovery runs once; a failure there is logged at warn and ends the pass
// (nothing retries it until the next startup). Rows are then recovered one
// at a time, in claimed_at/id order, sharing the same stale re-claim path an
// approve request's own recovery uses (cutoff = t0 instead of the
// two-minute rule). The pass stops early if ctx is cancelled (shutdown).
func (s *Service) StartupRecoveryPass(ctx context.Context, t0 time.Time) {
	rows, err := s.store.ListApprovingClaimedBeforeAnyKind(ctx, t0)
	if err != nil {
		s.logger.Warn("startup recovery: discovery query failed", zap.Error(err))
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		s.recoverStaleRow(ctx, row, t0)
	}
}

// recoverStaleRow re-claims one row. A re-claim that matches no row (an
// approve request won it first, or the row is gone) is not an error: it is
// skipped with no action, per proposals.md#recovery. Any other error is
// logged at warn with the proposal id, and the pass continues with the next
// row; the row is left exactly as the stale re-claim's own error rule
// leaves it.
func (s *Service) recoverStaleRow(ctx context.Context, row *Proposal, cutoff time.Time) {
	_, err := s.reclaimStaleAndProceed(ctx, row.WorkspaceID, row.CoordinatorID, row.ID, cutoff)
	if err == nil {
		s.logger.Info("startup recovery: row recovered",
			zap.String("proposal_id", row.ID), zap.String("coordinator_id", row.CoordinatorID), zap.String("workspace_id", row.WorkspaceID))
		return
	}
	var conflict *ProposalConflictError
	if errors.As(err, &conflict) || errors.Is(err, ErrNotFound) {
		return
	}
	s.logger.Warn("startup recovery: row failed",
		zap.String("proposal_id", row.ID), zap.Error(err))
}
