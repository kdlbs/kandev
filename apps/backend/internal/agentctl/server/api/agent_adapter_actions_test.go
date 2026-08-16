package api

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// bareAgentAdapter satisfies adapter.AgentAdapter and nothing else, so it
// stands in for a transport that supports none of the optional session
// controls (set_mode, set_model, authenticate, reset).
type bareAgentAdapter struct {
	sessionID string
}

func (*bareAgentAdapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (*bareAgentAdapter) PrepareCommandArgs() []string                   { return nil }
func (*bareAgentAdapter) Connect(io.Writer, io.Reader) error             { return nil }
func (*bareAgentAdapter) Initialize(context.Context) error               { return nil }
func (*bareAgentAdapter) GetAgentInfo() *adapter.AgentInfo               { return nil }
func (a *bareAgentAdapter) NewSession(context.Context, []types.McpServer) (string, error) {
	return a.sessionID, nil
}

func (a *bareAgentAdapter) LoadSession(_ context.Context, sessionID string, _ []types.McpServer) error {
	a.sessionID = sessionID
	return nil
}

func (*bareAgentAdapter) Prompt(context.Context, string, []v1.MessageAttachment, uint64) error {
	return nil
}
func (*bareAgentAdapter) Cancel(context.Context) error                   { return nil }
func (*bareAgentAdapter) Updates() <-chan adapter.AgentEvent             { return nil }
func (a *bareAgentAdapter) GetSessionID() string                         { return a.sessionID }
func (*bareAgentAdapter) GetOperationID() string                         { return "" }
func (*bareAgentAdapter) SetPermissionHandler(adapter.PermissionHandler) {}
func (*bareAgentAdapter) Close() error                                   { return nil }
func (*bareAgentAdapter) RequiresProcessKill() bool                      { return false }

// capableAgentAdapter additionally implements every optional session-control
// interface and records exactly what it was handed, so the tests can prove the
// payload field reached the adapter rather than a zero value.
type capableAgentAdapter struct {
	bareAgentAdapter
	modeID       string
	modelID      string
	methodID     string
	resetServers []types.McpServer
	resetErr     error
	failWith     error
	modelState   *streams.SessionModelState
}

func (a *capableAgentAdapter) SetMode(_ context.Context, modeID string) error {
	a.modeID = modeID
	return a.failWith
}

func (a *capableAgentAdapter) SetModel(_ context.Context, modelID string) error {
	a.modelID = modelID
	return a.failWith
}

func (a *capableAgentAdapter) Authenticate(_ context.Context, methodID string) error {
	a.methodID = methodID
	return a.failWith
}

func (a *capableAgentAdapter) GetSessionModelState() *streams.SessionModelState {
	return a.modelState
}

func (a *capableAgentAdapter) ResetSession(_ context.Context, servers []types.McpServer) (string, error) {
	a.resetServers = servers
	if a.resetErr != nil {
		return "", a.resetErr
	}
	return "session-after-reset", nil
}

func errorPayload(t *testing.T, msg *ws.Message) ws.ErrorPayload {
	t.Helper()
	if msg.Type != ws.MessageTypeError {
		t.Fatalf("message type = %q, want %q (payload %s)", msg.Type, ws.MessageTypeError, string(msg.Payload))
	}
	var payload ws.ErrorPayload
	if err := msg.ParsePayload(&payload); err != nil {
		t.Fatalf("parse error payload: %v", err)
	}
	return payload
}

func assertAdapterActionSuccess(t *testing.T, msg *ws.Message) {
	t.Helper()
	if msg.Type == ws.MessageTypeError {
		t.Fatalf("action failed: %s", string(msg.Payload))
	}
	var payload map[string]any
	if err := msg.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response payload: %v", err)
	}
	if payload["success"] != true {
		t.Errorf("payload = %v, want success:true", payload)
	}
}

// adapterActionCase describes one optional session-control handler. Running
// them through a single table is what keeps the three failure shapes
// (no adapter / unsupported adapter / adapter error) identical across
// handlers — the frontend renders them with shared code.
type adapterActionCase struct {
	name    string
	action  string
	payload any
	invoke  func(s *Server, msg *ws.Message) *ws.Message
	// unsupported is the message a transport lacking the optional interface
	// must produce.
	unsupported string
	// assertReceived checks the value the adapter was handed.
	assertReceived func(t *testing.T, a *capableAgentAdapter)
}

