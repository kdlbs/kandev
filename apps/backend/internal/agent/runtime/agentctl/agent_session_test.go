package client

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// captureStreamRequest wires a stream client that records the first request the
// client sends, answers it with `response`, and returns both the client and a
// getter for the captured frame.
func captureStreamRequest(
	t *testing.T,
	response func(msg ws.Message) *ws.Message,
) (*Client, func() ws.Message) {
	t.Helper()

	var mu sync.Mutex
	var seen ws.Message

	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		mu.Lock()
		seen = msg
		mu.Unlock()
		return response(msg)
	})
	// Close the client before the server so the read loop unwinds against a
	// live connection rather than a torn-down listener.
	t.Cleanup(func() {
		c.Close()
		ts.Close()
	})

	return c, func() ws.Message {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
}

func okResponse(payload map[string]any) func(ws.Message) *ws.Message {
	return func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, payload)
		return resp
	}
}

func TestResetSession_UsesResetActionAndReturnsNewSessionID(t *testing.T) {
	modelState := &streams.SessionModelState{
		CurrentModelID: "mock-fast",
		Models:         []streams.SessionModelInfo{{ModelID: "mock-fast"}, {ModelID: "mock-smart"}},
	}
	c, captured := captureStreamRequest(t, okResponse(map[string]any{
		"success": true, "session_id": "sess-reset-1", "model_state": modelState,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mcpServers := []types.McpServer{{Name: "kandev", Command: "kandev-mcp"}}
	sessionID, err := c.ResetSession(ctx, "/workspace/task-1", mcpServers)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if sessionID != "sess-reset-1" {
		t.Errorf("session ID = %q, want sess-reset-1", sessionID)
	}
	state := c.GetLastSessionModelState()
	if state == nil || state.CurrentModelID != "mock-fast" || len(state.Models) != 2 {
		t.Fatalf("model state = %+v, want the synchronous reset catalog", state)
	}

	sent := captured()
	// Reset must NOT reuse agent.session.new — that restarts the subprocess
	// instead of resetting context on the live connection.
	if sent.Action != "agent.session.reset" {
		t.Errorf("action = %q, want agent.session.reset", sent.Action)
	}

	var payload struct {
		Cwd        string            `json:"cwd"`
		McpServers []types.McpServer `json:"mcp_servers"`
	}
	if err := json.Unmarshal(sent.Payload, &payload); err != nil {
		t.Fatalf("decode sent payload: %v", err)
	}
	if payload.Cwd != "/workspace/task-1" {
		t.Errorf("cwd = %q, want /workspace/task-1", payload.Cwd)
	}
	if len(payload.McpServers) != 1 || payload.McpServers[0].Name != "kandev" {
		t.Errorf("mcp_servers = %+v, want the kandev server forwarded", payload.McpServers)
	}
}

func TestResetSession_FailureModes(t *testing.T) {
	t.Run("stream error frame", func(t *testing.T) {
		c, _ := captureStreamRequest(t, func(msg ws.Message) *ws.Message {
			resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "reset unsupported", nil)
			return resp
		})
		_, err := c.ResetSession(context.Background(), "/w", nil)
		if err == nil || !strings.Contains(err.Error(), "agent.session.reset failed: reset unsupported") {
			t.Fatalf("error = %v, want the labelled reset failure", err)
		}
	})

	t.Run("unsuccessful payload", func(t *testing.T) {
		c, _ := captureStreamRequest(t, okResponse(map[string]any{
			"success": false, "error": "no active session",
		}))
		_, err := c.ResetSession(context.Background(), "/w", nil)
		if err == nil || !strings.Contains(err.Error(), "agent.session.reset failed: no active session") {
			t.Fatalf("error = %v, want the labelled reset failure", err)
		}
	})
}

// SetMode / SetModel / SetConfigOption / Authenticate share one shape: send a
// typed payload, treat an error frame as a failure, ignore the response body.
func TestAgentSessionSetters_SendExpectedActionAndPayload(t *testing.T) {
	tests := []struct {
		name        string
		call        func(c *Client) error
		wantAction  string
		wantPayload map[string]any
	}{
		{
			name:        "set mode",
			call:        func(c *Client) error { return c.SetMode(context.Background(), "sess-1", "plan") },
			wantAction:  "agent.session.set_mode",
			wantPayload: map[string]any{"session_id": "sess-1", "mode_id": "plan"},
		},
		{
			name:        "set model",
			call:        func(c *Client) error { return c.SetModel(context.Background(), "claude-opus-5") },
			wantAction:  "agent.session.set_model",
			wantPayload: map[string]any{"model_id": "claude-opus-5"},
		},
		{
			name:        "set config option",
			call:        func(c *Client) error { return c.SetConfigOption(context.Background(), "thinking", "high") },
			wantAction:  "agent.session.set_config_option",
			wantPayload: map[string]any{"config_id": "thinking", "value": "high"},
		},
		{
			name:        "authenticate",
			call:        func(c *Client) error { return c.Authenticate(context.Background(), "oauth") },
			wantAction:  "agent.session.authenticate",
			wantPayload: map[string]any{"method_id": "oauth"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, captured := captureStreamRequest(t, okResponse(map[string]any{"success": true}))

			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}

			sent := captured()
			if sent.Action != tc.wantAction {
				t.Errorf("action = %q, want %q", sent.Action, tc.wantAction)
			}
			if sent.Type != ws.MessageTypeRequest {
				t.Errorf("type = %q, want request", sent.Type)
			}

			var payload map[string]any
			if err := json.Unmarshal(sent.Payload, &payload); err != nil {
				t.Fatalf("decode sent payload: %v", err)
			}
			if len(payload) != len(tc.wantPayload) {
				t.Errorf("payload = %v, want exactly %v", payload, tc.wantPayload)
			}
			for key, want := range tc.wantPayload {
				if payload[key] != want {
					t.Errorf("payload[%q] = %v, want %v", key, payload[key], want)
				}
			}
		})
	}
}

func TestAgentSessionSetters_SurfaceStreamErrorFrames(t *testing.T) {
	tests := []struct {
		name    string
		call    func(c *Client) error
		wantErr string
	}{
		{
			name:    "set mode",
			call:    func(c *Client) error { return c.SetMode(context.Background(), "sess-1", "plan") },
			wantErr: "set mode failed: mode not supported",
		},
		{
			name:    "set model",
			call:    func(c *Client) error { return c.SetModel(context.Background(), "m") },
			wantErr: "set model failed: mode not supported",
		},
		{
			name:    "set config option",
			call:    func(c *Client) error { return c.SetConfigOption(context.Background(), "c", "v") },
			wantErr: "set config option failed: mode not supported",
		},
		{
			name:    "authenticate",
			call:    func(c *Client) error { return c.Authenticate(context.Background(), "oauth") },
			wantErr: "authenticate failed: mode not supported",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := captureStreamRequest(t, func(msg ws.Message) *ws.Message {
				resp, _ := ws.NewError(msg.ID, msg.Action,
					ws.ErrorCodeBadRequest, "mode not supported", nil)
				return resp
			})

			err := tc.call(c)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// The setters deliberately ignore the response payload — only an error frame
// is a failure. An unsuccessful-looking body must not be turned into an error.
func TestAgentSessionSetters_IgnoreResponsePayload(t *testing.T) {
	c, _ := captureStreamRequest(t, okResponse(map[string]any{
		"success": false, "error": "ignored",
	}))

	if err := c.SetMode(context.Background(), "sess-1", "plan"); err != nil {
		t.Errorf("SetMode = %v, want nil — only error frames are failures here", err)
	}
}

func TestAgentSessionSetters_FailWhenStreamIsNotConnected(t *testing.T) {
	c := &Client{
		baseURL:         "http://localhost:0",
		logger:          newTestLogger(),
		pendingRequests: make(map[string]chan *ws.Message),
	}

	calls := map[string]func() error{
		"SetMode":         func() error { return c.SetMode(context.Background(), "s", "m") },
		"SetModel":        func() error { return c.SetModel(context.Background(), "m") },
		"SetConfigOption": func() error { return c.SetConfigOption(context.Background(), "c", "v") },
		"Authenticate":    func() error { return c.Authenticate(context.Background(), "oauth") },
		"ResetSession": func() error {
			_, err := c.ResetSession(context.Background(), "/w", nil)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "agent stream not connected") {
				t.Fatalf("error = %v, want the not-connected sentinel", err)
			}
		})
	}
}

func TestExtractMCPRequestCorrelation(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		wantSession   string
		wantPendingID string
	}{
		{
			name:          "both fields",
			payload:       `{"session_id":"sess-1","pending_id":"pend-1"}`,
			wantSession:   "sess-1",
			wantPendingID: "pend-1",
		},
		{"session only", `{"session_id":"sess-1"}`, "sess-1", ""},
		{"pending only", `{"pending_id":"pend-1"}`, "", "pend-1"},
		{"empty payload", ``, "", ""},
		{"empty object", `{}`, "", ""},
		{"malformed JSON", `{"session_id":`, "", ""},
		{"non-object JSON", `["a"]`, "", ""},
		// Wrong types must not panic on the type assertion.
		{"wrong field types", `{"session_id":42,"pending_id":true}`, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session, pending := extractMCPRequestCorrelation(json.RawMessage(tc.payload))
			if session != tc.wantSession {
				t.Errorf("sessionID = %q, want %q", session, tc.wantSession)
			}
			if pending != tc.wantPendingID {
				t.Errorf("pendingID = %q, want %q", pending, tc.wantPendingID)
			}
		})
	}
}

