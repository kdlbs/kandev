package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

func (h *TaskHandlers) taskSessionSummariesWithPendingActions(
	ctx context.Context,
	sessions []*models.TaskSession,
) ([]dto.TaskSessionSummaryDTO, error) {
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	actions, revisions, err := h.service.GetPendingActionProjectionsForSessions(ctx, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("get task session pending actions: %w", err)
	}
	summaries := make([]dto.TaskSessionSummaryDTO, 0, len(sessions))
	for _, session := range sessions {
		summary := dto.FromTaskSessionSummary(session)
		dto.EnrichForegroundActivitySummary(&summary, h.foregroundActivity)
		dto.EnrichCancellationPendingSummary(&summary, h.cancellationPending)
		if isInputCapableSession(session) {
			summary.PendingAction = pendingActionPtr(&session.ID, actions)
		}
		summary.PendingActionRevision = pendingActionRevisionPtr(session.ID, revisions)
		summaries = append(summaries, summary)
	}
	return summaries, nil
}
