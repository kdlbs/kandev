package api

import (
	"context"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleWSSetConfigOption_NoAdapter verifies the WS handler surfaces a
// clean "agent not running" error when no adapter is connected — same as the
// other adapter-action handlers (set_mode, set_model, authenticate).
func TestHandleWSSetConfigOption_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.session.set_config_option", map[string]string{
		"config_id": "model",
		"value":     "claude-3-7-sonnet",
	})
	resp := s.handleWSSetConfigOption(ctx, msg)

	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if !strings.Contains(errPayload.Message, "agent not running") {
		t.Errorf("expected 'agent not running', got %q", errPayload.Message)
	}
}

// TestHandleWSSetConfigOption_BadPayload verifies the handler rejects
// malformed payloads with BAD_REQUEST (the adapterAction helper parses the
// payload before checking the adapter, so a malformed payload errors first).
func TestHandleWSSetConfigOption_BadPayload(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// Use a non-JSON-decodable payload to trip the parse path.
	msg, _ := ws.NewRequest("req-1", "agent.session.set_config_option", "not-an-object")
	resp := s.handleWSSetConfigOption(ctx, msg)

	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %q", errPayload.Code)
	}
}
