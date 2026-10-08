package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func (a *messageCreatorAdapter) PublishRecoveryMessage(ctx context.Context, message *models.Message) error {
	turn, err := a.svc.GetTurn(ctx, message.TurnID)
	if err != nil {
		return err
	}
	if err = a.svc.PublishTurnStarted(ctx, turn); err != nil {
		return err
	}
	return a.svc.PublishMessageEvent(ctx, events.MessageAdded, message)
}
