package automation

import (
	"context"
	"errors"
	"fmt"
)

func retryRunBindingIncomplete(run *AutomationRun) bool {
	return run == nil || run.TaskID == "" || run.SessionID == "" || run.TurnID == ""
}

func (s *Service) markStoppedAutomationRun(ctx context.Context, run, boundRun *AutomationRun, originallyUnbound bool) error {
	markErr := s.store.MarkRunTerminal(ctx, run.ID, boundRun.SessionID, boundRun.TurnID, RunStatusFailed, "automation deleted")
	if markErr != nil && originallyUnbound {
		markErr = s.store.MarkRunTerminal(ctx, run.ID, "", "", RunStatusFailed, "automation deleted")
	}
	if markErr != nil {
		return fmt.Errorf("mark automation run %s failed: %w", run.ID, markErr)
	}
	return nil
}

func retryOperationBindingConflicts(run *AutomationRun, operation *RetryOperation) bool {
	return (run.TaskID != "" && run.TaskID != operation.ExternalTaskID) ||
		(run.SessionID != "" && run.SessionID != operation.ExternalSessionID) ||
		(run.TurnID != "" && run.TurnID != operation.ExternalTurnID)
}

func (s *Service) requireUnboundRetryStopProvenance(ctx context.Context, run *AutomationRun) error {
	if run.RetryGroupID != "" {
		var modern int
		if err := s.store.ro.GetContext(ctx, &modern, s.store.ro.Rebind(`SELECT COUNT(*) FROM automation_retry_launch_protocols WHERE run_id = ? AND version = 1`), run.ID); err != nil {
			return err
		}
		if modern != 1 {
			return ErrRetryLaunchUncertain
		}
	}
	return nil
}

func (s *Service) stopRetryExecution(ctx context.Context, run *AutomationRun) (bool, error) {
	var stopped bool
	var err error
	if definitive, ok := s.runStopper.(interface {
		StopAutomationRunDefinitively(context.Context, string, string, string) (bool, error)
	}); ok {
		stopped, err = definitive.StopAutomationRunDefinitively(ctx, run.TaskID, run.SessionID, run.TurnID)
	} else {
		stopped, err = s.runStopper.StopAutomationRun(ctx, run.TaskID, run.SessionID, run.TurnID)
		if err == nil && run.RetryGroupID != "" {
			return false, ErrRetryLaunchUncertain
		}
	}
	return stopped, err
}

func (s *Service) finishRetryStop(ctx context.Context, run *AutomationRun, launch *RetryLaunch, stopped bool) error {
	if launch != nil {
		if !stopped {
			current, err := s.store.GetRetryLaunch(ctx, run.ID)
			if err == nil && current.State == RetryLaunchStopped {
				return nil
			}
			return errors.Join(ErrRetryLaunchUncertain, err)
		}
		return s.store.CompleteRetryLaunchStop(ctx, launch)
	}
	if !stopped {
		return ErrRetryLaunchUncertain
	}
	if run.RetryGroupID != "" {
		return s.store.recordLegacyRetryStop(ctx, run)
	}
	return nil
}
