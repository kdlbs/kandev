package coordinator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// onTurnCompleted settles the open turn row bound to the completed session
// turn. A turn no row is bound to (an attended turn, an orphan turn) does
// nothing.
func (s *Service) onTurnCompleted(ctx context.Context, event *bus.Event) {
	data, ok := eventData(event)
	if !ok {
		return
	}
	turnID := eventString(data, "id")
	if turnID == "" {
		return
	}
	turn, err := s.store.openTurnBySessionTurn(ctx, turnID)
	if err != nil {
		s.logger.Warn("coordinator turn end: read bound turn failed", zap.String("session_turn_id", turnID), zap.Error(err))
		return
	}
	if turn != nil {
		s.settleBoundTurn(ctx, turn)
	}
}

// boundTurnOutcome derives a bound turn's outcome from the ceiling stop marker
// and the session's state; ok is false when the state cannot be read.
func (s *Service) boundTurnOutcome(ctx context.Context, turn *unattendedTurn) (string, bool) {
	if turn.StopRequestedAt != nil {
		return outcomeStopped, true
	}
	if s.convReader == nil {
		return "", false
	}
	state, exists, err := s.convReader.SessionState(ctx, turn.SessionID)
	if err != nil {
		s.logger.Warn("coordinator turn end: session state unreadable", zap.String("turn_id", turn.ID), zap.Error(err))
		return "", false
	}
	switch {
	case !exists || taskmodels.TaskSessionState(state) == taskmodels.TaskSessionStateCancelled:
		return outcomeCancelled, true
	case taskmodels.TaskSessionState(state) == taskmodels.TaskSessionStateFailed:
		return outcomeFailed, true
	}
	return outcomeCompleted, true
}

// settleBoundTurn settles a bound open turn. Only a settle that changed the
// row computes the cost, counts, publishes and kicks.
func (s *Service) settleBoundTurn(ctx context.Context, turn *unattendedTurn) {
	outcome, ok := s.boundTurnOutcome(ctx, turn)
	if !ok {
		return
	}
	finished := s.store.now().UTC()
	changed, err := s.store.settleOpenTurn(ctx, turn.ID, outcome, finished)
	if err != nil {
		s.logger.Warn("coordinator turn end: settle failed", zap.String("turn_id", turn.ID), zap.Error(err))
		return
	}
	if !changed {
		return
	}
	key := TurnKey{SessionID: turn.SessionID, SessionTurnID: turn.SessionTurnID, FinishedAt: finished}
	if err := s.RecomputeTurnCost(ctx, turn.ID, key); err != nil {
		s.logger.Warn("coordinator turn end: cost unavailable at settle", zap.String("turn_id", turn.ID), zap.Error(err))
	}
	unattendedTurnTotal.Add(outcome, 1)
	s.publishTurnSettled(ctx, turn.CoordinatorID)
	s.callKick(ctx, turn.CoordinatorID)
}
