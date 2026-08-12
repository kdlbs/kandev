package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// mockProxyInvalidator tracks InvalidateProxy calls.
type mockProxyInvalidator struct {
	invalidated []string
}

func (m *mockProxyInvalidator) InvalidateProxy(sessionID string) {
	m.invalidated = append(m.invalidated, sessionID)
}

func TestNewVscodeHandlers(t *testing.T) {
	log := newTestLogger()
	proxy := &mockProxyInvalidator{}
	h := NewVscodeHandlers(nil, proxy, log)

	if h == nil {
		t.Fatal("expected non-nil handlers")
	}
	if h.proxyInvalidator == nil {
		t.Error("expected proxyInvalidator to be set")
	}
}

func TestBuildVscodeProxyURL(t *testing.T) {
	if got := buildVscodeProxyURL("session", ""); got != "/vscode/session/" {
		t.Fatalf("URL = %q", got)
	}
	if got := buildVscodeProxyURL("session", "/work/my repo"); got != "/vscode/session/?folder=%2Fwork%2Fmy+repo" {
		t.Fatalf("URL = %q", got)
	}
}

func TestVscodeHandlersSuccessContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/vscode/start":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["theme"] != "dark" {
				t.Errorf("start body = %v", body)
			}
			_, _ = w.Write([]byte(`{"success":true,"status":"starting","port":43123}`))
		case "/api/v1/vscode/status":
			_, _ = w.Write([]byte(`{"status":"running","port":43123,"url":"internal"}`))
		case "/api/v1/vscode/open-file":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["path"] != "main.go" || body["line"] != float64(12) {
				t.Errorf("open body = %v", body)
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/vscode/stop":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	h, proxy := vscodeHandlerServer(t, server)

	start, err := h.wsVscodeStart(context.Background(), vscodeMessage(t, ws.ActionVscodeStart, VscodeStartRequest{SessionID: "s", Theme: "dark"}))
	if err != nil || !strings.Contains(string(start.Payload), `"port":43123`) || !strings.Contains(string(start.Payload), `folder=%2Fwork%2Fmy+repo`) {
		t.Fatalf("start = (%s, %v)", start.Payload, err)
	}
	status, err := h.wsVscodeStatus(context.Background(), vscodeMessage(t, ws.ActionVscodeStatus, VscodeStatusRequest{SessionID: "s"}))
	if err != nil || !strings.Contains(string(status.Payload), `"status":"running"`) || strings.Contains(string(status.Payload), `"url":"internal"`) {
		t.Fatalf("status = (%s, %v)", status.Payload, err)
	}
	opened, err := h.wsVscodeOpenFile(context.Background(), vscodeMessage(t, ws.ActionVscodeOpenFile, VscodeOpenFileRequest{SessionID: "s", Path: "main.go", Line: 12, Col: 3}))
	if err != nil || !strings.Contains(string(opened.Payload), `"success":true`) {
		t.Fatalf("open = (%s, %v)", opened.Payload, err)
	}
	stopped, err := h.wsVscodeStop(context.Background(), vscodeMessage(t, ws.ActionVscodeStop, VscodeStopRequest{SessionID: "s"}))
	if err != nil || !strings.Contains(string(stopped.Payload), `"status":"stopped"`) || len(proxy.invalidated) != 1 || proxy.invalidated[0] != "s" {
		t.Fatalf("stop = (%s, %v), invalidated=%v", stopped.Payload, err, proxy.invalidated)
	}
}

func TestVscodeHandlersMapDependencyFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failure", http.StatusBadGateway)
	}))
	defer server.Close()
	h, _ := vscodeHandlerServer(t, server)
	tests := []struct {
		name   string
		action string
		body   any
		invoke func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		{name: "start", action: ws.ActionVscodeStart, body: VscodeStartRequest{SessionID: "s"}, invoke: h.wsVscodeStart},
		{name: "stop", action: ws.ActionVscodeStop, body: VscodeStopRequest{SessionID: "s"}, invoke: h.wsVscodeStop},
		{name: "open", action: ws.ActionVscodeOpenFile, body: VscodeOpenFileRequest{SessionID: "s", Path: "a"}, invoke: h.wsVscodeOpenFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := tt.invoke(context.Background(), vscodeMessage(t, tt.action, tt.body))
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			assertWSErrorCode(t, response, ws.ErrorCodeInternalError)
		})
	}
	status, err := h.wsVscodeStatus(context.Background(), vscodeMessage(t, ws.ActionVscodeStatus, VscodeStatusRequest{SessionID: "s"}))
	if err != nil || !strings.Contains(string(status.Payload), `"status":"stopped"`) {
		t.Fatalf("status = (%s, %v)", status.Payload, err)
	}
}

