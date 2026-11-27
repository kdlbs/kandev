package automation

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// notifyRunRecorded reports a committed run insert to the observer, if any.
func (s *Store) notifyRunRecorded(ctx context.Context, automationID, runID string, status RunStatus) {
	if s.runRecorded != nil {
		s.runRecorded(ctx, automationID, runID, status)
	}
}

func (s *Store) automationWorkspaceID(ctx context.Context, automationID string) (string, error) {
	var workspaceID string
	err := s.ro.GetContext(ctx, &workspaceID, s.ro.Rebind(`SELECT workspace_id FROM automations WHERE id = ?`), automationID)
	return workspaceID, err
}

// publishRunRecorded announces a recorded run to the automation's workspace
// so open run pages refresh. It carries identifiers and status only, and a
// failure never affects the write that recorded the run.
func (s *Service) publishRunRecorded(ctx context.Context, automationID, runID string, status RunStatus) {
	workspaceID, err := s.store.automationWorkspaceID(ctx, automationID)
	if err != nil || workspaceID == "" {
		s.logger.Warn("skipped automation.run.created: workspace unknown",
			zap.String("automation_id", automationID), zap.String("run_id", runID), zap.Error(err))
		return
	}
	data := map[string]any{
		"workspace_id": workspaceID, "automation_id": automationID, "run_id": runID, "status": string(status),
	}
	event := bus.NewEvent(events.AutomationRunCreated, "automation_service", data)
	if err := s.eventBus.Publish(ctx, events.AutomationRunCreated, event); err != nil {
		s.logger.Warn("failed to publish automation.run.created",
			zap.String("automation_id", automationID), zap.String("run_id", runID), zap.Error(err))
	}
}
