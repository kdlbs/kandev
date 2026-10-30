package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// publishOfficeTurnSettlement confirms that this turn no longer owns a runtime
// teardown. The original turn start fences delayed delivery against a successor
// run claimed on the same persistent Office session.
func (s *Service) publishOfficeTurnSettlement(
	ctx context.Context, taskID, sessionID string, snapshot *models.TaskSession, turnID string,
) {
	if s.eventBus == nil || snapshot == nil || snapshot.ErrorMessage != "" {
		return
	}
	startedAt := snapshot.UpdatedAt
	if turnID != "" && s.turnService != nil {
		turn, err := s.turnService.GetTurn(ctx, turnID)
		if err != nil || turn == nil || turn.TaskSessionID != sessionID || turn.TaskID != taskID {
			s.logger.Warn("cannot attribute completed Office turn settlement",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
			return
		}
		startedAt = turn.StartedAt
	} else if s.sessionTurnAdvancedSince(ctx, sessionID, turnID) {
		return
	}
	if startedAt.IsZero() {
		return
	}
	data := map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"agent_profile_id":   snapshot.AgentProfileID,
		"agent_execution_id": snapshot.AgentExecutionID,
		"turn_id":            turnID,
		"turn_started_at":    startedAt.Format(time.RFC3339Nano),
		"updated_at":         snapshot.UpdatedAt.Format(time.RFC3339Nano),
	}
	if err := s.eventBus.Publish(ctx, events.OfficeTurnSettled,
		bus.NewEvent(events.OfficeTurnSettled, "orchestrator", data)); err != nil {
		s.logger.Warn("cannot publish completed Office turn settlement",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}
