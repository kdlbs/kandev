package handlers

import (
	"context"
	"encoding/json"
	"testing"

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
