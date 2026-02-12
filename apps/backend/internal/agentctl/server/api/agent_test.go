package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// newTestServer creates a minimal Server with a process.Manager (no adapter).
// Adapter is nil so all handlers that need it return "agent not running".
func newTestServer(t *testing.T) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		Port:    0,
		WorkDir: "/tmp/test",
	}
	procMgr := process.NewManager(cfg, log)
	return NewServer(cfg, procMgr, nil, nil, log)
}

// dialTestWS connects a WebSocket client to the test server's /api/v1/agent/stream endpoint.
func dialTestWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/stream"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial WebSocket: %v", err)
	}
	return conn
}

// sendWSRequest sends a ws.Message request and reads the response.
func sendWSRequest(t *testing.T, conn *websocket.Conn, action string, payload interface{}) *ws.Message {
	t.Helper()
	msg, err := ws.NewRequest("test-req-id", action, payload)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read response with timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var resp ws.Message
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	return &resp
}

// --- Tests for handleAgentStreamRequest dispatcher ---

func TestHandleAgentStreamRequest_UnknownAction(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "unknown.action", nil)
	resp := s.handleAgentStreamRequest(ctx, msg)

	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}

	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error payload: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeUnknownAction {
		t.Errorf("expected UNKNOWN_ACTION code, got %q", errPayload.Code)
	}
	if !strings.Contains(errPayload.Message, "unknown.action") {
		t.Errorf("expected error message to contain action name, got %q", errPayload.Message)
	}
}

func TestHandleAgentStreamRequest_DispatchesCorrectActions(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// All these should dispatch to their handlers (returning "agent not running" since no adapter)
	actions := []string{
		"agent.initialize",
		"agent.session.new",
		"agent.session.load",
		"agent.prompt",
		"agent.cancel",
		"agent.permissions.respond",
		"agent.stderr",
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			msg, _ := ws.NewRequest("req-"+action, action, map[string]string{})
			resp := s.handleAgentStreamRequest(ctx, msg)

			if resp == nil {
				t.Fatal("expected response")
			}
			if resp.ID != "req-"+action {
				t.Errorf("expected response ID 'req-%s', got %q", action, resp.ID)
			}
			// stderr doesn't need an adapter, so it should succeed
			if action == "agent.stderr" {
				if resp.Type != ws.MessageTypeResponse {
					t.Errorf("expected response type for stderr, got %q", resp.Type)
				}
			}
		})
	}
}

// --- Tests for individual WS handlers with no adapter (agent not running) ---

func TestHandleWSInitialize_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.initialize", map[string]string{
		"client_name":    "test",
		"client_version": "1.0.0",
	})
	resp := s.handleWSInitialize(ctx, msg)

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

func TestHandleWSNewSession_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.session.new", map[string]string{
		"cwd": "/workspace",
	})
	resp := s.handleWSNewSession(ctx, msg)

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

func TestHandleWSLoadSession_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.session.load", map[string]string{
		"session_id": "sess-123",
	})
	resp := s.handleWSLoadSession(ctx, msg)

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

func TestHandleWSLoadSession_MissingSessionID(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.session.load", map[string]string{})
	resp := s.handleWSLoadSession(ctx, msg)

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
	if !strings.Contains(errPayload.Message, "session_id is required") {
		t.Errorf("expected 'session_id is required', got %q", errPayload.Message)
	}
}

func TestHandleWSPrompt_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.prompt", map[string]string{
		"text": "hello",
	})
	resp := s.handleWSPrompt(ctx, msg)

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

