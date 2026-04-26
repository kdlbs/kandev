package service

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// LogActivity creates an activity log entry. It is a convenience helper
// called from all orchestrate service mutations (agents, skills, budgets, etc.).
func (s *Service) LogActivity(
	ctx context.Context,
	workspaceID, actorType, actorID, action, targetType, targetID, details string,
) {
	entry := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   actorType,
		ActorID:     actorID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Details:     details,
	}
	if err := s.repo.CreateActivityEntry(ctx, entry); err != nil {
		s.logger.Error("failed to log activity",
			zap.String("action", action),
			zap.Error(err))
	}
}

// ListActivityFiltered returns activity entries filtered by optional criteria.
func (s *Service) ListActivityFiltered(
	ctx context.Context,
	wsID string,
	filterType string,
	limit int,
) ([]*models.ActivityEntry, error) {
	if filterType == "" || filterType == "all" {
		return s.repo.ListActivityEntries(ctx, wsID, limit)
	}
	return s.repo.ListActivityEntriesByType(ctx, wsID, filterType, limit)
}

// ListRecentActivity returns the most recent activity entries for a workspace.
func (s *Service) ListRecentActivity(
	ctx context.Context,
	wsID string,
	limit int,
) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.ListActivityEntries(ctx, wsID, limit)
}