// stubMCPHandler returns a fixed response/error pair for every dispatch.
type stubMCPHandler struct {
	resp *ws.Message
	err  error

	mu   sync.Mutex
	sawe []string
}

func (h *stubMCPHandler) Dispatch(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	h.mu.Lock()
	h.sawe = append(h.sawe, msg.Action)
	h.mu.Unlock()
	return h.resp, h.err
}

func (h *stubMCPHandler) actions() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.sawe...)
}

func TestDispatchMCPRequest_WritesTheHandlerResponseBack(t *testing.T) {
	resp, err := ws.NewResponse("req-1", "mcp.tools.call", map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	handler := &stubMCPHandler{resp: resp}

	var written [][]byte
	writeMessage := func(data []byte) error {
		written = append(written, data)
		return nil
	}

	c := &Client{logger: newTestLogger()}
	msg := ws.Message{
		ID:      "req-1",
		Type:    ws.MessageTypeRequest,
		Action:  "mcp.tools.call",
		Payload: json.RawMessage(`{"session_id":"sess-1","pending_id":"pend-1"}`),
	}
	c.dispatchMCPRequest(context.Background(), msg, handler, writeMessage)

	if actions := handler.actions(); len(actions) != 1 || actions[0] != "mcp.tools.call" {
		t.Errorf("handler saw %v, want one mcp.tools.call dispatch", actions)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %d frames, want 1", len(written))
	}

	var sent ws.Message
	if err := json.Unmarshal(written[0], &sent); err != nil {
		t.Fatalf("decode written frame: %v", err)
	}
	if sent.ID != "req-1" || sent.Action != "mcp.tools.call" {
		t.Errorf("written frame = %+v, want the request's id and action echoed", sent)
	}
	if sent.Type != ws.MessageTypeResponse {
		t.Errorf("written type = %q, want response", sent.Type)
	}
}

// A handler error must still produce a frame — agentctl's MCP caller is blocked
// waiting on a correlated reply, so swallowing the error would hang the agent.
func TestDispatchMCPRequest_WritesAnErrorFrameOnHandlerFailure(t *testing.T) {
	handler := &stubMCPHandler{err: errUnauthorizedStub{}}

	var written [][]byte
	c := &Client{logger: newTestLogger()}
	c.dispatchMCPRequest(context.Background(),
		ws.Message{ID: "req-2", Type: ws.MessageTypeRequest, Action: "mcp.tools.call"},
		handler,
		func(data []byte) error { written = append(written, data); return nil })

	if len(written) != 1 {
		t.Fatalf("wrote %d frames, want 1 error frame", len(written))
	}
	var sent ws.Message
	if err := json.Unmarshal(written[0], &sent); err != nil {
		t.Fatalf("decode written frame: %v", err)
	}
	if sent.Type != ws.MessageTypeError {
		t.Errorf("type = %q, want error", sent.Type)
	}
	if sent.ID != "req-2" {
		t.Errorf("id = %q, want req-2 so the agent can correlate the failure", sent.ID)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(sent.Payload, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload.Code != ws.ErrorCodeInternalError {
		t.Errorf("code = %q, want %q", payload.Code, ws.ErrorCodeInternalError)
	}
	if payload.Message != "not authorized" {
		t.Errorf("message = %q, want the handler error text", payload.Message)
	}
}

type errUnauthorizedStub struct{}

func (errUnauthorizedStub) Error() string { return "not authorized" }

// A nil response with no error means the handler intentionally answered
// nothing (e.g. a notification); nothing should go on the wire.
func TestDispatchMCPRequest_WritesNothingForANilResponse(t *testing.T) {
	handler := &stubMCPHandler{}

	var written [][]byte
	c := &Client{logger: newTestLogger()}
	c.dispatchMCPRequest(context.Background(),
		ws.Message{ID: "req-3", Type: ws.MessageTypeRequest, Action: "mcp.notify"},
		handler,
		func(data []byte) error { written = append(written, data); return nil })

	if len(written) != 0 {
		t.Errorf("wrote %d frames, want none for a nil handler response", len(written))
	}
}

// A write failure is logged and swallowed — the read loop must keep running.
func TestDispatchMCPRequest_SurvivesAWriteFailure(t *testing.T) {
	resp, _ := ws.NewResponse("req-4", "mcp.tools.call", map[string]any{"ok": true})
	handler := &stubMCPHandler{resp: resp}

	c := &Client{logger: newTestLogger()}
	c.dispatchMCPRequest(context.Background(),
		ws.Message{ID: "req-4", Type: ws.MessageTypeRequest, Action: "mcp.tools.call"},
		handler,
		func([]byte) error { return errUnauthorizedStub{} })
}
