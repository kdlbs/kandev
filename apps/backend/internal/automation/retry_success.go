package automation

import "context"

func (s *Service) MarkAutomationRetrySucceeded(ctx context.Context, runID string, generation int64) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return ErrAutomationRunNotDispatchable
	}
	if !retryRunLockHeld(ctx, run.AutomationID) {
		unlock := s.automationRunLock(run.AutomationID)
		defer unlock()
	}
	return s.store.MarkRetrySucceeded(ctx, runID, generation)
}
