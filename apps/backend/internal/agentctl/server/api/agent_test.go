package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

func TestHandleAgentConfigure_PrefersPresentStructuredArgs(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	procMgr := process.NewManager(cfg, log)
	server := NewServer(cfg, procMgr, nil, nil, log)

	body := strings.NewReader(`{"command":"legacy command","agent_args":["runner","two words","","C:\\tools\\agent.exe"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("configure status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	want := []string{"runner", "two words", "", `C:\tools\agent.exe`}
	if got := cfg.AgentArgs; !slices.Equal(got, want) {
		t.Fatalf("AgentArgs = %#v, want %#v", got, want)
	}
}

func TestHandleAgentConfigure_UsesLegacyOnlyWhenArgsAbsent(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	for _, tc := range []struct {
		name string
		body string
		code int
		want []string
	}{
		{name: "legacy absent", body: `{"command":"legacy  --flag"}`, code: http.StatusOK, want: []string{"legacy", "--flag"}},
		{name: "present empty rejects", body: `{"command":"legacy --flag","agent_args":[]}`, code: http.StatusInternalServerError},
		{name: "present flag executable rejects", body: `{"command":"legacy --flag","agent_args":["--flag"]}`, code: http.StatusInternalServerError},
		{name: "present null rejects", body: `{"command":"legacy --flag","agent_args":null}`, code: http.StatusInternalServerError},
		{name: "present empty continue rejects", body: `{"command":"runner","agent_args":["runner"],"continue_command":"legacy continue","continue_args":[]}`, code: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			server.router.ServeHTTP(resp, req)
			if resp.Code != tc.code {
				t.Fatalf("configure status = %d, want %d: %s", resp.Code, tc.code, resp.Body.String())
			}
			if tc.want != nil && !slices.Equal(cfg.AgentArgs, tc.want) {
				t.Fatalf("AgentArgs = %#v, want %#v", cfg.AgentArgs, tc.want)
			}
		})
	}
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
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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
	} else if resp.Type != ws.MessageTypeError {
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
			} else if resp.ID != "req-"+action {
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

func TestHandleWSPrompt_SuppressesPromptAbandonedAfterCancel(t *testing.T) {
	s := newTestServer(t)
	prompted := make(chan uint64, 1)
	s.procMgr.SetAdapterForTest(&promptErrorAdapter{
		sessionID: "session-123",
		err:       errors.New("prompt failed: prompt abandoned after cancel"),
		prompted:  prompted,
	})

	msg, _ := ws.NewRequest("req-1", "agent.prompt", PromptRequest{
		Text:             "hello",
		PromptGeneration: 42,
	})
	resp := s.handleWSPrompt(context.Background(), msg)

	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response type, got %q", resp.Type)
	}
	var result PromptResponse
	if err := resp.ParsePayload(&result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !result.Success {
		t.Fatal("expected prompt response success")
	}

	select {
	case generation := <-prompted:
		if generation != 42 {
			t.Fatalf("prompt generation = %d, want 42", generation)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt was not called")
	}

	select {
	case event := <-s.procMgr.GetUpdates():
		t.Fatalf("expected no error event for prompt abandoned after cancel, got %+v", event)
	case <-time.After(100 * time.Millisecond):
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

type promptErrorAdapter struct {
	sessionID string
	err       error
	prompted  chan<- uint64
}

type mcpCaptureAdapter struct {
	promptErrorAdapter
	newSessionServers  []types.McpServer
	loadSessionServers []types.McpServer
}

func (a *mcpCaptureAdapter) NewSession(_ context.Context, servers []types.McpServer) (string, error) {
	a.newSessionServers = append([]types.McpServer(nil), servers...)
	return "new-session", nil
}

func (a *mcpCaptureAdapter) LoadSession(_ context.Context, sessionID string, servers []types.McpServer) error {
	a.sessionID = sessionID
	a.loadSessionServers = append([]types.McpServer(nil), servers...)
	return nil
}

func (a *promptErrorAdapter) PrepareEnvironment() (map[string]string, error) {
	return nil, nil
}

func (a *promptErrorAdapter) PrepareCommandArgs() []string {
	return nil
}

func (a *promptErrorAdapter) Connect(_ io.Writer, _ io.Reader) error {
	return nil
}

func (a *promptErrorAdapter) Initialize(_ context.Context) error {
	return nil
}

func (a *promptErrorAdapter) GetAgentInfo() *adapter.AgentInfo {
	return nil
}

func (a *promptErrorAdapter) NewSession(_ context.Context, _ []types.McpServer) (string, error) {
	return a.sessionID, nil
}

func (a *promptErrorAdapter) LoadSession(_ context.Context, sessionID string, _ []types.McpServer) error {
	a.sessionID = sessionID
	return nil
}

func (a *promptErrorAdapter) Prompt(
	_ context.Context,
	_ string,
	_ []v1.MessageAttachment,
	promptGeneration uint64,
) error {
	a.prompted <- promptGeneration
	return a.err
}

func (a *promptErrorAdapter) Cancel(_ context.Context) error {
	return nil
}

func (a *promptErrorAdapter) Updates() <-chan adapter.AgentEvent {
	return nil
}

func (a *promptErrorAdapter) GetSessionID() string {
	return a.sessionID
}

func (a *promptErrorAdapter) GetOperationID() string {
	return ""
}

func (a *promptErrorAdapter) SetPermissionHandler(_ adapter.PermissionHandler) {
}

func (a *promptErrorAdapter) Close() error {
	return nil
}

func (a *promptErrorAdapter) RequiresProcessKill() bool {
	return false
}

func assertLocalKandevMCPServers(t *testing.T, got []types.McpServer) {
	t.Helper()
	if len(got) < 2 {
		t.Fatalf("MCP servers = %+v, want local HTTP and SSE entries", got)
	}
	if got[0].Name != kandevMcpServerName || got[0].Type != mcpTransportHTTP || got[0].URL != "http://localhost:0/mcp" {
		t.Errorf("first MCP server = %+v, want local kandev HTTP server", got[0])
	}
	if got[1].Name != kandevMcpServerName || got[1].Type != mcpTransportSSE || got[1].URL != "http://localhost:0/sse" {
		t.Errorf("second MCP server = %+v, want local kandev SSE fallback", got[1])
	}
}

func TestHandleWSNewSession_InjectsLocalKandevMCPServers(t *testing.T) {
	s := newTestServerWithMCP(t)
	capture := &mcpCaptureAdapter{}
	s.procMgr.SetAdapterForTest(capture)

	msg, err := ws.NewRequest("req-new", "agent.session.new", NewSessionRequest{})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp := s.handleWSNewSession(context.Background(), msg)
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want %q", resp.Type, ws.MessageTypeResponse)
	}
	assertLocalKandevMCPServers(t, capture.newSessionServers)
}

func TestHandleWSLoadSession_InjectsLocalKandevMCPServers(t *testing.T) {
	s := newTestServerWithMCP(t)
	capture := &mcpCaptureAdapter{}
	s.procMgr.SetAdapterForTest(capture)

	msg, err := ws.NewRequest("req-load", "agent.session.load", LoadSessionRequest{SessionID: "existing-session"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp := s.handleWSLoadSession(context.Background(), msg)
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want %q", resp.Type, ws.MessageTypeResponse)
	}
	assertLocalKandevMCPServers(t, capture.loadSessionServers)
}

func TestMCPToolCatalogRemainsAvailableAfterAgentSessionLoad(t *testing.T) {
	log := newTestLogger()
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPStepComplete, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{"accepted": true})
	})
	backend := mcpserver.NewDispatcherBackendClient(dispatcher, log)
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	procMgr := process.NewManager(cfg, log)
	mcpServer := mcpserver.New(backend, "session-1", "task-1", cfg.Port, log, "", false, mcpserver.ModeTask)
	s := NewServer(cfg, procMgr, mcpServer, nil, log)
	capture := &mcpCaptureAdapter{}
	procMgr.SetAdapterForTest(capture)

	assertStepCompleteAvailableAndCallable(t, s)

	msg, err := ws.NewRequest("req-load", "agent.session.load", LoadSessionRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if resp := s.handleWSLoadSession(context.Background(), msg); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("load response type = %q, want %q", resp.Type, ws.MessageTypeResponse)
	}

	// A second MCP client session proves the server catalog still serves the
	// task-only completion tool after the agent session load path runs.
	assertStepCompleteAvailableAndCallable(t, s)
}

func assertStepCompleteAvailableAndCallable(t *testing.T, s *Server) {
	t.Helper()
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	init := postMCPRequest(t, s, initBody, "")
	if init.code != http.StatusOK || init.sessionID == "" {
		t.Fatalf("initialize status = %d, session = %q, body = %s", init.code, init.sessionID, init.body)
	}

	listed := postMCPRequest(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, init.sessionID)
	if listed.code != http.StatusOK || !strings.Contains(listed.body, `"name":"step_complete_kandev"`) {
		t.Fatalf("tools/list status = %d, body = %s", listed.code, listed.body)
	}

	called := postMCPRequest(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"step_complete_kandev","arguments":{"summary":"done"}}}`, init.sessionID)
	if called.code != http.StatusOK || !strings.Contains(called.body, `accepted`) || strings.Contains(called.body, `"isError":true`) {
		t.Fatalf("tools/call status = %d, body = %s", called.code, called.body)
	}
}

