package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	return log
}

// stubBus is a minimal in-memory EventBus that records published events on
// the SystemJobUpdate subject. It is *not* a full implementation of all
// EventBus methods - only Publish is used by Tracker.
type stubBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (s *stubBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *stubBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (s *stubBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (s *stubBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (s *stubBus) Close()            {}
func (s *stubBus) IsConnected() bool { return true }

func (s *stubBus) snapshot() []*bus.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*bus.Event, len(s.events))
	copy(cp, s.events)
	return cp
}

// waitForState polls Tracker.Get(id) until the job reaches a terminal state
// (succeeded or failed) or the timeout fires. The Tracker runs work in a
// goroutine so tests must wait for completion.
func waitForState(t *testing.T, tracker *Tracker, id string, target State) *Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j := tracker.Get(id)
		if j != nil && j.State == target {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach state %s within 2s; last = %+v", id, target, tracker.Get(id))
	return nil
}

// waitForEvents polls the stub bus until at least n events have been
// published or the timeout fires. The terminal lifecycle event is published
// after the transition releases the lock (see Tracker.transition), so
// observing the terminal state via waitForState does not guarantee the
// terminal event has landed yet - assertions on the event log must sync on
// the published count, not the job state.
func waitForEvents(t *testing.T, stub *stubBus, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(stub.snapshot()) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected at least %d published events within 2s, got %d", n, len(stub.snapshot()))
}

func TestStart_PublishesQueuedRunningSucceeded(t *testing.T) {
	stub := &stubBus{}
	tracker := NewTracker(stub, newTestLogger())

	id := tracker.Start(context.Background(), "vacuum", func(context.Context) (map[string]interface{}, error) {
		return map[string]interface{}{"reclaimed_bytes": 12345}, nil
	})

	job := waitForState(t, tracker, id, StateSucceeded)
	if job.Result["reclaimed_bytes"] != 12345 {
		t.Errorf("expected reclaimed_bytes in result, got %+v", job.Result)
	}

	waitForEvents(t, stub, 3)
	events := stub.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected at least 3 published events (queued/running/succeeded), got %d", len(events))
	}

	// Confirm the lifecycle in order. The implementation may publish more
	// than three (e.g. snapshots after each transition), so we only check
	// the prefix.
	wantStates := []State{StateQueued, StateRunning, StateSucceeded}
	for i, want := range wantStates {
		got := events[i].Data.(*Job)
		if got.State != want {
			t.Errorf("event[%d].state = %s, want %s", i, got.State, want)
		}
		if got.Kind != "vacuum" {
			t.Errorf("event[%d].kind = %s, want vacuum", i, got.Kind)
		}
	}
}

func TestStart_PublishesFailed(t *testing.T) {
	stub := &stubBus{}
	tracker := NewTracker(stub, newTestLogger())

	id := tracker.Start(context.Background(), "vacuum", func(context.Context) (map[string]interface{}, error) {
		return nil, errors.New("disk full")
	})

	job := waitForState(t, tracker, id, StateFailed)
	if job.Message != "disk full" {
		t.Errorf("expected message %q, got %q", "disk full", job.Message)
	}
	if job.EndedAt.IsZero() {
		t.Error("expected EndedAt to be set on failed job")
	}

	waitForEvents(t, stub, 3)
	events := stub.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}
	last := events[len(events)-1].Data.(*Job)
	if last.State != StateFailed {
		t.Errorf("last event state = %s, want failed", last.State)
	}
}

func TestGet_UnknownReturnsNil(t *testing.T) {
	tracker := NewTracker(&stubBus{}, newTestLogger())
	if got := tracker.Get("nonexistent"); got != nil {
		t.Errorf("expected nil for unknown id, got %+v", got)
	}
}

func TestList_ReturnsAllTrackedJobs(t *testing.T) {
	tracker := NewTracker(&stubBus{}, newTestLogger())
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ids = append(ids, tracker.Start(context.Background(), "noop", func(context.Context) (map[string]interface{}, error) {
			return nil, nil
		}))
	}
	for _, id := range ids {
		waitForState(t, tracker, id, StateSucceeded)
	}
	list := tracker.List()
	if len(list) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(list))
	}
}

func TestNilBus_DoesNotPanic(t *testing.T) {
	tracker := NewTracker(nil, newTestLogger())
	id := tracker.Start(context.Background(), "noop", func(context.Context) (map[string]interface{}, error) {
		return nil, nil
	})
	waitForState(t, tracker, id, StateSucceeded)
}
