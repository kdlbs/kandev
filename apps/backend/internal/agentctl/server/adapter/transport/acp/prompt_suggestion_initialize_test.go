package acp

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// capabilityAgent replies to initialize with a fixed response so tests can
// assert what the adapter publishes from real wire evidence.
type capabilityAgent struct {
	burstAgent
	response acp.InitializeResponse
}

func (a *capabilityAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return a.response, nil
}

func newCapabilityTestAdapter(t *testing.T, agentID string, requested bool) (*Adapter, *acp.AgentSideConnection) {
	t.Helper()
	a := newTestAdapterForAgent(agentID)
	a.cfg.PromptSuggestions = requested
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	agentConn := acp.NewAgentSideConnection(&capabilityAgent{response: acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentInfo:       &acp.Implementation{Name: "custom-bridge", Version: "1.0.0"},
		AgentCapabilities: acp.AgentCapabilities{
			Meta: map[string]any{"claudeCode": map[string]any{"promptSuggestions": true}},
		},
	}}, a2cW, c2aR)
	return a, agentConn
}

func capabilityEvent(t *testing.T, a *Adapter) AgentEvent {
	t.Helper()
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeAgentCapabilities {
			return event
		}
	}
	t.Fatal("no agent_capabilities event published")
	return AgentEvent{}
}

// TestConnectPublishesPromptSuggestionCapabilities verifies the initialize path
// publishes raw support independently of the effective launch request.
func TestConnectPublishesPromptSuggestionCapabilities(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	requestedAdapter, _ := newCapabilityTestAdapter(t, "custom-acp", true)
	if err := requestedAdapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	enabled := capabilityEvent(t, requestedAdapter)
	if enabled.NativePromptSuggestionsSupported == nil || !*enabled.NativePromptSuggestionsSupported {
		t.Fatalf("raw support = %v, want explicit true", enabled.NativePromptSuggestionsSupported)
	}
	if !enabled.SupportsPromptSuggestions {
		t.Fatal("effective enablement = false, want true for a requested supported session")
	}

	unrequestedAdapter, _ := newCapabilityTestAdapter(t, "custom-acp", false)
	if err := unrequestedAdapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	disabled := capabilityEvent(t, unrequestedAdapter)
	if disabled.NativePromptSuggestionsSupported == nil || !*disabled.NativePromptSuggestionsSupported {
		t.Fatalf("raw support = %v, want explicit true", disabled.NativePromptSuggestionsSupported)
	}
	if disabled.SupportsPromptSuggestions {
		t.Fatal("effective enablement = true, want false when the launch did not request suggestions")
	}
}
