package service

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// Persistence is the boundary for output evidence; chunks do not emit events.
func (s *Service) logMessageFlow(ctx context.Context, taskID string, message *models.Message) {
	s.logger.Flow(ctx, logger.ExecutionFlow{Stage: "message", Outcome: "persisted", TaskID: taskID, SessionID: message.TaskSessionID, TurnID: message.TurnID, MessageID: message.ID, Reason: string(message.Type), AuthorType: string(message.AuthorType)})
}
