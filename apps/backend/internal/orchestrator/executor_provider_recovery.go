package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) recordProviderRecovery(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.ConversationOutcome == "" {
		return
	}
	writer, ok := s.repo.(interface {
		RecordProviderRecovery(context.Context, string, string, string, string, string) (*models.Message, error)
	})
	if !ok {
		return
	}
	if current := s.currentACPSessionID(data.SessionID); current != "" && current != data.ACPSessionID {
		return
	}
	message, err := writer.RecordProviderRecovery(ctx, data.TaskID, data.SessionID, data.AgentExecutionID, data.ACPSessionID, data.ConversationOutcome)
	if err != nil {
		s.logger.Warn("provider recovery outcome persistence deferred", zap.Error(err))
		return
	}
	if publisher, ok := s.messageCreator.(interface {
		PublishRecoveryMessage(context.Context, *models.Message) error
	}); ok {
		if err = publisher.PublishRecoveryMessage(ctx, message); err != nil {
			s.logger.Warn("provider recovery outcome delivery deferred", zap.Error(err))
		}
	}
}
