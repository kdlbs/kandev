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
	msg, err := ws.NewNotification(message.EventType, map[string]interface{}{
		"task_id":               message.TaskID,
		"session_id":            message.TaskSessionID,
		"occurrence_id":         message.OccurrenceID,
		"title":                 message.Title,
		"body":                  message.Body,
		"version":               message.Payload["version"],
		"url":                   message.Payload["url"],
		"agent_name":            message.Payload["agent_name"],
		"runtime_id":            message.Payload["runtime_id"],
		"display_name":          message.Payload["display_name"],
		"previous_version":      message.Payload["previous_version"],
		"runtime_update_status": message.Payload["runtime_update_status"],
	})
	if err != nil {
		return err
	}
	msg.ID = message.OccurrenceID
	if !p.hub.BroadcastToUser(message.UserID, msg) {
		return ErrNoEligibleSubscriber
	}
	return nil
}
