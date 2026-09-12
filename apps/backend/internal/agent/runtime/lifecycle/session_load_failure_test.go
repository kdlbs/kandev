package lifecycle

import (
	"context"
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
	}{
		{
			name:    "serialized deadline",
			message: "internal error: context deadline exceeded",
		},
		{
			name:    "serialized internal error",
			message: "internal error: provider failed while loading the session",
		},
		{
			name:    "authentication failure",
			message: "authentication required",
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
				client,
				agentConfig,
				"saved-session",
				"/workspace",
				nil,
			)
			if err == nil {
				t.Fatal("expected session/load failure")
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("error = %q, want cause %q", err, tt.message)
			}

			for _, action := range mock.getActionLog() {
				if action == "agent.session.new" {
					t.Fatal("session/load failure must not create a replacement session")
				}
			}
		})
	}
}

func TestInitializeSession_LoadCompatibilityFailureCreatesReplacement(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "method not found", message: "method not found"},
		{name: "capability mismatch", message: "agent does not support session loading (LoadSession capability is false)"},
		{name: "unknown session", message: "Resource not found"},
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

			result, err := sessionManager.InitializeSession(
				context.Background(),
				client,
				agentConfig,
				"saved-session",
				"/workspace",
				nil,
			)
			if err != nil {
				t.Fatalf("InitializeSession: %v", err)
			}
			if result.SessionID != "test-session-123" {
				t.Fatalf("session ID = %q, want replacement session", result.SessionID)
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
			if loadCalls != 1 || newCalls != 1 {
				t.Fatalf("load/new calls = %d/%d, want 1/1; actions: %v", loadCalls, newCalls, actions)
			}
		})
	}
}
