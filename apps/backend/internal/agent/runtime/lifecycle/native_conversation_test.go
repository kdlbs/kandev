package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestContinuationNativeOnlyRestoreRejectsReplacement(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.session.load" {
			response, _ := ws.NewError(
				msg.ID,
				msg.Action,
				ws.ErrorCodeInternalError,
				"agent does not support session loading (LoadSession capability is false)",
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
		id:      "auggie",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("auggie"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: true,
			},
		},
	}

	_, err := sessionManager.InitializeSessionWithSettingsPolicy(
		context.WithValue(context.Background(), requiredNativeConversationKey{}, "saved-session"), nil, client, agentConfig, "saved-session", "/workspace", nil,
		SessionSettingsPolicyStrict,
	)
	if err == nil {
		t.Fatal("expected session/load failure while preserving provider identity")
	}

	var loadSeen bool
	for _, action := range mock.getActionLog() {
		if action == "agent.session.load" {
			loadSeen = true
		}
		if action == "agent.session.new" {
			t.Fatal("provider-restored recovery must not replace the stored provider conversation")
		}
	}
	if !loadSeen {
		t.Fatal("expected provider-restored recovery to attempt loading the stored conversation")
	}
}
