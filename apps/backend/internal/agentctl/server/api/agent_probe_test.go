package api

import (
	"context"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleAgentStreamRequest_DispatchesBackgroundProbe verifies
// "agent.background.probe" reaches handleWSBackgroundProbe and, with no
// agent process running, resolves to "unknown" per procMgr.ProbeProcessTree's
// own no-PID contract.
func TestHandleAgentStreamRequest_DispatchesBackgroundProbe(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-probe", "agent.background.probe", map[string]string{"session_id": "acp-session-1"})
	resp := s.handleAgentStreamRequest(ctx, msg)

	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.ID != "req-probe" {
		t.Errorf("response ID = %q, want req-probe", resp.ID)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response type, got %q", resp.Type)
	}
	var payload struct {
		Result string `json:"result"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if payload.Result != "unknown" {
		t.Fatalf("result = %q, want unknown (no agent process running)", payload.Result)
	}
}

// TestHandleWSBackgroundProbe_MalformedPayloadResolvesToUnknown verifies
// AC-46's fourth condition (unparseable response body — mirrored here as an
// unparseable request payload) resolves to "unknown" rather than an error.
func TestHandleWSBackgroundProbe_MalformedPayloadResolvesToUnknown(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg := &ws.Message{ID: "req-bad", Action: "agent.background.probe", Payload: []byte(`"not an object"`)}
	resp := s.handleWSBackgroundProbe(ctx, msg)

	if resp == nil {
		t.Fatal("expected response")
	}
	var payload struct {
		Result string `json:"result"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if payload.Result != "unknown" {
		t.Fatalf("result = %q, want unknown", payload.Result)
	}
}
