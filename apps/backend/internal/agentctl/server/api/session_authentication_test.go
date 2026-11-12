package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	acptransport "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type sessionAuthenticationAdapter struct {
	adapter.AgentAdapter
	methods []streams.AuthMethodInfo
	err     error
}

func (a *sessionAuthenticationAdapter) GetSessionOperationAuthMethods() []streams.AuthMethodInfo {
	return a.methods
}

func (a *sessionAuthenticationAdapter) NewSession(context.Context, []types.McpServer) (string, error) {
	return "", a.err
}

func (a *sessionAuthenticationAdapter) LoadSession(context.Context, string, []types.McpServer) error {
	return a.err
}

func (a *sessionAuthenticationAdapter) SupportsSessionResume() bool { return true }
func (a *sessionAuthenticationAdapter) SupportsSessionLoad() bool   { return true }

func (a *sessionAuthenticationAdapter) ResumeSession(context.Context, string, []types.McpServer) error {
	return a.err
}

func (a *sessionAuthenticationAdapter) ResetSession(context.Context, []types.McpServer) (string, error) {
	return "", a.err
}

func TestSessionAuthenticationProjectionPreservesNativeRestoreResponse(t *testing.T) {
	server := newTestServer(t)
	server.procMgr.SetAdapterForTest(&sessionAuthenticationAdapter{
		err: &acptransport.SessionRestoreError{
			Reason: acptransport.SessionRestoreReasonNativeStateMissing,
			Cause:  &acpsdk.RequestError{Code: -32000, Message: "authentication required"},
		},
	})
	request, err := ws.NewRequest("restore-request", "agent.session.load", LoadSessionRequest{SessionID: "native-session"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response := server.handleWSLoadSession(context.Background(), request)
	var payload ws.ErrorPayload
	if err := response.ParsePayload(&payload); err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if payload.Code != "SESSION_RESTORE_REQUIRED" {
		t.Fatalf("error code = %q, want SESSION_RESTORE_REQUIRED", payload.Code)
	}
	if payload.Details["kind"] != "session_restore_required" || payload.Details["recovery_action"] != "continue_from_history" {
		t.Fatalf("restore details = %#v", payload.Details)
	}
	if _, ok := payload.Details["session_failure"]; ok {
		t.Fatalf("native restore outcome was replaced by auth projection: %#v", payload.Details)
	}
}

func TestSessionAuthenticationFailureProjection(t *testing.T) {
	methods := []streams.AuthMethodInfo{{
		ID:          "cursor_login",
		Name:        "Cursor CLI login",
		Description: "Sign in to Cursor",
		Meta:        map[string]any{"token": "never-project-this-secret"},
		TerminalAuth: &streams.TerminalAuth{
			Command: "cursor-agent",
			Args:    []string{"login"},
			Label:   "Sign in",
		},
	}}
	server := newTestServer(t)
	server.procMgr.SetAdapterForTest(&sessionAuthenticationAdapter{
		methods: methods,
		err:     fmt.Errorf("provider rejected request: %w", &acpsdk.RequestError{Code: -32000, Message: "authentication required"}),
	})

	operations := []struct {
		name   string
		action string
		call   func(context.Context, *ws.Message) *ws.Message
	}{
		{name: "new", action: "agent.session.new", call: server.handleWSNewSession},
		{name: "load", action: "agent.session.load", call: server.handleWSLoadSession},
		{name: "resume", action: "agent.session.resume", call: server.handleWSResumeSession},
		{name: "reset", action: "agent.session.reset", call: server.handleWSResetSession},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			var payload any = NewSessionRequest{}
			switch operation.name {
			case "load":
				payload = LoadSessionRequest{SessionID: "native-session"}
			case "resume":
				payload = ResumeSessionRequest{SessionID: "native-session"}
			}
			msg, err := ws.NewRequest("auth-request", operation.action, payload)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			response := operation.call(context.Background(), msg)
			if response.Type != ws.MessageTypeError {
				t.Fatalf("response type = %q, want error", response.Type)
			}
			var errorPayload ws.ErrorPayload
			if err := response.ParsePayload(&errorPayload); err != nil {
				t.Fatalf("ParsePayload: %v", err)
			}
			raw, ok := errorPayload.Details["session_failure"]
			if !ok {
				t.Fatalf("details = %#v, want typed session_failure", errorPayload.Details)
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("marshal session_failure: %v", err)
			}
			var projected struct {
				Code        string                   `json:"code"`
				AuthMethods []streams.AuthMethodInfo `json:"auth_methods"`
			}
			if err := json.Unmarshal(encoded, &projected); err != nil {
				t.Fatalf("unmarshal session_failure: %v", err)
			}
			if projected.Code != "authentication_required" {
				t.Errorf("session failure code = %q, want authentication_required", projected.Code)
			}
			for {
				select {
				case event := <-server.procMgr.GetUpdates():
					if event.Type == streams.EventTypeMCPAttachment {
						t.Fatalf("authentication rejection published MCP attachment evidence: %+v", event)
					}
				default:
					goto updatesDrained
				}
			}
		updatesDrained:
			want := []streams.AuthMethodInfo{{
				ID:          "cursor_login",
				Name:        "Cursor CLI login",
				Description: "Sign in to Cursor",
				TerminalAuth: &streams.TerminalAuth{
					Command: "cursor-agent",
					Args:    []string{"login"},
					Label:   "Sign in",
				},
			}}
			if !reflect.DeepEqual(projected.AuthMethods, want) {
				t.Errorf("auth methods = %#v, want %#v", projected.AuthMethods, want)
			}
			if strings.Contains(string(encoded), "never-project-this-secret") {
				t.Errorf("session failure projection leaked raw method metadata: %s", encoded)
			}
		})
	}
}

func TestSessionAuthenticationFailureDoesNotExposeACPErrorData(t *testing.T) {
	const secret = "credential-bearing-acp-data"
	operations := []struct {
		name   string
		action string
		call   func(*Server, context.Context, *ws.Message) *ws.Message
	}{
		{name: "new", action: "agent.session.new", call: (*Server).handleWSNewSession},
		{name: "load", action: "agent.session.load", call: (*Server).handleWSLoadSession},
		{name: "resume", action: "agent.session.resume", call: (*Server).handleWSResumeSession},
		{name: "reset", action: "agent.session.reset", call: (*Server).handleWSResetSession},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			server := newTestServer(t)
			core, observed := observer.New(zapcore.ErrorLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create observed logger: %v", err)
			}
			server.logger = log
			server.procMgr.SetAdapterForTest(&sessionAuthenticationAdapter{
				err: fmt.Errorf("provider rejected request: %w", &acpsdk.RequestError{
					Code: -32000, Message: "authentication required", Data: secret,
				}),
			})
			var payload any = NewSessionRequest{}
			switch operation.name {
			case "load":
				payload = LoadSessionRequest{SessionID: "native-session"}
			case "resume":
				payload = ResumeSessionRequest{SessionID: "native-session"}
			}
			request, err := ws.NewRequest("auth-data-request", operation.action, payload)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			response := operation.call(server, context.Background(), request)
			var responseText strings.Builder
			if err := json.NewEncoder(&responseText).Encode(response); err != nil {
				t.Fatalf("encode response: %v", err)
			}
			if strings.Contains(responseText.String(), secret) {
				t.Fatalf("response leaked ACP error data: %s", responseText.String())
			}
			for _, entry := range observed.All() {
				if strings.Contains(entry.Message, secret) || strings.Contains(fmt.Sprint(entry.ContextMap()), secret) {
					t.Fatalf("log leaked ACP error data: %+v", entry)
				}
			}
		})
	}
}
