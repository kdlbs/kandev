package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestConnectionScopedStartupEventsBypassDurableReceipt(t *testing.T) {
	tests := []struct {
		name  string
		event agentctl.AgentEvent
	}{
		{
			name: "authentication required",
			event: agentctl.AgentEvent{
				Type:        streams.EventTypeAuthRequired,
				AuthMethods: []streams.AuthMethodInfo{{ID: "cursor_login", Name: "Cursor login"}},
			},
		},
		{
			name: "agent capabilities",
			event: agentctl.AgentEvent{
				Type:          streams.EventTypeAgentCapabilities,
				SupportsImage: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution, _ := retentionBootstrapOwner()
			called := false
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{
				OnAgentEventWithGeneration: func(_ *AgentExecution, event agentctl.AgentEvent, _ uint64) {
					called = event.Type == test.event.Type
				},
			}, nil, nil)
			defer manager.Wait()

			if err := manager.processAgentEvent(context.Background(), execution, nil, nil, test.event, 0); err != nil {
				t.Fatalf("process connection-scoped %s event: %v", test.name, err)
			}
			if !called {
				t.Fatalf("connection-scoped %s event was not forwarded", test.name)
			}
		})
	}
}

func TestConnectionScopedStartupControlEventRequiresNoIdentity(t *testing.T) {
	tests := []struct {
		name  string
		event agentctl.AgentEvent
		want  bool
	}{
		{name: "capabilities", event: agentctl.AgentEvent{Type: streams.EventTypeAgentCapabilities}, want: true},
		{name: "authentication", event: agentctl.AgentEvent{Type: streams.EventTypeAuthRequired}, want: true},
		{name: "session-bound capabilities", event: agentctl.AgentEvent{Type: streams.EventTypeAgentCapabilities, SessionID: "native-session"}},
		{name: "stream-bound authentication", event: agentctl.AgentEvent{Type: streams.EventTypeAuthRequired, DeliveryStreamID: "stream"}},
		{name: "sequence-bound capabilities", event: agentctl.AgentEvent{Type: streams.EventTypeAgentCapabilities, DeliverySequence: 1}},
		{name: "ordinary event", event: agentctl.AgentEvent{Type: streams.EventTypeMessageChunk}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionScopedStartupControlEvent(test.event); got != test.want {
				t.Fatalf("connectionScopedStartupControlEvent(%+v) = %v, want %v", test.event, got, test.want)
			}
		})
	}
}
