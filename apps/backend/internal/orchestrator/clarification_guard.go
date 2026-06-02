package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// sessionHasPendingClarification reports whether the session still has durable
// clarification_request rows awaiting user input. Used to fail closed on
// workflow on_turn_complete while the user can still answer.
func (s *Service) sessionHasPendingClarification(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	msgs, err := s.repo.FindPendingClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to check pending clarifications; blocking turn-complete transition",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return true
	}
	return len(msgs) > 0
}
