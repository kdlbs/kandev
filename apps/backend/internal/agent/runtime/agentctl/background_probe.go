package client

import (
	"context"
	"fmt"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// ProbeResult mirrors agentctl's agent.background.probe response literal
// (spec docs/specs/disambiguate-waiting/spec.md, AC-45). Defined
// independently of agentctl-server-side probe.Result: this package talks to
// agentctl only over the WS wire contract, never by importing its types.
type ProbeResult string

const (
	ProbeResultLive    ProbeResult = "live"
	ProbeResultSettled ProbeResult = "settled"
	ProbeResultUnknown ProbeResult = "unknown"
)

// backgroundProbeRequest is the agent.background.probe wire request. It
// carries no timestamp (AC-45) — the turn start it compares against was
// already recorded agentctl-side (spec D3).
type backgroundProbeRequest struct {
	SessionID string `json:"session_id"`
}

// backgroundProbeResponse is the agent.background.probe wire response.
type backgroundProbeResponse struct {
	Result string `json:"result"`
}

// ProbeBackgroundWorkloads samples the agent process's transitive
// descendant set for background-workload liveness via the agent WebSocket
// stream (spec §"Probe transport"). Every failure mode — stream
// disconnection, context deadline, a WS error frame (including
// ErrorCodeUnknownAction), an unparseable/absent body, or a result value
// outside the three literals — resolves to ProbeResultUnknown; this method
// never returns a result outside that three-literal union (AC-46).
func (c *Client) ProbeBackgroundWorkloads(ctx context.Context, sessionID string) (ProbeResult, error) {
	payload := backgroundProbeRequest{SessionID: sessionID}

	resp, err := c.sendStreamRequest(ctx, "agent.background.probe", payload)
	if err != nil {
		return ProbeResultUnknown, fmt.Errorf("background probe request failed: %w", err)
	}

	if resp.Type == ws.MessageTypeError {
		var errPayload ws.ErrorPayload
		if parseErr := resp.ParsePayload(&errPayload); parseErr != nil {
			return ProbeResultUnknown, fmt.Errorf("background probe failed: unable to parse error")
		}
		return ProbeResultUnknown, fmt.Errorf("background probe failed: %s", errPayload.Message)
	}

	var result backgroundProbeResponse
	if err := resp.ParsePayload(&result); err != nil {
		return ProbeResultUnknown, fmt.Errorf("failed to parse background probe response: %w", err)
	}

	switch ProbeResult(result.Result) {
	case ProbeResultLive, ProbeResultSettled, ProbeResultUnknown:
		return ProbeResult(result.Result), nil
	default:
		return ProbeResultUnknown, fmt.Errorf("background probe returned an unrecognized result %q", result.Result)
	}
}
