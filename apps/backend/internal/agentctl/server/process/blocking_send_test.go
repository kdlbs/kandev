package process

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

// TestSendUpdateBlockingSendsImmediatelyWhenChannelHasRoom pins the common
// case: a converted send behaves exactly like the non-blocking one did
// whenever the channel isn't actually full.
func TestSendUpdateBlockingSendsImmediatelyWhenChannelHasRoom(t *testing.T) {
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}
	stopCh := make(chan struct{})
	m.stopChSnapshot.Store(stopCh)

	if ok := m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError, Error: "boom"}); !ok {
		t.Fatal("sendUpdateBlocking() = false, want true (channel had room)")
	}

	select {
	case event := <-m.updatesCh:
		if event.Error != "boom" {
			t.Fatalf("Error = %q, want %q", event.Error, "boom")
		}
	default:
		t.Fatal("event was not enqueued on updatesCh")
	}
}

// TestSendUpdateBlockingParksUntilRoomFreesUp pins AC-EXECUTORS-SURVIVAL-001.5/.6:
// a full channel must PARK the producer rather than discard the event, and
// deliver it once a consumer drains the channel.
func TestSendUpdateBlockingParksUntilRoomFreesUp(t *testing.T) {
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}
	stopCh := make(chan struct{})
	m.stopChSnapshot.Store(stopCh)
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer

	done := make(chan bool, 1)
	go func() {
		done <- m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError, Error: "parked"})
	}()

	select {
	case <-done:
		t.Fatal("sendUpdateBlocking returned before the channel had room -- it must park, not discard")
	case <-time.After(50 * time.Millisecond):
	}

	<-m.updatesCh // drain the blocking event, freeing a slot

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("sendUpdateBlocking() = false once room freed up, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("sendUpdateBlocking did not unblock after the channel drained")
	}

	select {
	case event := <-m.updatesCh:
		if event.Error != "parked" {
			t.Fatalf("Error = %q, want %q", event.Error, "parked")
		}
	default:
		t.Fatal("the parked event was never delivered")
	}
}

// TestSendUpdateBlockingReleasesOnStop pins the "pause not a hang" half of
// the design: a producer parked on a full channel must be released the
// instant the instance's stop signal fires, without waiting for a consumer
// that may never arrive.
func TestSendUpdateBlockingReleasesOnStop(t *testing.T) {
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}
	stopCh := make(chan struct{})
	m.stopChSnapshot.Store(stopCh)
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer

	done := make(chan bool, 1)
	go func() {
		done <- m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError, Error: "released"})
	}()

	select {
	case <-done:
		t.Fatal("sendUpdateBlocking returned before stop was signalled")
	case <-time.After(50 * time.Millisecond):
	}

	close(stopCh)

	select {
	case ok := <-done:
		if ok {
			t.Fatal("sendUpdateBlocking() = true after a stop release, want false (event must not be delivered)")
		}
	case <-time.After(time.Second):
		t.Fatal("sendUpdateBlocking did not release on stop")
	}
}

// TestSendUpdateBlockingSendsWithRoomEvenWithNoStopChannelEstablished pins
// that the fast, common path never depends on a stop channel existing at
// all: when there's room, the send just happens.
func TestSendUpdateBlockingSendsWithRoomEvenWithNoStopChannelEstablished(t *testing.T) {
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}

	if ok := m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError}); !ok {
		t.Fatal("sendUpdateBlocking() = false with room available, want true regardless of stop-channel state")
	}
}

// TestSendUpdateBlockingReturnsFalseImmediatelyWhenNeverStartedAndFull pins
// the defensive behaviour for a Manager whose stop channel was never
// established (Start has not run) and whose channel is actually full: treat
// it as already stopped rather than parking forever with no lifecycle that
// could ever release it.
func TestSendUpdateBlockingReturnsFalseImmediatelyWhenNeverStartedAndFull(t *testing.T) {
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer

	done := make(chan bool, 1)
	go func() {
		done <- m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError})
	}()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("sendUpdateBlocking() = true with no stop channel established, want false")
		}
	case <-time.After(time.Second):
		t.Fatal("sendUpdateBlocking did not return promptly with no stop channel established")
	}
}
