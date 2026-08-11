package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type recordingTunnelController struct {
	startSession string
	startPort    int
	requested    int
	startResult  int
	startErr     error
	stopSession  string
	stopPort     int
	stopErr      error
	tunnels      any
}

func (c *recordingTunnelController) StartTunnel(sessionID string, port, tunnelPort int) (int, error) {
	c.startSession, c.startPort, c.requested = sessionID, port, tunnelPort
	return c.startResult, c.startErr
}

func (c *recordingTunnelController) StopTunnel(sessionID string, port int) error {
	c.stopSession, c.stopPort = sessionID, port
	return c.stopErr
}

func (c *recordingTunnelController) ListTunnels(string) any { return c.tunnels }

func TestPortHandlersRegistrationDependsOnTunnelController(t *testing.T) {
	dispatcher := ws.NewDispatcher()
	NewPortHandlers(nil, nil, newTestLogger()).RegisterHandlers(dispatcher)
	if !dispatcher.HasHandler(ws.ActionPortList) {
		t.Fatal("port list handler was not registered")
	}
	if dispatcher.HasHandler(ws.ActionPortTunnelStart) {
		t.Fatal("tunnel handler registered without a controller")
	}

	dispatcher = ws.NewDispatcher()
	NewPortHandlers(nil, &recordingTunnelController{}, newTestLogger()).RegisterHandlers(dispatcher)
	for _, action := range []string{ws.ActionPortTunnelStart, ws.ActionPortTunnelStop, ws.ActionPortTunnelList} {
		if !dispatcher.HasHandler(action) {
			t.Fatalf("handler %s was not registered", action)
		}
	}
}

func TestTunnelStartValidationDoesNotCallController(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		code    string
	}{
		{name: "malformed", payload: "invalid", code: ws.ErrorCodeBadRequest},
		{name: "missing session", payload: map[string]any{"port": 3000}, code: ws.ErrorCodeValidation},
		{name: "port too low", payload: map[string]any{"session_id": "s", "port": 0}, code: ws.ErrorCodeValidation},
		{name: "port too high", payload: map[string]any{"session_id": "s", "port": 65536}, code: ws.ErrorCodeValidation},
		{name: "tunnel port too high", payload: map[string]any{"session_id": "s", "port": 3000, "tunnel_port": 65536}, code: ws.ErrorCodeValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := &recordingTunnelController{}
			h := NewPortHandlers(nil, ctrl, newTestLogger())
			resp, err := h.wsTunnelStart(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStart, tt.payload))
			if err != nil {
				t.Fatalf("wsTunnelStart returned error: %v", err)
			}
			assertWSErrorCode(t, resp, tt.code)
			if ctrl.startSession != "" {
				t.Fatal("controller called for invalid request")
			}
		})
	}
}

func TestTunnelStartForwardsRequestAndReturnsAllocatedPort(t *testing.T) {
	ctrl := &recordingTunnelController{startResult: 43123}
	h := NewPortHandlers(nil, ctrl, newTestLogger())
	resp, err := h.wsTunnelStart(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStart, map[string]any{
		"session_id": "session-1", "port": 3000, "tunnel_port": 0,
	}))
	if err != nil {
		t.Fatalf("wsTunnelStart returned error: %v", err)
	}
	if ctrl.startSession != "session-1" || ctrl.startPort != 3000 || ctrl.requested != 0 {
		t.Fatalf("unexpected forwarded request: %+v", ctrl)
	}
	assertResponseField(t, resp, "tunnel_port", float64(43123))
}

func TestTunnelStartMapsControllerFailure(t *testing.T) {
	ctrl := &recordingTunnelController{startErr: errors.New("bind failed")}
	h := NewPortHandlers(nil, ctrl, newTestLogger())
	resp, err := h.wsTunnelStart(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStart, map[string]any{
		"session_id": "session-1", "port": 3000,
	}))
	if err != nil {
		t.Fatalf("wsTunnelStart returned error: %v", err)
	}
	assertWSErrorCode(t, resp, ws.ErrorCodeInternalError)
}

func TestTunnelStopAndListContracts(t *testing.T) {
	ctrl := &recordingTunnelController{tunnels: []map[string]any{{"port": 3000, "tunnel_port": 43123}}}
	h := NewPortHandlers(nil, ctrl, newTestLogger())
	stop, err := h.wsTunnelStop(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStop, map[string]any{
		"session_id": "session-1", "port": 3000,
	}))
	if err != nil || stop == nil {
		t.Fatalf("wsTunnelStop = (%v, %v)", stop, err)
	}
	if ctrl.stopSession != "session-1" || ctrl.stopPort != 3000 {
		t.Fatalf("unexpected stop request: %+v", ctrl)
	}

	list, err := h.wsTunnelList(context.Background(), tunnelTestMessage(ws.ActionPortTunnelList, map[string]any{"session_id": "session-1"}))
	if err != nil {
		t.Fatalf("wsTunnelList returned error: %v", err)
	}
	assertResponseField(t, list, "tunnels", []any{map[string]any{"port": float64(3000), "tunnel_port": float64(43123)}})
}

func TestTunnelStopValidationAndFailure(t *testing.T) {
	ctrl := &recordingTunnelController{stopErr: errors.New("missing tunnel")}
	h := NewPortHandlers(nil, ctrl, newTestLogger())
	for _, payload := range []any{"invalid", map[string]any{"port": 3000}, map[string]any{"session_id": "s", "port": 0}} {
		resp, err := h.wsTunnelStop(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStop, payload))
		if err != nil || resp == nil {
			t.Fatalf("validation response = (%v, %v)", resp, err)
		}
	}
	resp, err := h.wsTunnelStop(context.Background(), tunnelTestMessage(ws.ActionPortTunnelStop, map[string]any{"session_id": "s", "port": 3000}))
	if err != nil {
		t.Fatalf("wsTunnelStop returned error: %v", err)
	}
	assertWSErrorCode(t, resp, ws.ErrorCodeInternalError)
}

func TestTunnelListValidation(t *testing.T) {
	h := NewPortHandlers(nil, &recordingTunnelController{}, newTestLogger())
	for _, payload := range []any{"invalid", map[string]any{}} {
		resp, err := h.wsTunnelList(context.Background(), tunnelTestMessage(ws.ActionPortTunnelList, payload))
		if err != nil || resp == nil {
			t.Fatalf("validation response = (%v, %v)", resp, err)
		}
	}
}

func assertWSErrorCode(t *testing.T, message *ws.Message, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got := payload["code"]; got != want {
		t.Fatalf("error code = %v, want %s", got, want)
	}
}

func assertResponseField(t *testing.T, message *ws.Message, field string, want any) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got := payload[field]; !deepEqualJSON(got, want) {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}

func tunnelTestMessage(action string, payload any) *ws.Message {
	if payload == "invalid" {
		return &ws.Message{ID: "1", Action: action, Payload: json.RawMessage(`{invalid`)}
	}
	message, _ := ws.NewRequest("1", action, payload)
	return message
}
