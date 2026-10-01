package coordinator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/pause"
	"github.com/kandev/kandev/internal/orchestrator"
)

var (
	pauseLateSendTotal       = spendExpvarInt("coordinator_pause_late_send_total")
	pauseLateSendFailedTotal = spendExpvarInt("coordinator_pause_late_send_failed_total")
)

var _ pause.Stopper = (*Service)(nil)

// SetDreamStop registers the dream canceller. Stop calls it after the turn
// rows; a nil value clears it.
func (s *Service) SetDreamStop(stop pause.DreamStop) {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	s.dreamStop = stop
}

// stillPaused re-reads the gate. A read error answers paused, as everywhere.
func (s *Service) stillPaused(ctx context.Context, coordinatorID string) bool {
	paused, _ := s.gate.Active(ctx, coordinatorID)
	return paused
}

// Stop stops what a paused coordinator is running: each open unattended turn
// by its binding state, then its running dream. Every row is handled on its own
// and the joined errors are for the log only.
func (s *Service) Stop(ctx context.Context, coordinatorID string) error {
	if !s.stillPaused(ctx, coordinatorID) {
		return nil
	}
	turns, err := s.store.openUnattendedTurns(ctx, coordinatorID)
	errs := []error{err}
	for _, turn := range turns {
		errs = append(errs, s.stopTurn(ctx, coordinatorID, turn))
	}
	errs = append(errs, s.stopDream(ctx, coordinatorID))
	return errors.Join(errs...)
}

func (s *Service) stopDream(ctx context.Context, coordinatorID string) error {
	s.pauseMu.Lock()
	stop := s.dreamStop
	s.pauseMu.Unlock()
	if stop == nil || !s.stillPaused(ctx, coordinatorID) {
		return nil
	}
	return stop(ctx, coordinatorID)
}

func (s *Service) stopTurn(ctx context.Context, coordinatorID string, turn *unattendedTurn) error {
	if !s.stillPaused(ctx, coordinatorID) {
		return nil
	}
	if turn.SessionTurnID != "" {
		return s.stopRunningTurn(ctx, coordinatorID, turn.ID)
	}
	if _, err := s.store.markPauseRequested(ctx, turn.ID); err != nil {
		return err
	}
	if turn.ReservedTurnID != "" || s.store.now().UTC().Sub(turn.StartedAt) < sendSettleAge {
		return nil
	}
	changed, err := s.store.settlePausedUnsentTurn(ctx, turn.ID, coordinatorID)
	if err != nil || !changed {
		return err
	}
	unattendedTurnTotal.Add(outcomeStoppedByPause, 1)
	s.publishTurnSettled(ctx, coordinatorID)
	return nil
}

// stopRunningTurn is the per-row routine for a bound turn: mark, write the
// cancel intent, cancel through the ceiling's CancelTurn path, then settle
// through the one bound-turn settle.
func (s *Service) stopRunningTurn(ctx context.Context, coordinatorID, rowID string) error {
	if _, err := s.store.markPauseRequested(ctx, rowID); err != nil {
		return err
	}
	marked, err := s.store.markPauseCancel(ctx, rowID)
	if err != nil || !marked {
		return err
	}
	turn, err := s.store.getUnattendedTurn(ctx, rowID)
	if err != nil || turn == nil || turn.Outcome != "" || turn.SessionTurnID == "" {
		return err
	}
	if s.turnCanceller == nil {
		return errors.New("coordinator pause: turn canceller is not wired")
	}
	err = s.turnCanceller.CancelTurn(ctx, turn.SessionID, turn.SessionTurnID)
	switch {
	case errors.Is(err, orchestrator.ErrTurnNotActive):
		return nil
	case errors.Is(err, orchestrator.ErrCancelInFlight):
		s.logger.Info("coordinator pause: cancel already in flight",
			zap.String("coordinator_id", coordinatorID), zap.String("turn_id", rowID))
		return nil
	case err != nil:
		return fmt.Errorf("coordinator pause: cancel turn %s: %w", rowID, err)
	}
	fresh, err := s.store.getUnattendedTurn(ctx, rowID)
	if err != nil || fresh == nil || fresh.Outcome != "" {
		return err
	}
	s.settleBoundTurn(ctx, fresh)
	return nil
}

// stopPausedCoordinators is the backstop's pause step: it refreshes the
// known-paused set and calls Stop for every paused coordinator in id order,
// before any turn duty runs. A failed query skips the step for the pass.
func (s *Service) stopPausedCoordinators(ctx context.Context, skip func(what string, err error)) {
	ids, err := s.store.PausedCoordinatorIDs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			skip("paused coordinators", err)
		}
		return
	}
	s.knownPaused.Replace(ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		s.stopPaused(ctx, id)
	}
}

// onAccepted binds the accepted session turn to its row and, when a pause
// found the row or the coordinator is paused, cancels it from a service-owned
// goroutine: the callback runs inside the orchestrator's dispatch admission, so
// it never waits on CancelTurn.
func (s *Service) onAccepted(ctx context.Context, coordinatorID, sessionID, turnID, sessionTurnID string) {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bindTimeout)
	defer cancel()
	changed, err := s.store.bindAcceptedTurn(wctx, turnID, sessionTurnID)
	if err != nil || !changed {
		s.logger.Warn("coordinator delivery: turn binding not written",
			zap.String("turn_id", turnID), zap.String("binding", "accepted"), zap.Bool("changed", changed), zap.Error(err))
		s.cancelLateSend(wctx, coordinatorID, sessionID, sessionTurnID)
		return
	}
	row, err := s.store.getUnattendedTurn(wctx, turnID)
	if err != nil || row == nil || row.PauseRequestedAt == nil || !s.stillPaused(wctx, coordinatorID) {
		return
	}
	s.runPaused(func(runCtx context.Context) {
		if err := s.stopRunningTurn(runCtx, coordinatorID, turnID); err != nil {
			s.logger.Warn("coordinator pause: accepted turn not stopped, left to the backstop",
				zap.String("coordinator_id", coordinatorID), zap.String("turn_id", turnID), zap.Error(err))
		}
	})
}

// cancelLateSend cancels an accepted session turn no open row can own, when the
// coordinator is paused. It is not retried: no open row remains for the backstop.
func (s *Service) cancelLateSend(ctx context.Context, coordinatorID, sessionID, sessionTurnID string) {
	if !s.stillPaused(ctx, coordinatorID) {
		return
	}
	s.runPaused(func(runCtx context.Context) {
		pauseLateSendTotal.Add(1)
		if s.turnCanceller == nil {
			pauseLateSendFailedTotal.Add(1)
			return
		}
		err := s.turnCanceller.CancelTurn(runCtx, sessionID, sessionTurnID)
		if err == nil || errors.Is(err, orchestrator.ErrTurnNotActive) {
			return
		}
		pauseLateSendFailedTotal.Add(1)
		s.logger.Warn("coordinator pause: late send not cancelled",
			zap.String("coordinator_id", coordinatorID), zap.String("session_id", sessionID), zap.Error(err))
	})
}
