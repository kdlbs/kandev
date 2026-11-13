package orchestrator

import (
	"context"
	"errors"
	"fmt"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	runtimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// A failed launch can leave a historical starting claim without an owner.
// Reconcile it only for an explicit retry, under lifecycle ownership and the
// durable generation CAS. No timer, live launch or active turn is reclaimed.
func (s *Service) prepareFailedDynamicRouteRetry(ctx context.Context, request RouteActionRequest) error {
	if request.Action != RouteActionRetry || s.profileExecutionResolver == nil || !s.profileExecutionResolver.Enabled() {
		return nil
	}
	release := s.acquireSessionLifecycleLock(request.SessionID)
	defer release()
	session, err := s.repo.GetTaskSession(ctx, request.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.State != models.TaskSessionStateFailed ||
		session.RouteState != dynamicRouteStatusStarting || session.RouteGeneration <= 0 ||
		session.RouteGeneration != request.ExpectedGeneration {
		return nil
	}
	if err := s.checkFailedRouteRuntimeStopped(ctx, session); err != nil {
		return err
	}
	decision, err := s.profileExecutionResolver.MarkRouteActionRequired(ctx, session.ID, session.RouteGeneration, "failed_start_route_retry")
	if err != nil {
		return err
	}
	s.mirrorDynamicRouteProjection(ctx, session, session.RouteGeneration, decision.Status, decision.Reason)
	return nil
}

func (s *Service) checkFailedRouteRuntimeStopped(ctx context.Context, session *models.TaskSession) error {
	if s.agentManager == nil {
		return fmt.Errorf("%w: failed_route_manager_unavailable", dynamicruntime.ErrRecoveryPending)
	}
	running, err := s.probeAgentRunning(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("%w: failed_route_runtime_probe_unknown", dynamicruntime.ErrRecoveryPending)
	}
	if running {
		return fmt.Errorf("%w: failed_route_runtime_live", dynamicruntime.ErrRecoveryPending)
	}
	row, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		// The manual relaunch cold-resumes from this record; without it the
		// retried prompt cannot start a runtime, so keep the guard.
		return fmt.Errorf("%w: failed_route_runtime_record_absent", dynamicruntime.ErrRecoveryPending)
	}
	if err != nil {
		return fmt.Errorf("check failed route runtime owner: %w", err)
	}
	if !failedRouteHasStoppedLocalHistory(session, row) {
		return fmt.Errorf("%w: failed_route_runtime_record_not_stopped", dynamicruntime.ErrRecoveryPending)
	}
	return nil
}

// Normal cleanup preserves stopped rows for conversation and workspace resume.
// This predicate observes that history without deleting it or settling claims.
// last_seen_at is not required: a launch that failed before agentctl reported
// an endpoint or PID persists its terminal row without one, and liveness has
// already been probed by the caller.
func failedRouteHasStoppedLocalHistory(session *models.TaskSession, row *models.ExecutorRunning) bool {
	return row != nil && row.SessionID == session.ID && row.TaskID == session.TaskID &&
		row.AgentExecutionID != "" && row.AgentExecutionID == session.AgentExecutionID &&
		row.Runtime == runtimekind.RuntimeStandalone &&
		(row.ExecutorID == models.ExecutorIDLocal || row.ExecutorID == models.ExecutorIDWorktree) &&
		row.Status == models.ExecutorRunningStatusStopped && row.LocalPID == 0 && row.PID == 0 &&
		!row.UpdatedAt.IsZero()
}
