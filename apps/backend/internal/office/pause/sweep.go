package pause

import (
	"context"
	"errors"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/office/models"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
)

// haltSweepCancelReason is the literal written to runs.cancel_reason for
// every run the sweep cancels, and passed to TaskCanceller.
// CancelTaskExecution as the cancellation reason (F50) — matching the
// sibling literals this capability already pins (skip_reason and the
// wakeup-skip reason) for cross-run debugging consistency.
const haltSweepCancelReason = "workspace_paused"

// SweepResult is the halt sweep's observable outcome, returned in the
// pause response body (AC-003.7, AC-004.6).
type SweepResult struct {
	RunsCancelled        int64 `json:"runs_cancelled"`
	ExecutionsCancelled  int   `json:"executions_cancelled"`
	ExecutionsNotRunning int   `json:"executions_not_running"`
	Failures             int   `json:"failures"`
}

// runHaltSweep cancels queued/claimed Office runs, releases the task
// checkouts they held, and requests cancellation of in-flight task
// executions for the union of two sources: runs' payload task ids, and
// live heavy-routine task ids with no runs row. Best-effort throughout —
// every sub-step logs and continues rather than aborting the sweep
// (-003.7); F48's accepted recommendation folds every sub-step's failure
// into the single `failures` count, not just sub-step (d)'s.
func (s *Service) runHaltSweep(ctx context.Context, workspaceID string) SweepResult {
	var result SweepResult
	s.cancelRunSessions(ctx, workspaceID, &result)

	runs, err := s.repo.ListInflightRunsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("halt sweep: list inflight runs failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		result.Failures++
		runs = nil
	}

	runIDs := make([]string, 0, len(runs))
	taskIDSet := map[string]struct{}{}
	for _, run := range runs {
		runIDs = append(runIDs, run.RunID)
		if run.TaskID != "" {
			taskIDSet[run.TaskID] = struct{}{}
		}
	}

	if len(runIDs) > 0 {
		cancelled, err := s.repo.CancelRunsForWorkspace(ctx, runIDs, haltSweepCancelReason)
		result.RunsCancelled = cancelled
		if err != nil {
			s.logger.Warn("halt sweep: cancel runs failed",
				zap.String("workspace_id", workspaceID), zap.Error(err))
			result.Failures++
		}

		if err := s.repo.ReleaseCheckoutsForWorkspace(ctx, runIDs); err != nil {
			s.logger.Warn("halt sweep: release checkouts failed",
				zap.String("workspace_id", workspaceID), zap.Error(err))
			result.Failures++
		}
	}

	liveRoutineTaskIDs, err := s.repo.ListLiveRoutineTaskIDsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("halt sweep: list live routine tasks failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		result.Failures++
	}
	for _, taskID := range liveRoutineTaskIDs {
		if taskID != "" {
			taskIDSet[taskID] = struct{}{}
		}
	}

	liveOfficeTaskIDs, err := s.repo.ListLiveOfficeTaskIDsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("halt sweep: list live Office task sessions failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		result.Failures++
	}
	for _, taskID := range liveOfficeTaskIDs {
		if taskID != "" {
			taskIDSet[taskID] = struct{}{}
		}
	}

	s.cancelTaskExecutions(ctx, taskIDSet, &result)
	return result
}

func (s *Service) cancelRunSessions(ctx context.Context, workspaceID string, result *SweepResult) {
	sessions, err := s.repo.ListLiveRunSessionsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("halt sweep: list live run sessions failed", zap.String("workspace_id", workspaceID), zap.Error(err))
		result.Failures++
		return
	}
	for _, session := range sessions {
		if _, err := s.repo.RequestRunSessionCancellation(ctx, session.ID); err != nil {
			result.Failures++
			s.logger.Warn("halt sweep: mark run session cancelled failed", zap.String("run_session_id", session.ID), zap.Error(err))
			continue
		}
		if s.runStopper == nil {
			result.Failures++
			s.logger.Warn("halt sweep: run execution stopper unavailable", zap.String("run_session_id", session.ID))
			continue
		}
		err := s.runStopper.Stop(ctx, session.ExecutionID, haltSweepCancelReason)
		switch {
		case err == nil:
			result.ExecutionsCancelled++
			if _, finishErr := s.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateCancelled, haltSweepCancelReason); finishErr != nil {
				result.Failures++
				s.logger.Warn("halt sweep: finish run session cancellation failed", zap.String("run_session_id", session.ID), zap.Error(finishErr))
			}
		case errors.Is(err, orchestratorexecutor.ErrExecutionNotFound), errors.Is(err, runtimeapi.ErrNotFound):
			result.ExecutionsNotRunning++
			if _, finishErr := s.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateInterrupted, "runtime execution not found"); finishErr != nil {
				result.Failures++
				s.logger.Warn("halt sweep: mark missing run session interrupted failed", zap.String("run_session_id", session.ID), zap.Error(finishErr))
			}
		default:
			result.Failures++
			s.logger.Warn("halt sweep: stop run execution failed", zap.String("run_session_id", session.ID), zap.Error(err))
		}
	}
}

// cancelTaskExecutions requests cancellation for the deduplicated task id
// union, partitioning the outcome into three counts: an actual stop, a
// task with nothing to stop (ErrExecutionNotFound — expected for an idle
// task, not a failure), and every other error.
func (s *Service) cancelTaskExecutions(ctx context.Context, taskIDs map[string]struct{}, result *SweepResult) {
	for taskID := range taskIDs {
		err := s.canceller.CancelTaskExecution(ctx, taskID, haltSweepCancelReason, true)
		switch {
		case err == nil:
			result.ExecutionsCancelled++
		case errors.Is(err, orchestratorexecutor.ErrExecutionNotFound):
			result.ExecutionsNotRunning++
			s.logger.Debug("halt sweep: no live execution for task",
				zap.String("task_id", taskID))
		default:
			result.Failures++
			s.logger.Warn("halt sweep: cancel task execution failed",
				zap.String("task_id", taskID), zap.Error(err))
		}
	}
}
