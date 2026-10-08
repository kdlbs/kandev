package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestInitializeFailurePhase(t *testing.T) {
	tests := []struct {
		name             string
		failAction       string
		resume           bool
		existingSession  string
		wantInitPhaseErr bool
	}{
		{name: "ACP initialize", failAction: "agent.initialize", wantInitPhaseErr: true},
		{name: "session new", failAction: "agent.session.new"},
		{name: "session load", failAction: "agent.session.load", resume: true, existingSession: "saved-session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == tt.failAction {
					response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "injected phase failure", map[string]any{
						"startup_evidence": map[string]any{
							"process_generation":  uint64(7),
							"exit_disposition":    "ordinary_exit",
							"exit_code":           1,
							"npm_code":            "ECONNRESET",
							"collection_complete": true,
						},
					})
					return response
				}
				return mock.defaultHandler(msg)
			}

			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)
			if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
				t.Fatalf("connect agent stream: %v", err)
			}
			waitForWSConnected(t, mock)

			sessionManager := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
			agentConfig := &testAgent{
				id:      "test-agent",
				enabled: true,
				runtimeConfig: &agents.RuntimeConfig{
					Cmd:      agents.NewCommand("test-agent"),
					Protocol: agent.ProtocolACP,
					SessionConfig: agents.SessionConfig{
						NativeSessionResume: tt.resume,
					},
				},
			}
			_, err := sessionManager.InitializeSession(
				context.Background(), client, agentConfig, tt.existingSession, "/workspace", nil,
			)
			if err == nil {
				t.Fatal("expected injected failure")
			}
			var phaseErr *SessionInitializationPhaseError
			if errors.As(err, &phaseErr) != tt.wantInitPhaseErr {
				t.Fatalf("phase error = %#v, want initialize phase error %v (error: %v)", phaseErr, tt.wantInitPhaseErr, err)
			}
			if !tt.wantInitPhaseErr && phaseErr != nil {
				t.Fatalf("later session failure was mislabeled as initialize: %#v", phaseErr)
			}
			if tt.wantInitPhaseErr {
				var initializeErr *agentctl.InitializeError
				if !errors.As(phaseErr, &initializeErr) {
					t.Fatalf("cause type = %T, want *agentctl.InitializeError", phaseErr.Cause)
				}
				if initializeErr.StartupEvidence == nil || initializeErr.StartupEvidence.NPMCode != "ECONNRESET" {
					t.Fatalf("initialize evidence = %#v, want ECONNRESET", initializeErr.StartupEvidence)
				}
			}
		})
	}
}
