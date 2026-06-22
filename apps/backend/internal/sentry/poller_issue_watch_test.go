package sentry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// recordingSubscriber subscribes to SentryNewIssue on the in-memory bus and
// captures payloads so the test can assert on them without touching internals.
type recordingSubscriber struct {
	mu      sync.Mutex
	payload []*NewSentryIssueEvent
	done    chan struct{}
	want    int
}

func newRecordingSubscriber(eb bus.EventBus, want int) (*recordingSubscriber, error) {
	r := &recordingSubscriber{done: make(chan struct{}), want: want}
	_, err := eb.Subscribe(events.SentryNewIssue, func(_ context.Context, e *bus.Event) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		evt, ok := e.Data.(*NewSentryIssueEvent)
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

func (r *recordingSubscriber) snapshot() []*NewSentryIssueEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*NewSentryIssueEvent, len(r.payload))
	copy(out, r.payload)
	return out
}

func TestPoller_CheckIssueWatches_PublishesNewIssuesOnly(t *testing.T) {
	f := newPollerFixture(t)
	ctx := context.Background()
	instanceID := f.saveConfig(t, "tok")

	eb := bus.NewMemoryEventBus(logger.Default())
	defer eb.Close()
	f.svc.SetEventBus(eb)

	sub, err := newRecordingSubscriber(eb, 2)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	w := newTestIssueWatch("ws-1")
	w.InstanceID = instanceID
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	f.client.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		return &SearchResult{
			Issues: []SentryIssue{
				{ShortID: "PROJ-1", Title: "first", Permalink: "https://sentry.io/issues/PROJ-1"},
				{ShortID: "PROJ-2", Title: "second", Permalink: "https://sentry.io/issues/PROJ-2"},
			},
			IsLast: true,
		}, nil
	}

	// First tick: both issues are new, two events publish.
	f.poller.checkIssueWatches(ctx)

	<-sub.done
	first := sub.snapshot()
	if len(first) != 2 {
		t.Fatalf("expected 2 events on first tick, got %d", len(first))
	}
	ids := []string{first[0].Issue.ShortID, first[1].Issue.ShortID}
	if !contains(ids, "PROJ-1") || !contains(ids, "PROJ-2") {
		t.Errorf("expected events for PROJ-1 and PROJ-2, got %v", ids)
	}
	if first[0].WorkspaceID != "ws-1" || first[0].WorkflowID != "wf-1" {
		t.Errorf("event missing watch context: %+v", first[0])
	}

	// Reserve both to simulate the orchestrator having created tasks.
	for _, id := range []string{"PROJ-1", "PROJ-2"} {
		if _, err := f.store.ReserveIssueWatchTask(ctx, w.ID, id, "https://sentry.io/issues/"+id); err != nil {
			t.Fatalf("reserve %s: %v", id, err)
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
	instanceID := f.saveConfig(t, "tok")

	eb := bus.NewMemoryEventBus(logger.Default())
	defer eb.Close()
	f.svc.SetEventBus(eb)

	disabled := newTestIssueWatch("ws-1")
	disabled.InstanceID = instanceID
	disabled.Enabled = false
	if err := f.store.CreateIssueWatch(ctx, disabled); err != nil {
		t.Fatalf("create disabled: %v", err)
	}

	calls := 0
	f.client.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		calls++
		return &SearchResult{Issues: []SentryIssue{{ShortID: "PROJ-1"}}, IsLast: true}, nil
	}

	f.poller.checkIssueWatches(ctx)

	if calls != 0 {
		t.Errorf("expected disabled watch to be skipped (no Sentry call), got %d calls", calls)
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

func TestIsIssueWatchDue(t *testing.T) {
	now := time.Now()
	stamp := now.Add(-30 * time.Second)
	cases := []struct {
		name     string
		watch    *IssueWatch
		expected bool
	}{
		{
			name:     "never polled is always due",
			watch:    &IssueWatch{PollIntervalSeconds: 300},
			expected: true,
		},
		{
			name:     "polled less than interval ago is not due",
			watch:    &IssueWatch{PollIntervalSeconds: 300, LastPolledAt: &stamp},
			expected: false,
		},
		{
			name:     "polled at exactly the interval boundary is due",
			watch:    &IssueWatch{PollIntervalSeconds: 30, LastPolledAt: &stamp},
			expected: true,
		},
		{
			name:     "zero interval falls back to default and gates accordingly",
			watch:    &IssueWatch{PollIntervalSeconds: 0, LastPolledAt: &stamp},
			expected: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isIssueWatchDue(tc.watch, now); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestPoller_CheckIssueWatches_RespectsPerWatchInterval(t *testing.T) {
	f := newPollerFixture(t)
	ctx := context.Background()
	instanceID := f.saveConfig(t, "tok")

	eb := bus.NewMemoryEventBus(logger.Default())
	defer eb.Close()
	f.svc.SetEventBus(eb)

	w := newTestIssueWatch("ws-1")
	w.InstanceID = instanceID
	w.PollIntervalSeconds = 300
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now().UTC()
	if err := f.store.UpdateIssueWatchLastPolled(ctx, w.ID, now); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	calls := 0
	f.client.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		calls++
		return &SearchResult{Issues: []SentryIssue{{ShortID: "PROJ-1"}}, IsLast: true}, nil
	}

	f.poller.checkIssueWatches(ctx)

	if calls != 0 {
		t.Errorf("expected gating to skip the Sentry search, got %d call(s)", calls)
	}
}
