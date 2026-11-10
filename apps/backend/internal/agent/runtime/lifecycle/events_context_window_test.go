package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestPublishContextWindowCompactionPolicy pins provider-derived eligibility:
// Copilot ACP usage cannot distinguish a compaction from a switch between
// parent and subagent contexts, so its samples are excluded from inference.
// The advisory agent_id payload field stays the execution identifier.
func TestPublishContextWindowCompactionPolicy(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	publisher := NewEventPublisher(eventBus, log)
	subject := events.BuildContextWindowSubject("sess-policy")

	received := make(chan *bus.Event, 4)
	sub, err := eventBus.Subscribe(subject, func(_ context.Context, event *bus.Event) error {
		received <- event
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cases := []struct {
		name        string
		agentID     string
		executionID string
		native      bool
		want        bool
	}{
		{name: "copilot acp", agentID: agents.CopilotACPAgentID, executionID: "exec-copilot", native: true, want: true},
		// Restoration retains the provider type on AgentExecution even when a
		// replacement execution identifier is allocated.
		{name: "copilot with replacement execution id", agentID: agents.CopilotACPAgentID, executionID: "exec-restored", native: true, want: true},
		{name: "copilot without native collection", agentID: agents.CopilotACPAgentID, executionID: "exec-copilot-legacy", want: true},
		{name: "other provider", agentID: "claude-acp", executionID: "exec-claude", want: false},
		{name: "legacy execution without provider", agentID: "", executionID: "exec-legacy", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			publisher.PublishContextWindow(
				&AgentExecution{
					ID: tc.executionID, TaskID: "task-policy", SessionID: "sess-policy",
					AgentID: tc.agentID, CopilotNativeSessionEvents: tc.native,
				},
				200000, 50000, 150000, 25,
			)

			var event *bus.Event
			select {
			case event = <-received:
			default:
				t.Fatal("expected a published context window event")
			}
			payload, ok := event.Data.(ContextWindowEventPayload)
			if !ok {
				t.Fatalf("expected ContextWindowEventPayload, got %T", event.Data)
			}
			if payload.CompactionInferenceDisabled != tc.want {
				t.Fatalf("compaction_inference_disabled = %v, want %v", payload.CompactionInferenceDisabled, tc.want)
			}
			if payload.CopilotNativeSessionEvents != tc.native {
				t.Fatalf("copilot_native_session_events = %v, want %v", payload.CopilotNativeSessionEvents, tc.native)
			}
			if payload.AgentID != tc.executionID {
				t.Fatalf("agent_id = %q, want execution identifier %q", payload.AgentID, tc.executionID)
			}
			if payload.ContextWindowUsed != 50000 || payload.ContextWindowSize != 200000 || payload.ContextEfficiency != 25 {
				t.Fatalf("usage fields were not preserved: %#v", payload)
			}

			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("unmarshal payload fields: %v", err)
			}
			rawMarker, hasMarker := fields["compaction_inference_disabled"]
			if hasMarker != tc.want {
				t.Fatalf("serialized marker presence = %v, want %v: %s", hasMarker, tc.want, encoded)
			}
			if tc.want {
				var marker bool
				if err := json.Unmarshal(rawMarker, &marker); err != nil {
					t.Fatalf("decode marker: %v", err)
				}
				if !marker {
					t.Fatalf("serialized marker = false, want true")
				}
			}
			rawNative, hasNative := fields["copilot_native_session_events"]
			if hasNative != tc.native {
				t.Fatalf("serialized native presence = %v, want %v: %s", hasNative, tc.native, encoded)
			}
			if tc.native {
				var native bool
				if err := json.Unmarshal(rawNative, &native); err != nil || !native {
					t.Fatalf("serialized native = %v, want true: %s", native, encoded)
				}
			}
		})
	}
}
