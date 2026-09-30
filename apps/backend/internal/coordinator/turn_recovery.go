package coordinator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	// sendSettleAge is how long an unbound turn stays open before the backstop
	// may settle it send_failed.
	sendSettleAge = 2 * time.Minute
	// costRecomputeWindow is how long after settling the backstop re-sums a
	// turn's ledger cost.
	costRecomputeWindow = 11 * time.Minute
)

// turnDuties is the backstop's per-coordinator turn duty: send recovery and
// missed-settle re-derivation for the open turn, the cost recompute for
// recently settled turns, then the ceiling check. Each duty runs whatever an
// earlier one returned; the errors are joined.
func (s *Service) turnDuties(ctx context.Context, coordinatorID string) error {
	var errs []error
	open, err := s.store.openUnattendedTurns(ctx, coordinatorID)
	errs = append(errs, err)
	for _, turn := range open {
		s.recoverOpenTurn(ctx, turn)
	}
	recent, err := s.store.recentSettledTurns(ctx, coordinatorID, s.store.now().UTC().Add(-costRecomputeWindow))
	errs = append(errs, err)
	for _, turn := range recent {
		key := TurnKey{SessionID: turn.SessionID, SessionTurnID: turn.SessionTurnID, FinishedAt: *turn.FinishedAt}
		if err := s.RecomputeTurnCost(ctx, turn.ID, key); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, s.CheckCeiling(ctx, coordinatorID))
	return errors.Join(errs...)
}

// recoverOpenTurn runs send recovery for an unbound row and message lookup
// plus missed-settle re-derivation for a bound one.
func (s *Service) recoverOpenTurn(ctx context.Context, turn *unattendedTurn) {
	if turn.SessionTurnID == "" {
		s.recoverUnboundTurn(ctx, turn)
		return
	}
	if turn.MessageID == "" {
		s.recordMissingMessage(ctx, turn)
	}
	if s.settleMissed(ctx, turn) {
		s.settleBoundTurn(ctx, turn)
	}
}

func (s *Service) recoverUnboundTurn(ctx context.Context, turn *unattendedTurn) {
	if s.store.now().UTC().Sub(turn.StartedAt) < sendSettleAge {
		return
	}
	busy, err := s.sessionBusy(ctx, turn.SessionID)
	if err != nil || busy {
		return
	}
	s.settleNotSent(ctx, turn.ID, outcomeSendFailed)
}

// sessionBusy reports whether the session is RUNNING or STARTING; a session
// that no longer exists is neither.
func (s *Service) sessionBusy(ctx context.Context, sessionID string) (bool, error) {
	if s.convReader == nil {
		return false, errors.New("coordinator recovery: conversation reader is not wired")
	}
	state, exists, err := s.convReader.SessionState(ctx, sessionID)
	if err != nil {
		s.logger.Warn("coordinator recovery: session state unreadable", zap.String("session_id", sessionID), zap.Error(err))
		return false, err
	}
	if !exists {
		return false, nil
	}
	st := taskmodels.TaskSessionState(state)
	return st == taskmodels.TaskSessionStateRunning || st == taskmodels.TaskSessionStateStarting, nil
}

func (s *Service) recordMissingMessage(ctx context.Context, turn *unattendedTurn) {
	if s.wakeFinder == nil {
		return
	}
	msg, err := s.wakeFinder.FindWakeMessage(ctx, turn.SessionID, turn.ID, turn.StartedAt)
	if err != nil || msg == nil {
		if err != nil {
			s.logger.Warn("coordinator recovery: wake message lookup failed", zap.String("turn_id", turn.ID), zap.Error(err))
		}
		return
	}
	if _, err := s.store.recordFoundMessage(ctx, turn.ID, msg.ID); err != nil {
		s.logger.Warn("coordinator recovery: message id not recorded", zap.String("turn_id", turn.ID), zap.Error(err))
	}
}

// settleMissed reports whether a bound open turn has ended without its
// turn.completed being handled: its session turn is completed or gone, or the
// session's active turn is another turn or none. A failed read is not a
// trigger.
func (s *Service) settleMissed(ctx context.Context, turn *unattendedTurn) bool {
	if s.convReader == nil || s.activeTurns == nil {
		return false
	}
	info, err := s.convReader.Turn(ctx, turn.SessionTurnID)
	if err != nil {
		s.logger.Warn("coordinator recovery: turn unreadable", zap.String("turn_id", turn.ID), zap.Error(err))
		return false
	}
	if info == nil || info.Completed {
		return true
	}
	active, err := s.activeTurns.GetActiveTurn(ctx, turn.SessionID)
	if err != nil {
		s.logger.Warn("coordinator recovery: active turn unreadable", zap.String("turn_id", turn.ID), zap.Error(err))
		return false
	}
	return active == nil || active.ID != turn.SessionTurnID
}

// RecoverUnattendedStartup settles the open turns that started before this
// call, then prunes retained wake state. A bound row is left to turn end and
// the backstop. An unbound row whose reserved turn is set and whose session is
// persisted RUNNING or STARTING is left to the backstop; any other unbound row
// settles interrupted. Every settle is conditional, so a concurrent delivery,
// turn end or backstop tick makes it a no-op.
func (s *Service) RecoverUnattendedStartup(ctx context.Context) error {
	t0 := s.store.now().UTC()
	open, err := s.store.openUnattendedTurns(ctx, "")
	if err != nil {
		return err
	}
	for _, turn := range open {
		if !turn.StartedAt.Before(t0) || turn.SessionTurnID != "" {
			continue
		}
		if turn.ReservedTurnID != "" {
			busy, err := s.sessionBusy(ctx, turn.SessionID)
			if err != nil || busy {
				continue
			}
		}
		s.settleNotSent(ctx, turn.ID, outcomeInterrupted)
	}
	if _, _, err := s.PruneWakeState(ctx, t0); err != nil {
		return err
	}
	return nil
}
