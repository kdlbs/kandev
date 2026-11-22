package acp

import (
	"context"
	"io"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type startupWarningBeforeBindingAgent struct {
	*sessionRequestCaptureAgent
	conn          *acpsdk.AgentSideConnection
	notifications []acpsdk.SessionNotification
}

func (a *startupWarningBeforeBindingAgent) NewSession(
	ctx context.Context,
	_ acpsdk.NewSessionRequest,
) (acpsdk.NewSessionResponse, error) {
	for _, notification := range a.notifications {
		if err := a.conn.SessionUpdate(ctx, notification); err != nil {
			return acpsdk.NewSessionResponse{}, err
		}
	}
	return acpsdk.NewSessionResponse{SessionId: "created-session"}, nil
}

func TestCodexMCPStartupWarningBuffersUntilNewSessionBinding(t *testing.T) {
	newSessionReport := codexMCPStartupNotification(
		t, "mcp_startup.server.550e8400-e29b-41d4-a716-446655440004", "mcp__server__startup",
	)
	newSessionReport.SessionId = "created-session"
	otherSessionReport := codexMCPStartupNotification(
		t, "mcp_startup.server.550e8400-e29b-41d4-a716-446655440005", "mcp__server__startup",
	)
	otherSessionReport.SessionId = "other-session"

	agent := &startupWarningBeforeBindingAgent{
		sessionRequestCaptureAgent: &sessionRequestCaptureAgent{},
		notifications:              []acpsdk.SessionNotification{newSessionReport, otherSessionReport},
	}
	a := newTestAdapterForAgent(codexAgentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	agent.conn = acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize adapter: %v", err)
	}
	if sessionID, err := a.NewSession(ctx, nil); err != nil || sessionID != "created-session" {
		t.Fatalf("NewSession() = (%q, %v), want created-session", sessionID, err)
	}

	events := drainEvents(a)
	var warnings []AgentEvent
	for _, event := range events {
		if event.Type == streams.EventTypeMCPStartupWarning {
			warnings = append(warnings, event)
		}
	}
	if len(warnings) != 1 || warnings[0].SessionID != "created-session" ||
		warnings[0].ToolCallID != string(newSessionReport.Update.ToolCall.ToolCallId) {
		t.Fatalf("bound startup warnings = %+v, want only the returned session report", warnings)
	}
	a.mu.RLock()
	creationPending := a.sessionCreationPending
	pending := len(a.pendingSessionStartupWarnings)
	a.mu.RUnlock()
	if creationPending || pending != 0 {
		t.Fatalf("session creation warning buffer retained state: pending=%t reports=%d", creationPending, pending)
	}
}
