package orchestrator

import (
	"context"

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
