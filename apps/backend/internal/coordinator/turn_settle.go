package coordinator

import (
	"context"

	"go.uber.org/zap"
)

// settleNotSent settles an open turn whose send never bound a session turn.
// A reserved orphan turn is completed first; any failure leaves the row for
// the next tick. Only a settle that changed the row counts and publishes.
func (s *Service) settleNotSent(ctx context.Context, turnID, outcome string) {
	turn, err := s.store.getUnattendedTurn(ctx, turnID)
	if err != nil || turn == nil || turn.Outcome != "" || turn.SessionTurnID != "" {
		return
	}
	if turn.ReservedTurnID != "" {
		if s.wakeFinder == nil {
			return
		}
		if err := s.wakeFinder.CompleteOrphanTurn(ctx, turn.SessionID, turn.ReservedTurnID); err != nil {
			s.logger.Warn("coordinator delivery: orphan turn not completed",
				zap.String("turn_id", turnID), zap.Error(err))
			return
		}
	}
	changed, err := s.store.settleUnsentTurn(ctx, turnID, outcome)
	if err != nil {
		s.logger.Warn("coordinator delivery: unsent settle failed", zap.String("turn_id", turnID), zap.Error(err))
		return
	}
	if !changed {
		return
	}
	unattendedTurnTotal.Add(outcome, 1)
	s.publishTurnSettled(ctx, turn.CoordinatorID)
	s.markOrphanMessage(ctx, turn)
}

func (s *Service) publishTurnSettled(ctx context.Context, coordinatorID string) {
	coord, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		s.logger.Warn("coordinator delivery: publish after settle skipped", zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return
	}
	s.publishCoordinatorUpdatedWith(ctx, coord.WorkspaceID, coordinatorID, true)
}

func (s *Service) markOrphanMessage(ctx context.Context, turn *unattendedTurn) {
	if s.wakeFinder == nil || turn.MessageID == "" {
		return
	}
	if err := s.wakeFinder.MarkOrphan(ctx, turn.SessionID, turn.MessageID); err != nil {
		s.logger.Warn("coordinator delivery: orphan message not marked", zap.String("turn_id", turn.ID), zap.Error(err))
	}
}
