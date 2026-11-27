package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// dynamicFailureSession returns the routed session a failure may advance: the
// failed execution must be the session's execution and must run the candidate
// the route holds now.
func (s *Service) dynamicFailureSession(
	ctx context.Context,
	data watcher.AgentEventData,
) (*models.TaskSession, bool) {
	if s.profileExecutionResolver == nil || data.SessionID == "" {
		return nil, false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return nil, false
	}
	if session.AgentExecutionID != "" && data.AgentExecutionID != "" &&
		session.AgentExecutionID != data.AgentExecutionID {
		return nil, false
	}
	if failureFromSupersededCandidate(data, session) {
		return nil, false
	}
	return session, true
}

// failureFromSupersededCandidate reports whether the failed execution runs a
// different concrete profile than the route's current candidate. The route
// records its successor before the predecessor's process is replaced, and a
// deferred successor launch leaves the predecessor serving the session, so a
// failure of that execution belongs to the predecessor's profile and must not
// be charged to the successor.
func failureFromSupersededCandidate(data watcher.AgentEventData, session *models.TaskSession) bool {
	return data.ExecutionProfileID != "" && session != nil &&
		session.ExecutionProfileID != data.ExecutionProfileID
}

// recordIneligibleFailureCircuit charges a classified failure that may not
// advance the route, such as one reported after the attempt produced output
// or effects, to the shared circuit of the candidate that produced it. The
// route, its generation, and the recovery surface are left unchanged, so the
// failed turn stays with the operator while later selections skip the
// unhealthy candidate. The engine fences the failure to the route's current
// generation and candidate and ignores codes that do not qualify.
func (s *Service) recordIneligibleFailureCircuit(
	ctx context.Context,
	session *models.TaskSession,
	classified *routingerr.Error,
) {
	if s.profileExecutionResolver == nil || session == nil || classified == nil {
		return
	}
	opened, err := s.profileExecutionResolver.RecordFailureCircuit(
		ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
		session.RouteGeneration, classified,
	)
	if err != nil {
		if !errors.Is(err, dynamicruntime.ErrStaleGeneration) {
			s.logger.Warn("failed to record dynamic candidate circuit for a non-routable failure",
				zap.String("session_id", session.ID),
				zap.String("execution_profile_id", session.ExecutionProfileID), zap.Error(err))
		}
		return
	}
	if opened {
		s.logger.Info("opened dynamic candidate circuit for a non-routable failure",
			zap.String("session_id", session.ID),
			zap.String("execution_profile_id", session.ExecutionProfileID),
			zap.String("error_code", string(classified.Code)))
	}
}

// resetDynamicCircuitFailuresForCompletion clears the consecutive failure
// count of the candidate whose turn completed without a provider error, so a
// later failure restarts the backoff ladder. A completion of a superseded
// execution or candidate clears nothing.
func (s *Service) resetDynamicCircuitFailuresForCompletion(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) {
	if s.profileExecutionResolver == nil || session == nil || session.RouteGeneration <= 0 ||
		session.ExecutionProfileID == "" || session.AgentProfileID == "" {
		return
	}
	if session.AgentExecutionID != "" && data.AgentExecutionID != "" &&
		session.AgentExecutionID != data.AgentExecutionID {
		return
	}
	if failureFromSupersededCandidate(data, session) {
		return
	}
	if _, err := s.profileExecutionResolver.ResetCircuitFailures(
		ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID, session.RouteGeneration,
	); err != nil && !errors.Is(err, dynamicruntime.ErrStaleGeneration) {
		s.logger.Debug("could not reset dynamic candidate circuit failures after a completed turn",
			zap.String("session_id", session.ID), zap.Error(err))
	}
}
