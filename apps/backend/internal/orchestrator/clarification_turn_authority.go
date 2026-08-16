package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

func (s *Service) clarificationTurnStillCurrent(ctx context.Context, data clarificationAnsweredData) bool {
	if data.ClarificationTurnID == "" || s.turnService == nil {
		s.logger.Warn("skipping clarification fallback without durable turn authority",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID))
		return false
	}
	turn, err := s.turnService.GetActiveTurn(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to verify clarification fallback turn authority",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID),
			zap.Error(err))
		return false
	}
	if turn == nil || turn.ID != data.ClarificationTurnID {
		currentTurnID := ""
		if turn != nil {
			currentTurnID = turn.ID
		}
		s.logger.Info("skipping superseded clarification fallback",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID),
			zap.String("current_turn_id", currentTurnID))
		return false
	}
	return true
}
