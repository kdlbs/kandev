package handlers

import (
	"context"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/review"
)

// mcpReviewOrigin uses the actual caller for its own task and the primary
// writer for another task; payload fields are never used as writer identity.
func (h *Handlers) mcpReviewOrigin(ctx context.Context, taskID string, principal mcpscope.Principal) *review.OriginIdentity {
	if taskID == principal.CallerTaskID {
		if h.sessionRepo == nil {
			return nil
		}
		session, err := h.sessionRepo.GetTaskSession(ctx, principal.CallerSessionID)
		if err != nil || session == nil || session.TaskID != taskID {
			return nil
		}
		return review.OriginFromSession(session)
	}
	sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, session := range sessions {
		if session != nil && session.IsPrimary {
			return review.OriginFromSession(session)
		}
	}
	return nil
}
