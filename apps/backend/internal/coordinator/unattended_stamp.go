package coordinator

import (
	"context"

	"go.uber.org/zap"
)

// currentUnattendedTurn returns the id of the coordinator's open unattended
// turn when that turn is the one its conversation session is running now, and
// nil otherwise. It never matches on the session alone, so a manager's turn is
// not mistaken for the unattended one, and a failed read stamps nothing.
func (s *Service) currentUnattendedTurn(ctx context.Context, coordinatorID string) *string {
	if !s.phase3 || s.activeTurns == nil {
		return nil
	}
	turn, err := s.store.openBoundTurn(ctx, coordinatorID)
	if err != nil {
		s.logger.Warn("coordinator unattended stamp: turn read failed", zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return nil
	}
	if turn == nil {
		return nil
	}
	active, err := s.activeTurns.GetActiveTurn(ctx, turn.SessionID)
	if err != nil {
		s.logger.Warn("coordinator unattended stamp: active turn read failed", zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return nil
	}
	if active == nil || active.ID != turn.SessionTurnID {
		return nil
	}
	id := turn.ID
	return &id
}
