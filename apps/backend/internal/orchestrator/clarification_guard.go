package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const clarificationAgentDisconnectedMetadataKey = "agent_disconnected"

// sessionHasPendingClarification reports whether the session still has durable
// clarification_request rows awaiting user input. Used to fail closed on
// workflow on_turn_complete while the user can still answer.
func (s *Service) sessionHasPendingClarification(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	msgs, err := s.repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to check pending clarifications; blocking turn-complete transition",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return true
	}
	return len(msgs) > 0
}

// sessionHasLiveClarification reports whether a current-turn clarification
// still owns the agent turn. A detached bundle remains pending and answerable,
// but it no longer owns a live turn, so a parked session may dispatch a newer
// queued prompt. Repository failures fail closed because the caller cannot
// safely distinguish a detached bundle from a live clarification.
func (s *Service) sessionHasLiveClarification(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	msgs, err := s.repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to check live clarifications; blocking queue drain",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return true
	}
	for _, msg := range msgs {
		if msg == nil || msg.Metadata == nil {
			return true
		}
		detached, ok := msg.Metadata[clarificationAgentDisconnectedMetadataKey].(bool)
		if !ok || !detached {
			return true
		}
	}
	return false
}

// turnCompleteBlockedByUserInput reports and applies the workflow barrier for
// durable user-input waits. A pending clarification is a platform pause, not a
// step-completion signal, so all turn-complete transition entrypoints fail
// closed while it exists.
func (s *Service) turnCompleteBlockedByUserInput(ctx context.Context, taskID, sessionID string, session *models.TaskSession) bool {
	if !s.sessionHasPendingClarification(ctx, sessionID) {
		return false
	}
	s.logger.Info("deferring on_turn_complete while clarification is pending",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	if session != nil {
		if _, has := models.LoadPendingStepSignal(session.Metadata); has {
			s.clearPendingStepSignal(ctx, session)
		}
	}
	s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
	return true
}