func TestHandleWSCancel_NoAdapter(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.cancel", nil)
	resp := s.handleWSCancel(ctx, msg)

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

func TestHandleWSStderr_Empty(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	msg, _ := ws.NewRequest("req-1", "agent.stderr", nil)
	resp := s.handleWSStderr(ctx, msg)

	if resp.Type != ws.MessageTypeResponse {
		t.Errorf("expected response type, got %q", resp.Type)
	}

	var result AgentStderrResponse
	if err := resp.ParsePayload(&result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(result.Lines) != 0 {
		t.Errorf("expected empty lines, got %d", len(result.Lines))
	}
}

func TestHandleWSPermissionRespond_BadPayload(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// pending_id is required but since it's not gin binding, ParsePayload will succeed
	// with empty values. The actual error comes from RespondToPermission not finding it.
	msg, _ := ws.NewRequest("req-1", "agent.permissions.respond", map[string]string{
		"pending_id": "nonexistent",
		"option_id":  "allow",
	})
	resp := s.handleWSPermissionRespond(ctx, msg)

	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %q", errPayload.Code)
	}
}

func TestHandleWSInitialize_BadPayload(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// Send invalid payload (non-JSON raw message)
	msg := &ws.Message{
		ID:      "req-1",
		Type:    ws.MessageTypeRequest,
		Action:  "agent.initialize",
		Payload: json.RawMessage(`invalid json`),
	}
	resp := s.handleWSInitialize(ctx, msg)

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

// --- WebSocket integration tests ---

func TestAgentStreamWS_RequestResponseFlow(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	// Send an stderr request (doesn't need adapter)
	resp := sendWSRequest(t, conn, "agent.stderr", nil)
	if resp.Type != ws.MessageTypeResponse {
		t.Errorf("expected response type, got %q", resp.Type)
	}
	if resp.ID != "test-req-id" {
		t.Errorf("expected response ID 'test-req-id', got %q", resp.ID)
	}
}

func TestAgentStreamWS_UnknownActionReturnsError(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	resp := sendWSRequest(t, conn, "nonexistent.action", nil)
	if resp.Type != ws.MessageTypeError {
		t.Errorf("expected error type, got %q", resp.Type)
	}
	var errPayload ws.ErrorPayload
	if err := resp.ParsePayload(&errPayload); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errPayload.Code != ws.ErrorCodeUnknownAction {
		t.Errorf("expected UNKNOWN_ACTION, got %q", errPayload.Code)
	}
}

func TestAgentStreamWS_AgentNotRunningError(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	resp := sendWSRequest(t, conn, "agent.initialize", map[string]string{
		"client_name":    "test",
		"client_version": "1.0.0",
	})
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

func TestAgentStreamWS_MultipleRequests(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	// Send multiple requests sequentially
	actions := []string{"agent.stderr", "agent.stderr", "agent.cancel"}
	for i, action := range actions {
		msg, _ := ws.NewRequest("req-"+string(rune('a'+i)), action, nil)
		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatalf("failed to write request %d: %v", i, err)
		}
	}

	// Read all responses
	for i := 0; i < len(actions); i++ {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, respData, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read response %d: %v", i, err)
		}
		var resp ws.Message
		if err := json.Unmarshal(respData, &resp); err != nil {
			t.Fatalf("failed to parse response %d: %v", i, err)
		}
		// All should have some response (either success or error)
		if resp.Type != ws.MessageTypeResponse && resp.Type != ws.MessageTypeError {
			t.Errorf("response %d: expected response or error type, got %q", i, resp.Type)
		}
	}
}

func TestAgentStreamWS_ResponsePreservesMessageID(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	requestID := "unique-req-id-12345"
	msg, _ := ws.NewRequest(requestID, "agent.stderr", nil)
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	var resp ws.Message
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if resp.ID != requestID {
		t.Errorf("expected response ID %q, got %q", requestID, resp.ID)
	}
	if resp.Action != "agent.stderr" {
		t.Errorf("expected action 'agent.stderr', got %q", resp.Action)
	}
}

func TestAgentStreamWS_MalformedMessage(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialTestWS(t, server)
	defer conn.Close()

	// Send malformed JSON - should not crash the server
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`not json`)); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Server should still be alive - send a valid request
	time.Sleep(50 * time.Millisecond)
	resp := sendWSRequest(t, conn, "agent.stderr", nil)
	if resp.Type != ws.MessageTypeResponse {
		t.Errorf("expected response after malformed message, got %q", resp.Type)
	}
}
