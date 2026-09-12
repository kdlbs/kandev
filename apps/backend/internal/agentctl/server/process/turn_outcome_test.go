package process

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// fakeTurnOutcomeRecorder captures RetainTurnOutcome calls for assertion,
// standing in for instance.Manager without importing it (process must not
// import instance, which already imports process).
type fakeTurnOutcomeRecorder struct {
	calls []fakeTurnOutcomeCall
}

type fakeTurnOutcomeCall struct {
	instanceID string
	event      adapter.AgentEvent
}

func (f *fakeTurnOutcomeRecorder) RetainTurnOutcome(instanceID string, event adapter.AgentEvent) (int64, bool) {
	f.calls = append(f.calls, fakeTurnOutcomeCall{instanceID: instanceID, event: event})
	return int64(len(f.calls)), true
}

// TestRecordTerminalOutcomeRetainsCompleteAndErrorEvents pins
// AC-EXECUTORS-SURVIVAL-004.1: the two terminal event types must reach the
// wired recorder with the instance ID SetTurnOutcomeRecorder was given.
func TestRecordTerminalOutcomeRetainsCompleteAndErrorEvents(t *testing.T) {
	terminalTypes := []string{adapter.EventTypeComplete, adapter.EventTypeError}
	for _, eventType := range terminalTypes {
		t.Run(eventType, func(t *testing.T) {
			recorder := &fakeTurnOutcomeRecorder{}
			m := &Manager{}
			m.SetTurnOutcomeRecorder("instance-1", recorder)

			event := adapter.AgentEvent{Type: eventType}
			m.recordTerminalOutcome(&event)

			if len(recorder.calls) != 1 {
				t.Fatalf("recorder calls = %d, want 1", len(recorder.calls))
			}
			if recorder.calls[0].instanceID != "instance-1" {
				t.Fatalf("instanceID = %q, want instance-1", recorder.calls[0].instanceID)
			}
			if recorder.calls[0].event.Type != eventType {
				t.Fatalf("event type = %q, want %q", recorder.calls[0].event.Type, eventType)
			}
			if event.ControlTurnID != int64(len(recorder.calls)) {
				t.Fatalf("event.ControlTurnID = %d, want %d (stamped from RetainTurnOutcome's return)",
					event.ControlTurnID, len(recorder.calls))
			}
		})
	}
}

// TestRecordTerminalOutcomeIgnoresNonTerminalEvents pins that every other
// event type -- including the two excluded MCP-attachment sites' type and
// the permission lifecycle types -- must never reach the recorder. AC-004 is
// specifically "what happened to my last turn," not every lifecycle event.
func TestRecordTerminalOutcomeIgnoresNonTerminalEvents(t *testing.T) {
	nonTerminalTypes := []string{
		adapter.EventTypePermissionRequest,
		adapter.EventTypePermissionCancelled,
		streams.EventTypeMCPAttachment,
		"context_window",
		"session_models",
		"message_chunk",
	}
	for _, eventType := range nonTerminalTypes {
		t.Run(eventType, func(t *testing.T) {
			recorder := &fakeTurnOutcomeRecorder{}
			m := &Manager{}
			m.SetTurnOutcomeRecorder("instance-1", recorder)

			event := adapter.AgentEvent{Type: eventType}
			m.recordTerminalOutcome(&event)

			if len(recorder.calls) != 0 {
				t.Fatalf("recorder calls = %d for type %q, want 0", len(recorder.calls), eventType)
			}
		})
	}
}

// TestRecordTerminalOutcomeNoopWithoutRecorder pins that a Manager with no
// recorder wired (the common case: most tests, and the e2e harness) safely
// ignores terminal events instead of panicking.
func TestRecordTerminalOutcomeNoopWithoutRecorder(t *testing.T) {
	m := &Manager{}
	event := adapter.AgentEvent{Type: adapter.EventTypeComplete}
	m.recordTerminalOutcome(&event)
	if event.ControlTurnID != 0 {
		t.Fatalf("ControlTurnID = %d, want 0 with no recorder wired", event.ControlTurnID)
	}
}

