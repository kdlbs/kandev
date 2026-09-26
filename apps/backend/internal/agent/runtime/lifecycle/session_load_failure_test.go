package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestInitializeSession_LoadFailureDoesNotCreateReplacement(t *testing.T) {
	tests := []struct {
		name    string
		message string
		reason  RestoreReason
	}{
		{
			name:    "serialized deadline",
			message: "internal error: context deadline exceeded",
			reason:  RestoreReasonNone,
		},
		{
			name:    "serialized internal error",
			message: "internal error: provider failed while loading the session",
			reason:  RestoreReasonUnknown,
		},
		{
			name:    "unrelated missing resource",
			message: "internal error: resource not found while resolving configuration",
			reason:  RestoreReasonConfiguration,
		},
		{
			name:    "authentication failure",
			message: "authentication required",
			reason:  RestoreReasonAuthentication,
		},
		{
			name: "missing rollout for a different session",
			message: `load session failed: failed to load session: {"code":-32603,"message":"Internal error",` +
				`"data":{"details":"no rollout found for thread id another-session"}}`,
			reason: RestoreReasonUnknown,
		},
		{
			name:    "unstructured missing rollout phrase",
			message: "internal error: no rollout found for thread id saved-session",
			reason:  RestoreReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == "agent.session.load" {
					response, _ := ws.NewError(
						msg.ID,
						msg.Action,
						ws.ErrorCodeInternalError,
						tt.message,
						nil,
					)
					return response
				}
				return mock.defaultHandler(msg)
			}

			stopCh := newTestStopCh(t)
			sessionManager := NewSessionManager(newSessionTestLogger(), stopCh)
			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)

			if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
				t.Fatalf("connect agent stream: %v", err)
			}
			waitForWSConnected(t, mock)

			agentConfig := &testAgent{
				id:      "test-agent",
				enabled: true,
				runtimeConfig: &agents.RuntimeConfig{
					Cmd:      agents.NewCommand("test-agent"),
					Protocol: agent.ProtocolACP,
					SessionConfig: agents.SessionConfig{
						NativeSessionResume: true,
					},
				},
			}

			_, err := sessionManager.InitializeSession(
				context.Background(),
				nil,
				client,
				agentConfig,
				"saved-session",
				"/workspace",
				nil,
			)
			if err == nil {
				t.Fatal("expected session/load failure")
			}
			if tt.reason == RestoreReasonNone {
				if !strings.Contains(err.Error(), tt.message) {
					t.Fatalf("error = %q, want transport cause %q", err, tt.message)
				}
			} else {
				var restoreErr *RestoreRequiredError
				if !errors.As(err, &restoreErr) {
					t.Fatalf("error = %q, want RestoreRequiredError", err)
				}
				if restoreErr.Decision.Reason != tt.reason {
					t.Fatalf("restore reason = %q, want %q", restoreErr.Decision.Reason, tt.reason)
				}
			}

			for _, action := range mock.getActionLog() {
				if action == "agent.session.new" {
					t.Fatal("session/load failure must not create a replacement session")
				}
			}
		})
	}
}

func TestInitializeSession_LoadCompatibilityFailureBlocksReplacement(t *testing.T) {
	tests := []struct {
		name    string
		message string
		reason  RestoreReason
	}{
		{name: "method not found", message: "method not found", reason: RestoreReasonUnknown},
		{name: "capability mismatch", message: "agent does not support session loading (LoadSession capability is false)", reason: RestoreReasonNativeResumeUnsupported},
		{name: "unknown session", message: "Resource not found", reason: RestoreReasonNativeStateMissing},
		{
			name: "missing provider rollout",
			message: `load session failed: failed to load session: {"code":-32603,"message":"Internal error",` +
				`"data":{"details":"no rollout found for thread id saved-session"}}`,
			reason: RestoreReasonNativeStateMissing,
		},
		{
			name: "missing auggie session",
			message: `load session failed: failed to load session: {"code":-32602,"message":"Invalid params",` +
				`"data":{"details":"Session not found: saved-session"}}`,
			reason: RestoreReasonNativeStateMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == "agent.session.load" {
					response, _ := ws.NewError(
						msg.ID,
						msg.Action,
						ws.ErrorCodeInternalError,
						tt.message,
						nil,
					)
					return response
				}
				return mock.defaultHandler(msg)
			}

			stopCh := newTestStopCh(t)
			sessionManager := NewSessionManager(newSessionTestLogger(), stopCh)
			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)
			if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
				t.Fatalf("connect agent stream: %v", err)
			}
			waitForWSConnected(t, mock)

			agentConfig := &testAgent{
				id:      "test-agent",
				enabled: true,
				runtimeConfig: &agents.RuntimeConfig{
					Cmd:      agents.NewCommand("test-agent"),
					Protocol: agent.ProtocolACP,
					SessionConfig: agents.SessionConfig{
						NativeSessionResume: true,
					},
				},
			}

			_, err := sessionManager.InitializeSession(
				context.Background(),
				nil,
				client,
				agentConfig,
				"saved-session",
				"/workspace",
				nil,
			)
			if err == nil {
				t.Fatal("expected compatibility failure")
			}
			var restoreErr *RestoreRequiredError
			if !errors.As(err, &restoreErr) {
				t.Fatalf("error = %v, want RestoreRequiredError", err)
			}
			if restoreErr.Decision.Reason != tt.reason {
				t.Fatalf("restore reason = %q, want %q", restoreErr.Decision.Reason, tt.reason)
			}

			actions := mock.getActionLog()
			loadCalls, newCalls := 0, 0
			for _, action := range actions {
				switch action {
				case "agent.session.load":
					loadCalls++
				case "agent.session.new":
					newCalls++
				}
			}
			if loadCalls != 1 || newCalls != 0 {
				t.Fatalf("load/new calls = %d/%d, want 1/0; actions: %v", loadCalls, newCalls, actions)
			}
		})
	}
}
