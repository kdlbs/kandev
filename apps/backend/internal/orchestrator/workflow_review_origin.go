package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/steptelemetry"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// Prefer the committed arrival's writer. The engine's ledger-owned state can
// have no session, and the task's primary may already have changed on entry.
func (s *Service) reviewWriterSession(ctx context.Context, taskID, stateSessionID string) *taskmodels.TaskSession {
	if s.repo == nil || taskID == "" {
		return nil
	}
	for _, id := range []string{stateSessionID, steptelemetry.FromContext(ctx).SessionID} {
		if id == "" {
			continue
		}
		session, err := s.repo.GetTaskSession(ctx, id)
		if err != nil || session == nil {
			return nil
		}
		if session.TaskID == taskID {
			return session
		}
	}
	if !steptelemetry.HasTrigger(ctx) {
		return nil
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, session := range sessions {
		if session != nil && session.IsPrimary {
			return session
		}
	}
	return nil
}

func (s *Service) reviewOrigin(ctx context.Context, taskID, stateSessionID string) (*review.OriginIdentity, string) {
	writer := s.reviewWriterSession(ctx, taskID, stateSessionID)
	if writer == nil {
		return nil, stateSessionID
	}
	return review.OriginFromSession(writer), writer.ID
}
