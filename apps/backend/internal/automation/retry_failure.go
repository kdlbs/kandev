package automation

import (
	"context"
)

// FinalizeAutomationRetryFailure is the single service entry point for a
// retry-capable failure. It keeps generation and run identity server-owned
// and serializes finalization with retry side effects and cancellation.
func (s *Service) FinalizeAutomationRetryFailure(ctx context.Context, runID string, generation int64, raw error, phase string) (*AutomationRun, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, ErrAutomationRunNotDispatchable
	}
	if !retryRunLockHeld(ctx, run.AutomationID) {
		unlock := s.automationRunLock(run.AutomationID)
		defer unlock()
	}
	return s.store.FinalizeRetryFailure(ctx, runID, generation, raw, phase)
}