func vscodeHandlerServer(t *testing.T, server *httptest.Server) (*VscodeHandlers, *mockProxyInvalidator) {
	t.Helper()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := agentctl.NewClient(u.Hostname(), port, newTestLogger())
	execution := &lifecycle.AgentExecution{SessionID: "s", WorkspacePath: "/work/my repo"}
	execution.SetAgentCtlClientForTesting(client)
	lookup := &mockExecutionLookup{executions: map[string]*lifecycle.AgentExecution{"s": execution}}
	proxy := &mockProxyInvalidator{}
	return NewVscodeHandlers(lookup, proxy, newTestLogger()), proxy
}

func vscodeMessage(t *testing.T, action string, payload any) *ws.Message {
	t.Helper()
	message, err := ws.NewRequest("id", action, payload)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func TestRegisterVscodeHandlers(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)

	actions := []string{
		ws.ActionVscodeStart,
		ws.ActionVscodeStop,
		ws.ActionVscodeStatus,
		ws.ActionVscodeOpenFile,
	}
	for _, action := range actions {
		if !dispatcher.HasHandler(action) {
			t.Errorf("expected handler for %s to be registered", action)
		}
	}
}

func TestWsVscodeStart_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionVscodeStart,
		Payload: json.RawMessage(`{invalid json`),
	}

	resp, err := h.wsVscodeStart(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeStart_MissingSessionID(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStart, VscodeStartRequest{SessionID: "", Theme: "dark"})

	resp, err := h.wsVscodeStart(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeValidation {
		t.Errorf("expected VALIDATION_ERROR, got %q", errPayload.Code)
	}
}

func TestWsVscodeStart_NoExecution(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	h := NewVscodeHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStart, VscodeStartRequest{SessionID: "nonexistent", Theme: "dark"})

	resp, err := h.wsVscodeStart(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %q", errPayload.Code)
	}
}

func TestWsVscodeStop_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionVscodeStop,
		Payload: json.RawMessage(`{invalid json`),
	}

	resp, err := h.wsVscodeStop(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeStop_MissingSessionID(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStop, VscodeStopRequest{SessionID: ""})

	resp, err := h.wsVscodeStop(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeValidation {
		t.Errorf("expected VALIDATION_ERROR, got %q", errPayload.Code)
	}
}

func TestWsVscodeStop_NoExecution(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	h := NewVscodeHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStop, VscodeStopRequest{SessionID: "nonexistent"})

	resp, err := h.wsVscodeStop(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeStatus_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionVscodeStatus,
		Payload: json.RawMessage(`{invalid json`),
	}

	resp, err := h.wsVscodeStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeStatus_MissingSessionID(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStatus, VscodeStatusRequest{SessionID: ""})

	resp, err := h.wsVscodeStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeStatus_NoExecution_ReturnsStopped(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	h := NewVscodeHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeStatus, VscodeStatusRequest{SessionID: "nonexistent"})

	resp, err := h.wsVscodeStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Status handler returns "stopped" when no execution found (not an error)
	if resp.Type != ws.MessageTypeResponse {
		t.Errorf("expected response type, got %q", resp.Type)
	}
	var payload map[string]any
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if payload["status"] != "stopped" {
		t.Errorf("expected status=stopped, got %v", payload["status"])
	}
}

func TestWsVscodeOpenFile_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionVscodeOpenFile,
		Payload: json.RawMessage(`{invalid json`),
	}

	resp, err := h.wsVscodeOpenFile(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
}

func TestWsVscodeOpenFile_MissingSessionID(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeOpenFile, VscodeOpenFileRequest{
		SessionID: "",
		Path:      "main.go",
	})

	resp, err := h.wsVscodeOpenFile(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeValidation {
		t.Errorf("expected VALIDATION_ERROR, got %q", errPayload.Code)
	}
}

func TestWsVscodeOpenFile_MissingPath(t *testing.T) {
	log := newTestLogger()
	h := NewVscodeHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeOpenFile, VscodeOpenFileRequest{
		SessionID: "session-1",
		Path:      "",
	})

	resp, err := h.wsVscodeOpenFile(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeValidation {
		t.Errorf("expected VALIDATION_ERROR, got %q", errPayload.Code)
	}
}

func TestWsVscodeOpenFile_NoExecution(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	h := NewVscodeHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionVscodeOpenFile, VscodeOpenFileRequest{
		SessionID: "nonexistent",
		Path:      "main.go",
	})

	resp, err := h.wsVscodeOpenFile(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %q", errPayload.Code)
	}
}
