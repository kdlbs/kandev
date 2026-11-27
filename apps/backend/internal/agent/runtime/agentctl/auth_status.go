package client

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// AuthStatus reads the transient provider authentication snapshot from the
// agent stream while an authenticate request is pending.
func (c *Client) AuthStatus(ctx context.Context, attemptID string) (streams.AuthStatus, error) {
	resp, err := c.sendStreamRequest(ctx, "agent.auth.status", map[string]string{"attempt_id": attemptID})
	if err != nil {
		return streams.AuthStatus{}, fmt.Errorf("auth status request failed: %w", err)
	}
	if resp.Type == ws.MessageTypeError {
		var errPayload ws.ErrorPayload
		if parseErr := resp.ParsePayload(&errPayload); parseErr != nil {
			return streams.AuthStatus{}, fmt.Errorf("auth status failed: unable to parse error")
		}
		return streams.AuthStatus{}, fmt.Errorf("auth status failed: %s", errPayload.Message)
	}
	var status streams.AuthStatus
	if err := resp.ParsePayload(&status); err != nil {
		return streams.AuthStatus{}, fmt.Errorf("parse auth status response: %w", err)
	}
	return status, nil
}
