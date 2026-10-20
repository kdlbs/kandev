package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

// PrepareStartup completes local housekeeping without activating producers.
// Failed or cancelled preparation remains retryable in the same service lifecycle.
func (s *Service) PrepareStartup(ctx context.Context) error {
	s.startupPreparationMu.Lock()
	defer s.startupPreparationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.startupPrepared {
		return nil
	}
	if err := s.reconcileStartupState(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.startupPrepared = true
	return nil
}

func (s *Service) housekeepingStep(ctx context.Context, id startup.StepID, run func() error) error {
	startup.BeginStep(ctx, id)
	started := time.Now()
	err := run()
	s.logger.Info("startup housekeeping operation finished", zap.String("step", string(id)),
		zap.Duration("duration", time.Since(started)), zap.Bool("success", err == nil))
	if err == nil {
		startup.EndStep(ctx, id)
	}
	return err
}

func startupRuntimeNeedsReconciliation(row *models.ExecutorRunning, state models.TaskSessionState) bool {
	return row != nil && (row.Status != models.ExecutorRunningStatusStopped || row.LocalPID != 0 || state == models.TaskSessionStateCreated || state == models.TaskSessionStateStarting || state == models.TaskSessionStateRunning)
}

func (s *Service) startupRuntimeInventory(ctx context.Context) ([]*models.ExecutorRunning, error) {
	rows, err := s.repo.ListExecutorsRunning(ctx)
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	sessions, err := s.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return nil, err
	}
	states := make(map[string]models.TaskSessionState, len(sessions))
	for _, session := range sessions {
		if session != nil {
			states[session.ID] = session.State
		}
	}
	selected := rows[:0]
	for _, row := range rows {
		if row != nil && startupRuntimeNeedsReconciliation(row, states[row.SessionID]) {
			selected = append(selected, row)
		}
	}
	return selected, nil
}

func (s *Service) classifyStartupRuntimeInventory(ctx context.Context) error {
	rows, err := s.startupRuntimeInventory(ctx)
	if err != nil {
		return err
	}
	startup.SetTotal(ctx, startup.StepRuntimeInventory, int64(len(rows)))
	for range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		startup.Advance(ctx, startup.StepRuntimeInventory, 1)
	}
	return nil
}
