package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// inheritFromParentSession copies missing profile IDs from the parent task's
// primary session. Returns the (possibly updated) agentProfileID,
// executorProfileID, and executorID.
func (s *Service) inheritFromParentSession(
	ctx context.Context,
	parentID, agentProfileID, executorProfileID, executorID string,
) (string, string, string) {
	parentSessions, err := s.repo.ListTaskSessions(ctx, parentID)
	if err != nil {
		s.logger.Warn("failed to list parent task sessions for profile inheritance",
			zap.String("parent_task_id", parentID),
			zap.Error(err))
		return agentProfileID, executorProfileID, executorID
	}

	ps := findPrimarySession(parentSessions)
	if ps == nil {
		return agentProfileID, executorProfileID, executorID
	}

	if agentProfileID == "" && ps.AgentProfileID != "" {
		agentProfileID = ps.AgentProfileID
	}
	if executorProfileID == "" && ps.ExecutorProfileID != "" {
		executorProfileID = ps.ExecutorProfileID
	}
	// executorID is only inherited when executorProfileID is also still empty
	// (including after the assignment above), because a profile takes precedence
	// over a bare executor ID.
	if executorID == "" && executorProfileID == "" && ps.ExecutorID != "" {
		executorID = ps.ExecutorID
	}

	return agentProfileID, executorProfileID, executorID
}

// findPrimarySession returns the first primary session from the list, or nil.
func findPrimarySession(sessions []*models.TaskSession) *models.TaskSession {
	for _, s := range sessions {
		if s.IsPrimary {
			return s
		}
	}
	return nil
}
