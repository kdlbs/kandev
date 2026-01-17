package providers

import (
	"context"
	"fmt"

	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type LocalProvider struct {
	hub *gatewayws.Hub
}

func NewLocalProvider(hub *gatewayws.Hub) *LocalProvider {
	return &LocalProvider{hub: hub}
}

func (p *LocalProvider) Available() bool {
	return p.hub != nil
}

func (p *LocalProvider) Validate(_ map[string]interface{}) error {
	return nil
}

func (p *LocalProvider) Send(_ context.Context, message Message) error {
	if p.hub == nil {
		return fmt.Errorf("websocket hub not available")
	}
	msg, err := ws.NewNotification(ws.ActionTaskSessionWaitingForInput, map[string]interface{}{
		"task_id":         message.TaskID,
		"task_session_id": message.TaskSessionID,
		"title":           message.Title,
		"body":            message.Body,
	})
	if err != nil {
		return err
	}
	p.hub.BroadcastToUser(message.UserID, msg)
	return nil
}
