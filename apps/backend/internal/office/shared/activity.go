package shared

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"

	"go.uber.org/zap"
)

// ActivityRepo is the persistence interface required by ActivityLoggerImpl.
type ActivityRepo interface {
	// CreateActivityEntry persists an activity log entry.
	CreateActivityEntry(ctx context.Context, entry *models.ActivityEntry) error
}

// ActivityLoggerImpl is a concrete ActivityLogger backed by an ActivityRepo.
type ActivityLoggerImpl struct {
	repo   ActivityRepo
	logger *logger.Logger
}

// NewActivityLogger constructs an ActivityLoggerImpl.
func NewActivityLogger(repo ActivityRepo, log *logger.Logger) *ActivityLoggerImpl {
	return &ActivityLoggerImpl{repo: repo, logger: log}
}

// LogActivity records an activity entry. Errors are logged but not propagated
// so that callers are not disrupted by audit-log failures.
func (a *ActivityLoggerImpl) LogActivity(
	ctx context.Context,
	wsID, actorType, actorID, action, targetType, targetID, details string,
) {
	a.LogActivityWithRun(ctx, wsID, actorType, actorID, action, targetType, targetID, details, "", "")
}

// LogActivityWithRun records an activity entry tagged with the
// originating office run id (and optional session id). Pass empty
// strings for runID / sessionID when the action is genuinely user-
// initiated. The run detail page's Tasks Touched surface uses these
// columns to join activity rows back to the run that produced them.
func (a *ActivityLoggerImpl) LogActivityWithRun(
	ctx context.Context,
	wsID, actorType, actorID, action, targetType, targetID, details, runID, sessionID string,
) {
	entry := &models.ActivityEntry{
		WorkspaceID: wsID,
		ActorType:   models.ActivityActorType(actorType),
		ActorID:     actorID,
		Action:      models.ActivityAction(action),
		TargetType:  models.ActivityTargetType(targetType),
		TargetID:    targetID,
		Details:     details,
		RunID:       runID,
		SessionID:   sessionID,
	}
	if err := a.repo.CreateActivityEntry(ctx, entry); err != nil {
		a.logger.Error("failed to log activity",
			zap.String("action", action),
			zap.Error(err))
	}
}
