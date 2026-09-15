package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"

	"go.uber.org/zap"
)

// LogActivity creates an activity log entry. It is a convenience helper
// called from all office service mutations (agents, skills, budgets, etc.).
func (s *Service) LogActivity(
	ctx context.Context,
	workspaceID, actorType, actorID, action, targetType, targetID, details string,
) {
	s.LogActivityWithRun(ctx, workspaceID, actorType, actorID, action, targetType, targetID, details, "", "")
}

// LogActivityWithRun creates an activity log entry tagged with the
// originating office run (and optionally session) ID. This lets the
// run detail page's "Tasks Touched" surface join activity rows back
// to the run that produced them. Pass empty strings for runID /
// sessionID when the action is genuinely user-initiated.
func (s *Service) LogActivityWithRun(
	ctx context.Context,
	workspaceID, actorType, actorID, action, targetType, targetID, details, runID, sessionID string,
) {
	entry := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   models.ActivityActorType(actorType),
		ActorID:     actorID,
		Action:      models.ActivityAction(action),
		TargetType:  models.ActivityTargetType(targetType),
		TargetID:    targetID,
		Details:     details,
		RunID:       runID,
		SessionID:   sessionID,
	}
	if err := s.repo.CreateActivityEntry(ctx, entry); err != nil {
		s.logger.Error("failed to log activity",
			zap.String("action", action),
			zap.Error(err))
	}
}

// ResolveRunForTask returns the currently-claimed office run id for
// taskID if one exists, or empty string. Used by event subscribers
// (and the scheduler package) that act on behalf of an in-flight
// run but don't otherwise carry the run id through the call chain.
func (s *Service) ResolveRunForTask(ctx context.Context, taskID string) string {
	if taskID == "" {
		return ""
	}
	run, err := s.repo.GetClaimedRunByTaskID(ctx, taskID)
	if err != nil || run == nil {
		return ""
	}
	return run.ID
}

// AppendRunEvent writes a single run lifecycle event row and
// publishes a per-run-id notification on the event bus so the WS
// gateway can fan the event out to subscribed clients in real time.
// payload is expected to be compact JSON; pass nil to write the empty
// object. Persistence errors are logged and swallowed so the
// lifecycle pipeline never fails because the audit table is
// misbehaving; publish errors are likewise swallowed.
func (s *Service) AppendRunEvent(
	ctx context.Context,
	runID, eventType, level string,
	payload map[string]interface{},
) {
	if runID == "" {
		return
	}
	body := "{}"
	if len(payload) > 0 {
		if b, err := json.Marshal(payload); err == nil {
			body = string(b)
		}
	}
	evt, err := s.repo.AppendRunEvent(ctx, runID, eventType, level, body)
	if err != nil {
		s.logger.Warn("append run event failed",
			zap.String("run_id", runID),
			zap.String("event_type", eventType),
			zap.Error(err))
		return
	}
	s.publishRunEventAppended(ctx, evt)
}

// publishRunEventAppended emits the per-run notification on the event
// bus. Subjects are namespaced by run id so only clients subscribed
// to that run get the message.
func (s *Service) publishRunEventAppended(ctx context.Context, evt *models.RunEvent) {
	if s.eb == nil || evt == nil {
		return
	}
	subject := events.BuildOfficeRunEventSubject(evt.RunID)
	busEvent := bus.NewEvent(subject, "office-service", map[string]interface{}{
		"run_id": evt.RunID,
		"event": map[string]interface{}{
			"seq":        evt.Seq,
			"event_type": string(evt.EventType),
			"level":      string(evt.Level),
			"payload":    evt.Payload,
			"created_at": evt.CreatedAt.UTC().Format(time.RFC3339Nano),
		},
	})
	if err := s.eb.Publish(ctx, subject, busEvent); err != nil {
		s.logger.Debug("publish run event bus failed",
			zap.String("run_id", evt.RunID),
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
