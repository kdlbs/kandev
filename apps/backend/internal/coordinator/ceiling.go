package coordinator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	ceilingReasonCeiling      = "ceiling"
	ceilingReasonUnmeasurable = "unmeasurable"
	outcomeStoppedAtCeiling   = "stopped_at_ceiling"
)

var (
	ceilingStopTotal         = spendExpvarMap("coordinator_ceiling_stop_total")
	ceilingCancelFailedTotal = spendExpvarInt("coordinator_ceiling_cancel_failed_total")
	unattendedTurnTotal      = spendExpvarMap("coordinator_unattended_turn_total")
)

// ActiveTurnReader reads a session's active turn, (nil, nil) when it has none.
// Satisfied by the task service.
type ActiveTurnReader interface {
	GetActiveTurn(ctx context.Context, sessionID string) (*taskmodels.Turn, error)
}

// TurnCanceller cancels one identified turn of a session without side effects
// beyond the cancel itself. Satisfied by the orchestrator service.
type TurnCanceller interface {
	CancelTurn(ctx context.Context, sessionID, expectedTurnID string) error
}

// ObserveUsage is the usage writer's post-insert notice. It returns at once
// when phase 3 is not effective or an id is empty; any failure is logged and
// left to the next notice or backstop tick.
func (s *Service) ObserveUsage(ctx context.Context, taskID, sessionID string) {
	if !s.phase3 || taskID == "" || sessionID == "" {
		return
	}
	if err := s.CheckCeilingForSession(ctx, taskID, sessionID); err != nil {
		s.logger.Warn("coordinator usage observer: ceiling check failed",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}

// CheckCeilingForSession resolves the coordinator from taskID's metadata and
// runs the ceiling check when its open turn is on sessionID. A task that names
// no coordinator is nothing to do.
func (s *Service) CheckCeilingForSession(ctx context.Context, taskID, sessionID string) error {
	if s.conversationTasks == nil {
		return errors.New("coordinator ceiling: task reader is not wired")
	}
	task, err := s.conversationTasks.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("coordinator ceiling: read task: %w", err)
	}
	coordinatorID := conversationTaskCoordinatorID(task)
	if coordinatorID == "" {
		return nil
	}
	return s.checkCeiling(ctx, coordinatorID, sessionID)
}

// CheckCeiling is the backstop entry: it stops the coordinator's open
// unattended turn when spend is unmeasurable or at the ceiling, and retries a
// stop already requested.
func (s *Service) CheckCeiling(ctx context.Context, coordinatorID string) error {
	if coordinatorID == "" {
		return nil
	}
	return s.checkCeiling(ctx, coordinatorID, "")
}

func (s *Service) checkCeiling(ctx context.Context, coordinatorID, onlySessionID string) error {
	release, ok := s.ceilingLocks.tryAcquire(coordinatorID)
	if !ok {
		return nil
	}
	defer release()
	turn, err := s.store.openCeilingTurn(ctx, coordinatorID)
	if err != nil || turn == nil {
		return err
	}
	if onlySessionID != "" && turn.SessionID != onlySessionID {
		return nil
	}
	if turn.SessionTurnID == nil || *turn.SessionTurnID == "" {
		return nil
	}
	coord, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if turn.StopRequestedAt == nil {
		proceed, err := s.requestCeilingStop(ctx, coord, turn)
		if err != nil || !proceed {
			return err
		}
	}
	return s.cancelAndSettle(ctx, coord, turn)
}

// requestCeilingStop runs steps 4 to 6: the active-turn filter, the decision
// and the mark. It reports whether the caller should go on to cancel.
func (s *Service) requestCeilingStop(ctx context.Context, coord *Coordinator, turn *ceilingTurn) (bool, error) {
	if s.activeTurns == nil {
		return false, errors.New("coordinator ceiling: active turn reader is not wired")
	}
	active, err := s.activeTurns.GetActiveTurn(ctx, turn.SessionID)
	if err != nil {
		return false, fmt.Errorf("coordinator ceiling: read active turn: %w", err)
	}
	if active == nil || active.ID != *turn.SessionTurnID {
		return false, nil
	}
	now := s.store.now()
	ceiling := turn.StartCeilingSubcents
	if coord.CostCeilingSubcents != nil {
		ceiling = *coord.CostCeilingSubcents
	}
	reading, spendErr := s.Spend(ctx, coord, now)
	reason := ""
	switch {
	case spendErr != nil || !reading.Measurable:
		reason = ceilingReasonUnmeasurable
	case reading.WindowSubcents >= ceiling:
		reason = ceilingReasonCeiling
	default:
		return false, nil
	}
	marked, err := s.store.markTurnStopRequested(ctx, turn.ID, now)
	if err != nil {
		return false, err
	}
	if marked {
		ceilingStopTotal.Add(reason, 1)
		s.logCeilingStop(coord.ID, reason, reading, ceiling)
		return true, nil
	}
	current, err := s.store.ceilingTurnByID(ctx, turn.ID)
	if err != nil {
		return false, err
	}
	return current != nil && current.Outcome == nil && current.StopRequestedAt != nil, nil
}

func (s *Service) logCeilingStop(coordinatorID, reason string, reading SpendReading, ceiling int64) {
	if reason == ceilingReasonUnmeasurable {
		s.logger.Info("coordinator ceiling stop requested",
			zap.String("coordinator_id", coordinatorID), zap.String("reason", reason))
		return
	}
	s.logger.Info("coordinator ceiling stop requested",
		zap.String("coordinator_id", coordinatorID), zap.String("reason", reason),
		zap.Int64("window_subcents", reading.WindowSubcents), zap.Int64("ceiling_subcents", ceiling))
}

// cancelAndSettle runs steps 7 and 8 for a marked turn.
func (s *Service) cancelAndSettle(ctx context.Context, coord *Coordinator, turn *ceilingTurn) error {
	if s.turnCanceller == nil {
		ceilingCancelFailedTotal.Add(1)
		return errors.New("coordinator ceiling: turn canceller is not wired")
	}
	err := s.turnCanceller.CancelTurn(ctx, turn.SessionID, *turn.SessionTurnID)
	if errors.Is(err, orchestrator.ErrTurnNotActive) {
		return nil
	}
	if err != nil {
		ceilingCancelFailedTotal.Add(1)
		s.logger.Warn("coordinator ceiling: cancel failed",
			zap.String("coordinator_id", coord.ID), zap.String("session_id", turn.SessionID), zap.Error(err))
		return fmt.Errorf("coordinator ceiling: cancel turn: %w", err)
	}
	finished := s.store.now()
	var cost *int64
	if sub, known, costErr := s.TurnCost(ctx, TurnKey{SessionID: turn.SessionID, SessionTurnID: *turn.SessionTurnID, FinishedAt: finished}); costErr != nil {
		s.logger.Warn("coordinator ceiling: turn cost unavailable at settle", zap.String("turn_id", turn.ID), zap.Error(costErr))
	} else if known {
		cost = &sub
	}
	changed, err := s.store.settleTurnStoppedAtCeiling(ctx, turn.ID, finished, cost)
	if err != nil || !changed {
		return err
	}
	unattendedTurnTotal.Add(outcomeStoppedAtCeiling, 1)
	s.afterAutonomyChange(ctx, coord.WorkspaceID, coord.ID)
	return nil
}

// RecomputeTurnCost re-sums a settled turn's ledger cost and stores it when it
// fills a NULL or raises a lower value. Rows are only ever added to a turn, so
// the value only grows; the backstop repeats it for turns settled in the last
// 11 minutes.
func (s *Service) RecomputeTurnCost(ctx context.Context, turnRowID string, key TurnKey) error {
	cost, known, err := s.TurnCost(ctx, key)
	if err != nil || !known {
		return err
	}
	_, err = s.store.raiseTurnCost(ctx, turnRowID, cost)
	return err
}