func adapterActionCases() []adapterActionCase {
	return []adapterActionCase{
		{
			name:        "set_mode",
			action:      "agent.session.set_mode",
			payload:     map[string]string{"session_id": "s1", "mode_id": "plan"},
			invoke:      func(s *Server, msg *ws.Message) *ws.Message { return s.handleWSSetMode(context.Background(), msg) },
			unsupported: "agent does not support mode switching",
			assertReceived: func(t *testing.T, a *capableAgentAdapter) {
				if a.modeID != "plan" {
					t.Errorf("mode_id = %q, want %q", a.modeID, "plan")
				}
			},
		},
		{
			name:        "set_model",
			action:      "agent.session.set_model",
			payload:     map[string]string{"model_id": "claude-opus-5"},
			invoke:      func(s *Server, msg *ws.Message) *ws.Message { return s.handleWSSetModel(context.Background(), msg) },
			unsupported: "agent does not support model switching",
			assertReceived: func(t *testing.T, a *capableAgentAdapter) {
				if a.modelID != "claude-opus-5" {
					t.Errorf("model_id = %q, want %q", a.modelID, "claude-opus-5")
				}
			},
		},
		{
			name:        "authenticate",
			action:      "agent.authenticate",
			payload:     map[string]string{"method_id": "oauth"},
			invoke:      func(s *Server, msg *ws.Message) *ws.Message { return s.handleWSAuthenticate(context.Background(), msg) },
			unsupported: "agent does not support authenticate",
			assertReceived: func(t *testing.T, a *capableAgentAdapter) {
				if a.methodID != "oauth" {
					t.Errorf("method_id = %q, want %q", a.methodID, "oauth")
				}
			},
		},
	}
}

// TestAdapterActions_ForwardPayloadToAdapter covers the success branch of
// adapterAction for every optional session control, and asserts the specific
// payload field reached the adapter.
func TestAdapterActions_ForwardPayloadToAdapter(t *testing.T) {
	for _, tc := range adapterActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			agentAdapter := &capableAgentAdapter{}
			s.procMgr.SetAdapterForTest(agentAdapter)

			msg, err := ws.NewRequest("req-1", tc.action, tc.payload)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			assertAdapterActionSuccess(t, tc.invoke(s, msg))
			tc.assertReceived(t, agentAdapter)
		})
	}
}

// TestAdapterActions_RejectWhenAgentNotRunning covers the nil-adapter branch,
// which is what a click on a stale UI produces after the agent exits.
func TestAdapterActions_RejectWhenAgentNotRunning(t *testing.T) {
	for _, tc := range adapterActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)

			msg, err := ws.NewRequest("req-1", tc.action, tc.payload)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			payload := errorPayload(t, tc.invoke(s, msg))
			if payload.Code != ws.ErrorCodeInternalError {
				t.Errorf("code = %q, want %q", payload.Code, ws.ErrorCodeInternalError)
			}
			if payload.Message != "agent not running" {
				t.Errorf("message = %q, want %q", payload.Message, "agent not running")
			}
		})
	}
}

// TestAdapterActions_RejectUnsupportedTransport covers the interface assertion.
// A transport that never implemented the control must say so by name rather
// than reporting a generic failure.
func TestAdapterActions_RejectUnsupportedTransport(t *testing.T) {
	for _, tc := range adapterActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.procMgr.SetAdapterForTest(&bareAgentAdapter{})

			msg, err := ws.NewRequest("req-1", tc.action, tc.payload)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if got := errorPayload(t, tc.invoke(s, msg)).Message; got != tc.unsupported {
				t.Errorf("message = %q, want %q", got, tc.unsupported)
			}
		})
	}
}

// TestAdapterActions_SurfaceAdapterError covers the last branch: the adapter
// accepted the call and failed, and its reason has to reach the caller intact.
func TestAdapterActions_SurfaceAdapterError(t *testing.T) {
	for _, tc := range adapterActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.procMgr.SetAdapterForTest(&capableAgentAdapter{failWith: errors.New("upstream refused")})

			msg, err := ws.NewRequest("req-1", tc.action, tc.payload)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if got := errorPayload(t, tc.invoke(s, msg)).Message; got != "upstream refused" {
				t.Errorf("message = %q, want %q", got, "upstream refused")
			}
		})
	}
}

