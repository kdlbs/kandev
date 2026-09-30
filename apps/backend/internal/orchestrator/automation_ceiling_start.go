package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// ceilingPayloadAutomationRunKey holds, in a ceiling "start" replay payload,
// the automation run that the queued start serves.
const ceilingPayloadAutomationRunKey = "automation_run"

// automationRunLaunch names the admitted automation run a task start serves,
// with the thread disposition its binding records.
type automationRunLaunch struct {
	RunID        string                  `json:"run_id"`
	ThreadAction automation.ThreadAction `json:"thread_action"`
	ThreadReason string                  `json:"thread_reason"`
}

// automationRunFromCeilingPayload returns the run a queued start serves, or
// nil for a start that serves none.
func automationRunFromCeilingPayload(payload map[string]interface{}) *automationRunLaunch {
	var run *automationRunLaunch
	decodeCeilingPayloadField(payload[ceilingPayloadAutomationRunKey], &run)
	if run == nil || run.RunID == "" {
		return nil
	}
	return run
}

// automationRunDispatchFor converts a task start into the exact identity its
// run binds. A start queued by the session ceiling is reported as
// automation.ErrRunDeferred, which keeps the run open for the replay.
func automationRunDispatchFor(taskID string, execution *executor.TaskExecution, err error) (automation.RunDispatch, error) {
	if errors.Is(err, ErrCeilingLaunchDeferred) {
		return automation.RunDispatch{}, automation.ErrRunDeferred
	}
	if err != nil {
		return automation.RunDispatch{}, err
	}
	if execution == nil || execution.SessionID == "" || execution.TurnID == "" {
		return automation.RunDispatch{}, errors.New("automation task start returned no session or turn identity")
	}
	return automation.RunDispatch{TaskID: taskID, SessionID: execution.SessionID, TurnID: execution.TurnID}, nil
}

// ceilingDetailAutomationRunClosed is the drop detail of a queued start whose
// automation run was stopped, bound, failed, or deleted.
const ceilingDetailAutomationRunClosed = "automation run is no longer open"

// errCeilingAutomationRunClosed reports that a queued start was not launched
// because its automation run is no longer open.
var errCeilingAutomationRunClosed = errors.New("orchestrator: queued start's automation run is no longer open")

// startQueuedAutomationRun replays a queued start through the run dispatcher,
// so the replayed launch binds its run or fails it. A launch whose run is not
// bound is stopped, because no completion would settle that run. A run that
// closed before dispatch is reported as errCeilingAutomationRunClosed without
// launching. A start that serves no run, or a service without the dispatcher,
// launches unbound.
func (s *Service) startQueuedAutomationRun(
	ctx context.Context, taskID string, run *automationRunLaunch, start func() (*executor.TaskExecution, error),
) (*executor.TaskExecution, error) {
	dispatcher, ok := s.automationService.(automationRunDispatcher)
	if run == nil || !ok {
		return start()
	}
	var execution *executor.TaskExecution
	err := dispatcher.DispatchRun(ctx, run.RunID, run.ThreadAction, run.ThreadReason, func() (automation.RunDispatch, error) {
		var startErr error
		execution, startErr = start()
		return automationRunDispatchFor(taskID, execution, startErr)
	})
	switch {
	case errors.Is(err, automation.ErrRunDeferred):
		return nil, ErrCeilingLaunchDeferred
	case errors.Is(err, automation.ErrAutomationRunNotDispatchable):
		return nil, errCeilingAutomationRunClosed
	case err != nil && execution != nil && execution.SessionID != "":
		s.cancelAutomationDispatch(ctx, taskID, execution.SessionID)
	}
	return execution, err
}

// automationRunAwaitsLaunch reports whether an admitted run is still open and
// unbound. A deleted run is not waiting. A run that cannot be read counts as
// waiting, because read uncertainty does not prove its queued start is
// obsolete.
func (s *Service) automationRunAwaitsLaunch(ctx context.Context, runID string) bool {
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return true
	}
	run, err := binding.GetRun(ctx, runID)
	if err != nil {
		return true
	}
	return run != nil && run.Status == automation.RunStatusTriggered
}

// failQueuedAutomationRun fails the run whose queued start is being dropped,
// since no launch will produce the completion that settles it. DispatchRun
// settles the run only while it still waits, under the lock that also
// serializes stop and binding, so a run that was already stopped, bound, or
// deleted is left alone. It reports false when the run may still be waiting,
// so the caller keeps the queued start for a later sweep to drop again.
func (s *Service) failQueuedAutomationRun(ctx context.Context, payload map[string]interface{}, reason string) bool {
	run := automationRunFromCeilingPayload(payload)
	dispatcher, ok := s.automationService.(automationRunDispatcher)
	if run == nil || !ok {
		return true
	}
	dropped := errors.New(reason)
	err := dispatcher.DispatchRun(ctx, run.RunID, run.ThreadAction, run.ThreadReason, func() (automation.RunDispatch, error) {
		return automation.RunDispatch{}, dropped
	})
	switch {
	case errors.Is(err, automation.ErrAutomationRunNotDispatchable):
		return true
	case err == dropped:
		// DispatchRun returns the callback's own error once the run is failed.
		s.logger.Info("failed the automation run of a dropped queued start",
			zap.String("run_id", run.RunID), zap.String("reason", reason))
		return true
	default:
		s.logger.Warn("could not fail the automation run of a dropped queued start",
			zap.String("run_id", run.RunID), zap.Error(err))
		return false
	}
}
