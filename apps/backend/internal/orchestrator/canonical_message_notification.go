package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func canonicalMessageNotification(ctx context.Context, reader canonicalMessageReader, payload *runtime.AgentStreamEventPayload) (*models.Message, *models.ConversationMutationReceipt, error) {
	receipt := payload.Data.ConversationReceipt
	if receipt != nil && receipt.SessionID == payload.SessionID {
		for _, operation := range receipt.Operations {
			if operation.Entity == models.ConversationEntityMessage && operation.Kind == models.ConversationMutationUpsert &&
				operation.ID == payload.Data.MessageID && operation.Message != nil && operation.Message.TaskSessionID == payload.SessionID {
				return operation.Message, receipt, nil
			}
		}
	}
	// Legacy or incomplete notification evidence still requests source recovery.
	message, err := reader.GetMessage(ctx, payload.Data.MessageID)
	return message, nil, err
}