// TestSendUpdateBlockingRecordsTerminalOutcomeOnDelivery pins that both the
// fast path (room available) and the parked path (room freed up later) of
// sendUpdateBlocking retain a terminal event on successful delivery.
func TestSendUpdateBlockingRecordsTerminalOutcomeOnDelivery(t *testing.T) {
	t.Run("fast path", func(t *testing.T) {
		recorder := &fakeTurnOutcomeRecorder{}
		m := &Manager{updatesCh: make(chan adapter.AgentEvent, 1)}
		m.SetTurnOutcomeRecorder("instance-1", recorder)

		if !m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError}) {
			t.Fatal("sendUpdateBlocking returned false with room available")
		}
		if len(recorder.calls) != 1 {
			t.Fatalf("recorder calls = %d, want 1", len(recorder.calls))
		}
	})

	t.Run("parked path", func(t *testing.T) {
		recorder := &fakeTurnOutcomeRecorder{}
		m := &Manager{updatesCh: make(chan adapter.AgentEvent, 1)}
		m.SetTurnOutcomeRecorder("instance-1", recorder)
		m.stopChSnapshot.Store(make(chan struct{}))
		m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer

		done := make(chan bool, 1)
		go func() { done <- m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError}) }()

		select {
		case <-done:
			t.Fatal("sendUpdateBlocking returned before the channel had room")
		case <-time.After(50 * time.Millisecond):
		}

		<-m.updatesCh // drain the pre-filled slot

		select {
		case ok := <-done:
			if !ok {
				t.Fatal("sendUpdateBlocking returned false after room freed up")
			}
		case <-time.After(time.Second):
			t.Fatal("sendUpdateBlocking did not return promptly after room freed up")
		}
		if len(recorder.calls) != 1 {
			t.Fatalf("recorder calls = %d, want 1", len(recorder.calls))
		}
	})
}

// TestForwardUpdatesRecordsTerminalOutcomeForAdapterOriginatedEvents pins
// that a terminal event emitted by the ACP adapter (session/prompt
// completion, protocol-level error) -- which never goes through
// sendUpdateBlocking, only through forwardUpdates's copy from the adapter's
// own channel -- is also retained. This is the coverage this manager cannot
// get any other way, since the adapter has no direct reference to the
// recorder.
func TestForwardUpdatesRecordsTerminalOutcomeForAdapterOriginatedEvents(t *testing.T) {
	recorder := &fakeTurnOutcomeRecorder{}
	stopCh := make(chan struct{})
	stub := newStubAdapter()
	m := &Manager{
		adapter:   stub,
		updatesCh: make(chan adapter.AgentEvent, 10),
		stopCh:    stopCh,
		logger:    newTestLogger(t),
	}
	m.SetTurnOutcomeRecorder("instance-1", recorder)
	m.wg.Add(1)
	go m.forwardUpdates(stub, stopCh)
	t.Cleanup(func() { close(stopCh) })

	stub.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
	stub.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk}

	select {
	case <-m.updatesCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the complete event to be forwarded")
	}
	select {
	case <-m.updatesCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the message_chunk event to be forwarded")
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("recorder calls = %d, want 1 (only the complete event)", len(recorder.calls))
	}
	if recorder.calls[0].event.Type != adapter.EventTypeComplete {
		t.Fatalf("retained event type = %q, want complete", recorder.calls[0].event.Type)
	}
}

// TestSendUpdateBlockingStampsControlTurnIDOnDeliveredCopy and
// TestForwardUpdatesStampsControlTurnIDOnDeliveredCopy pin
// AC-EXECUTORS-SURVIVAL-004.4: the control-server-assigned turn identifier
// must reach the copy actually delivered on m.updatesCh (the "delivered
// event" observation), not just the pre-stamp copy Retain stores (the
// "retained turn status" observation, which carries it via
// TurnOutcome.TurnID instead). Without this stamp a backend re-attaching
// after a restart has no way to recognize a live redelivery of the exact
// turn it already applied from GetTurnOutcome.
func TestSendUpdateBlockingStampsControlTurnIDOnDeliveredCopy(t *testing.T) {
	recorder := &fakeTurnOutcomeRecorder{}
	m := &Manager{updatesCh: make(chan adapter.AgentEvent, 1)}
	m.SetTurnOutcomeRecorder("instance-1", recorder)

	if !m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError}) {
		t.Fatal("sendUpdateBlocking returned false with room available")
	}

	delivered := <-m.updatesCh
	if delivered.ControlTurnID != 1 {
		t.Fatalf("delivered.ControlTurnID = %d, want 1", delivered.ControlTurnID)
	}
}

func TestForwardUpdatesStampsControlTurnIDOnDeliveredCopy(t *testing.T) {
	recorder := &fakeTurnOutcomeRecorder{}
	stopCh := make(chan struct{})
	stub := newStubAdapter()
	m := &Manager{
		adapter:   stub,
		updatesCh: make(chan adapter.AgentEvent, 10),
		stopCh:    stopCh,
		logger:    newTestLogger(t),
	}
	m.SetTurnOutcomeRecorder("instance-1", recorder)
	m.wg.Add(1)
	go m.forwardUpdates(stub, stopCh)
	t.Cleanup(func() { close(stopCh) })

	stub.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}

	select {
	case delivered := <-m.updatesCh:
		if delivered.ControlTurnID != 1 {
			t.Fatalf("delivered.ControlTurnID = %d, want 1", delivered.ControlTurnID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the complete event to be forwarded")
	}
}
