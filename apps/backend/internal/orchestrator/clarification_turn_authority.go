package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

func (s *Service) clarificationTurnStillCurrent(ctx context.Context, data clarificationAnsweredData) bool {
	if s.turnService == nil {
		s.logger.Warn("skipping clarification fallback: turn service unavailable",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID))
		return false
	}
	if data.ClarificationTurnID == "" {
		s.logger.Debug("skipping clarification fallback: event carries no turn ID",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID))
		return false
	}
	turn, err := s.turnService.GetActiveTurn(ctx, data.SessionID)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			s.logger.Debug("clarification fallback authority check canceled",
				zap.String("session_id", data.SessionID),
				zap.String("pending_id", data.PendingID),
				zap.String("clarification_turn_id", data.ClarificationTurnID),
				zap.Error(err))
			return false
		}
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

// clarificationTurnStillCurrentAfterRecovery repeats the turn-authority check
// after silent cancellation has settled. Owned silent cancellation normally
// completes the captured clarification turn, so an absent active turn is valid
// only when that exact turn is durably completed. A different active turn is a
// successor and must prevent the stale watchdog from queueing over it.
func (s *Service) clarificationTurnStillCurrentAfterRecovery(
	ctx context.Context,
	data clarificationAnsweredData,
) bool {
	if s.turnService == nil {
		s.logger.Warn("skipping clarification fallback after recovery: turn service unavailable",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID))
		return false
	}
	if data.ClarificationTurnID == "" {
		s.logger.Debug("skipping clarification fallback after recovery: event carries no turn ID",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID))
		return false
	}

	activeTurn, err := s.turnService.GetActiveTurn(ctx, data.SessionID)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			s.logger.Debug("clarification fallback post-recovery authority check canceled",
				zap.String("session_id", data.SessionID),
				zap.String("pending_id", data.PendingID),
				zap.String("clarification_turn_id", data.ClarificationTurnID),
				zap.Error(err))
			return false
		}
		s.logger.Warn("failed to verify clarification fallback turn authority after recovery",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID),
			zap.Error(err))
		return false
	}
	if activeTurn != nil {
		if activeTurn.ID == data.ClarificationTurnID {
			return true
		}
		s.logger.Info("skipping superseded clarification fallback after recovery",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID),
			zap.String("current_turn_id", activeTurn.ID))
		return false
	}

	completedTurn, err := s.turnService.GetTurn(ctx, data.ClarificationTurnID)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			s.logger.Debug("clarification fallback completed-turn lookup canceled",
				zap.String("session_id", data.SessionID),
				zap.String("pending_id", data.PendingID),
				zap.String("clarification_turn_id", data.ClarificationTurnID),
				zap.Error(err))
			return false
		}
		s.logger.Warn("failed to verify completed clarification turn after recovery",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID),
			zap.Error(err))
		return false
	}
	if completedTurn == nil || completedTurn.CompletedAt == nil {
		s.logger.Info("skipping clarification fallback after recovery: captured turn is not settled",
			zap.String("session_id", data.SessionID),
			zap.String("pending_id", data.PendingID),
			zap.String("clarification_turn_id", data.ClarificationTurnID))
		return false
	}
	return true
}
