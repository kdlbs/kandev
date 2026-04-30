package jira

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// recordingSubscriber subscribes to JiraNewIssue on the in-memory bus and
// captures payloads so the test can assert on them without touching internals.
type recordingSubscriber struct {
	mu      sync.Mutex
	payload []*NewJiraIssueEvent
	done    chan struct{} // closed once `expected` events have arrived.
	want    int
}

func newRecordingSubscriber(eb bus.EventBus, want int) (*recordingSubscriber, error) {
	r := &recordingSubscriber{done: make(chan struct{}), want: want}
	_, err := eb.Subscribe(events.JiraNewIssue, func(_ context.Context, e *bus.Event) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		evt, ok := e.Data.(*NewJiraIssueEvent)
		if !ok {
			return nil
		}
		r.payload = append(r.payload, evt)
		if r.want > 0 && len(r.payload) == r.want {
			close(r.done)
		}
		return nil
	})
	return r, err
}

func (r *recordingSubscriber) snapshot() []*NewJiraIssueEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*NewJiraIssueEvent, len(r.payload))
	copy(out, r.payload)
	return out
}

func TestPoller_CheckIssueWatches_PublishesNewTicketsOnly(t *testing.T) {
	f := newPollerFixture(t)
	ctx := context.Background()
	f.saveConfig(t, "ws-1", "tok")

	eb := bus.NewMemoryEventBus(logger.Default())
	defer eb.Close()
	f.svc.SetEventBus(eb)

	sub, err := newRecordingSubscriber(eb, 2)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	w := newTestIssueWatch("ws-1")
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	f.client.searchFn = func(_ string) (*SearchResult, error) {
		return &SearchResult{
			Tickets: []JiraTicket{
				{Key: "PROJ-1", Summary: "first", URL: "https://x/browse/PROJ-1"},
				{Key: "PROJ-2", Summary: "second", URL: "https://x/browse/PROJ-2"},
			},
			IsLast: true,
		}, nil
	}

	// First tick: both tickets are new, so two events publish.
	f.poller.checkIssueWatches(ctx)

	<-sub.done // wait for both events to arrive on the in-memory bus
	first := sub.snapshot()
	if len(first) != 2 {
		t.Fatalf("expected 2 events on first tick, got %d", len(first))
	}
	keys := []string{first[0].Issue.Key, first[1].Issue.Key}
	if !contains(keys, "PROJ-1") || !contains(keys, "PROJ-2") {
		t.Errorf("expected events for PROJ-1 and PROJ-2, got %v", keys)
	}
	if first[0].WorkspaceID != "ws-1" || first[0].WorkflowID != "wf-1" {
		t.Errorf("event missing watch context: %+v", first[0])
	}

	// Reserve both keys to simulate the orchestrator having created tasks.
	for _, key := range []string{"PROJ-1", "PROJ-2"} {
		if _, err := f.store.ReserveIssueWatchTask(ctx, w.ID, key, "https://x/browse/"+key); err != nil {
			t.Fatalf("reserve %s: %v", key, err)
		}
	}

	// Second tick: nothing new, no additional events should publish.
	f.poller.checkIssueWatches(ctx)
	if got := len(sub.snapshot()); got != 2 {
		t.Errorf("expected event count to stay at 2 after dedup, got %d", got)
	}
}

func TestPoller_CheckIssueWatches_SkipsDisabled(t *testing.T) {
	f := newPollerFixture(t)
	ctx := context.Background()
	f.saveConfig(t, "ws-1", "tok")

	eb := bus.NewMemoryEventBus(logger.Default())
	defer eb.Close()
	f.svc.SetEventBus(eb)

	disabled := newTestIssueWatch("ws-1")
	disabled.Enabled = false
	if err := f.store.CreateIssueWatch(ctx, disabled); err != nil {
		t.Fatalf("create disabled: %v", err)
	}

	calls := 0
	f.client.searchFn = func(_ string) (*SearchResult, error) {
		calls++
		return &SearchResult{Tickets: []JiraTicket{{Key: "X-1"}}, IsLast: true}, nil
	}

	f.poller.checkIssueWatches(ctx)

	if calls != 0 {
		t.Errorf("expected disabled watch to be skipped (no JIRA call), got %d calls", calls)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