type mcpHTTPResponse struct {
	code      int
	body      string
	sessionID string
}

func postMCPRequest(t *testing.T, s *Server, body, sessionID string) mcpHTTPResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	s.router.ServeHTTP(recorder, req)
	return mcpHTTPResponse{
		code:      recorder.Code,
		body:      recorder.Body.String(),
		sessionID: recorder.Header().Get("Mcp-Session-Id"),
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
	defer func() { _ = conn.Close() }()

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
	defer func() { _ = conn.Close() }()

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
	defer func() { _ = conn.Close() }()

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
	defer func() { _ = conn.Close() }()

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
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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
	defer func() { _ = conn.Close() }()

	requestID := "unique-req-id-12345"
	msg, _ := ws.NewRequest(requestID, "agent.stderr", nil)
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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
	defer func() { _ = conn.Close() }()

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

// --- injectKandevMcpServers ordering ---

// TestInjectKandevMcpServers_HttpFirst ensures the kandev MCP HTTP entry is injected
// before the SSE entry so that the capability-filter dedup (first surviving entry per
// name wins) prefers the HTTP transport whenever an agent advertises both
// mcpCapabilities.http and mcpCapabilities.sse. SSE-only agents still pick up the SSE
// fallback.
func TestInjectKandevMcpServers_HttpFirst(t *testing.T) {
	srv := newTestServer(t)

	got := srv.injectKandevMcpServers(nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 kandev entries, got %d: %+v", len(got), got)
	}
	if got[0].Name != kandevMcpServerName || got[0].Type != mcpTransportHTTP {
		t.Errorf("expected first entry name=%q type=%q, got name=%q type=%q",
			kandevMcpServerName, mcpTransportHTTP, got[0].Name, got[0].Type)
	}
	if got[1].Name != kandevMcpServerName || got[1].Type != mcpTransportSSE {
		t.Errorf("expected second entry name=%q type=%q, got name=%q type=%q",
			kandevMcpServerName, mcpTransportSSE, got[1].Name, got[1].Type)
	}

	// Upstream non-kandev entries must be preserved and appended after the injected pair;
	// any upstream "kandev" must be filtered out.
	upstream := []types.McpServer{
		{Name: "other", Type: "stdio", Command: "x"},
		{Name: kandevMcpServerName, Type: mcpTransportSSE, URL: "http://stale/sse"},
	}
	got = srv.injectKandevMcpServers(upstream)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (http+sse+other), got %d: %+v", len(got), got)
	}
	if got[0].Type != mcpTransportHTTP || got[1].Type != mcpTransportSSE {
		t.Errorf("expected injected order http,sse; got %q,%q", got[0].Type, got[1].Type)
	}
	if got[2].Name != "other" || got[2].Command != "x" {
		t.Errorf("expected upstream 'other' entry last, got %+v", got[2])
	}
}