// TestAdapterActions_RejectMalformedPayload covers the parse guard, which runs
// before the adapter is even looked up.
func TestAdapterActions_RejectMalformedPayload(t *testing.T) {
	for _, tc := range adapterActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.procMgr.SetAdapterForTest(&capableAgentAdapter{})

			msg, err := ws.NewRequest("req-1", tc.action, "not-an-object")
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			payload := errorPayload(t, tc.invoke(s, msg))
			if payload.Code != ws.ErrorCodeBadRequest {
				t.Errorf("code = %q, want %q", payload.Code, ws.ErrorCodeBadRequest)
			}
			if !strings.HasPrefix(payload.Message, "invalid request: ") {
				t.Errorf("message = %q, want an %q prefix", payload.Message, "invalid request: ")
			}
		})
	}
}

// TestHandleWSResetSession_ReturnsNewSessionID covers the happy path of the
// context-reset control. The new session ID is what the backend persists as
// ACPSessionID, so returning the old one would silently break resume.
func TestHandleWSResetSession_ReturnsNewSessionID(t *testing.T) {
	s := newTestServer(t)
	agentAdapter := &capableAgentAdapter{}
	agentAdapter.sessionID = "session-before-reset"
	agentAdapter.modelState = &streams.SessionModelState{
		CurrentModelID: "mock-fast",
		Models:         []streams.SessionModelInfo{{ModelID: "mock-fast"}, {ModelID: "mock-smart"}},
	}
	s.procMgr.SetAdapterForTest(agentAdapter)

	msg, err := ws.NewRequest("req-1", "agent.session.reset", NewSessionRequest{
		McpServers: []types.McpServer{{Name: "external", URL: "https://example.invalid/mcp"}},
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp := s.handleWSResetSession(context.Background(), msg)
	if resp.Type == ws.MessageTypeError {
		t.Fatalf("reset failed: %s", string(resp.Payload))
	}
	var payload NewSessionResponse
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse reset payload: %v", err)
	}
	if !payload.Success || payload.SessionID != "session-after-reset" {
		t.Errorf("payload = %+v, want the adapter's new session id", payload)
	}
	if payload.ModelState == nil || payload.ModelState.CurrentModelID != "mock-fast" || len(payload.ModelState.Models) != 2 {
		t.Errorf("model state = %+v, want the adapter's synchronous catalog", payload.ModelState)
	}
	if len(agentAdapter.resetServers) != 1 || agentAdapter.resetServers[0].Name != "external" {
		t.Errorf("adapter received %+v, want the caller's MCP servers", agentAdapter.resetServers)
	}
}

// TestHandleWSResetSession_Rejections covers all three refusal branches of the
// reset handler, which does not go through adapterAction and so needs its own
// coverage of each shape.
func TestHandleWSResetSession_Rejections(t *testing.T) {
	cases := []struct {
		name        string
		payload     any
		adapter     adapter.AgentAdapter
		wantCode    string
		wantMessage string
	}{
		{"malformed payload", "not-an-object", &capableAgentAdapter{}, ws.ErrorCodeBadRequest, ""},
		{"agent not running", NewSessionRequest{}, nil, ws.ErrorCodeInternalError, "agent not running"},
		{
			"unsupported transport",
			NewSessionRequest{},
			&bareAgentAdapter{},
			ws.ErrorCodeInternalError,
			"agent does not support session reset",
		},
		{
			"adapter failure",
			NewSessionRequest{},
			&capableAgentAdapter{resetErr: errors.New("reset refused")},
			ws.ErrorCodeInternalError,
			"reset refused",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			if tc.adapter != nil {
				s.procMgr.SetAdapterForTest(tc.adapter)
			}

			msg, err := ws.NewRequest("req-1", "agent.session.reset", tc.payload)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			payload := errorPayload(t, s.handleWSResetSession(context.Background(), msg))
			if payload.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", payload.Code, tc.wantCode)
			}
			if tc.wantMessage != "" && payload.Message != tc.wantMessage {
				t.Errorf("message = %q, want %q", payload.Message, tc.wantMessage)
			}
		})
	}
}
