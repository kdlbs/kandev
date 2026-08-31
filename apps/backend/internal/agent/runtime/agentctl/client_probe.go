package client

import (
	"context"
)

const probeResultUnknown = "unknown"

// probeBackgroundWorkloadsRequest is the payload for agent.background.probe.
type probeBackgroundWorkloadsRequest struct {
	SessionID string `json:"session_id"`
}

// probeBackgroundWorkloadsResponse is the response payload for agent.background.probe.
type probeBackgroundWorkloadsResponse struct {
	Result string `json:"result"`
}

// ProbeBackgroundWorkloads asks agentctl whether any background processes
// spawned during the given ACP session are still alive. The argument must be
// the ACP session id (already translated from the Kandev task-session id by
// the caller). Every error and every unrecognised result value maps to
// "unknown"; the only values returned on success are "live", "settled", or
// "unknown".
func (c *Client) ProbeBackgroundWorkloads(ctx context.Context, acpSessionID string) (string, error) {
	resp, err := c.sendStreamRequest(ctx, "agent.background.probe", probeBackgroundWorkloadsRequest{
		SessionID: acpSessionID,
	})
	if err != nil {
		return probeResultUnknown, err
	}
	var payload probeBackgroundWorkloadsResponse
	if err := resp.ParsePayload(&payload); err != nil {
		return probeResultUnknown, err
	}
	switch payload.Result {
	case "live", "settled":
		return payload.Result, nil
	default:
		return probeResultUnknown, nil
	}
}
