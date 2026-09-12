package acp

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestConvergenceEventEmittersParkOnFullChannelThenDeliver pins
// AC-EXECUTORS-SURVIVAL-001.5/.6 for the three session_models convergence
// emitters in adapter_session.go (emitSetModelEvent, emitSetConfigOptionEvent,
// emitAuthoritativeConfigOptions): a full updatesCh must park the call rather
// than silently dropping the convergence event, and release once a consumer
// drains a slot. All three used to call sendUpdateLocked (drop-on-full) while
// holding a.mu; they now build the event under a.mu, release the lock, and
// deliver via the existing sendUpdate helper (see emitDialectContextWindow's
// doc comment for why the lock must be released first).
func TestConvergenceEventEmittersParkOnFullChannelThenDeliver(t *testing.T) {
	cachedModels := []modelInfo{{ModelId: "gpt-5", Name: "GPT-5"}}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "gpt-5"},
		{Type: "select", ID: "reasoning_effort", CurrentValue: "low"},
	}

	tests := []struct {
		name string
		emit func(a *Adapter)
	}{
		{
			name: "emitSetModelEvent",
			emit: func(a *Adapter) {
				a.emitSetModelEvent("sess-1", "gpt-5.6", cachedModels, cachedConfig)
			},
		},
		{
			name: "emitSetConfigOptionEvent",
			emit: func(a *Adapter) {
				a.emitSetConfigOptionEvent("sess-1", "reasoning_effort", "high", cachedModels, cachedConfig)
			},
		},
		{
			name: "emitAuthoritativeConfigOptions",
			emit: func(a *Adapter) {
				a.emitAuthoritativeConfigOptions("sess-1", "reasoning_effort", nil, cachedModels)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAdapter()
			t.Cleanup(func() { _ = a.Close() })
			a.sessionID = "sess-1"
			a.updatesCh = make(chan AgentEvent, 1)
			a.updatesCh <- AgentEvent{Type: streams.EventTypeMessageChunk} // fill the buffer

			done := make(chan struct{})
			go func() {
				tt.emit(a)
				close(done)
			}()

			select {
			case <-done:
				t.Fatal("emitter returned before the channel had room -- it must park, not drop")
			case <-time.After(50 * time.Millisecond):
			}

			<-a.updatesCh // drain the pre-filled slot, freeing room for the parked send

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("emitter did not return promptly after the channel had room")
			}

			select {
			case event := <-a.updatesCh:
				if event.Type != streams.EventTypeSessionModels {
					t.Fatalf("event type = %q, want session_models", event.Type)
				}
			default:
				t.Fatal("the parked convergence event was never delivered")
			}
		})
	}
}

// TestConvergenceEventEmittersReleaseOnAdapterClose pins the pause-not-a-hang
// half: a parked convergence emitter must release once the adapter closes,
// without ever delivering the event.
func TestConvergenceEventEmittersReleaseOnAdapterClose(t *testing.T) {
	cachedModels := []modelInfo{{ModelId: "gpt-5", Name: "GPT-5"}}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "gpt-5"},
	}

	a := newTestAdapter()
	a.sessionID = "sess-1"
	a.updatesCh = make(chan AgentEvent, 1)
	a.updatesCh <- AgentEvent{Type: streams.EventTypeMessageChunk}

	done := make(chan struct{})
	go func() {
		a.emitSetModelEvent("sess-1", "gpt-5.6", cachedModels, cachedConfig)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("emitter returned before the channel had room or the adapter closed")
	case <-time.After(50 * time.Millisecond):
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emitter did not return promptly after Close")
	}
}
