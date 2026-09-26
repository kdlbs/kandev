package webapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.2 AC-PLUGINS-ISOLATED-WEB-APPS-006.3 AC-PLUGINS-ISOLATED-WEB-APPS-006.4
func TestEventHubReplaysAuthorizedEventsAfterLastEventID(t *testing.T) {
	hub := NewEventHub(EventHubConfig{Generation: "generation-1", SubscriberQueueSize: 4})
	t.Cleanup(func() { hub.Close() })

	first := publishEvent(t, hub, EventInput{
		Type:  "task.updated",
		Scope: EventScope{TaskID: "task-1"},
		Data:  map[string]string{"state": "started"},
	})
	publishEvent(t, hub, EventInput{
		Type:  "task.updated",
		Scope: EventScope{TaskID: "task-2"},
		Data:  map[string]string{"state": "hidden"},
	})
	third := publishEvent(t, hub, EventInput{
		Type:  "task.updated",
		Scope: EventScope{TaskID: "task-1"},
		Data:  map[string]string{"state": "completed"},
	})

	sub, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{
		InstanceID:  "instance-1",
		UserID:      "user-1",
		LastEventID: first.ID,
		Filter: func(event Event) bool {
			return event.Scope.TaskID == "task-1"
		},
	})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	got := receiveEvent(t, sub)
	if got.ID != third.ID || got.Scope.InstanceID != "instance-1" {
		t.Fatalf("replayed event = %+v, want id %q for instance-1", got, third.ID)
	}
	if got.Type != "task.updated" || string(got.Data) != `{"state":"completed"}` {
		t.Fatalf("replayed event payload = %+v", got)
	}
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.1 AC-PLUGINS-ISOLATED-WEB-APPS-006.3
func TestEventHubHandlerStreamsReplayAsSSE(t *testing.T) {
	hub := NewEventHub(EventHubConfig{
		Generation:          "generation-1",
		HeartbeatInterval:   time.Hour,
		StreamLifetime:      time.Minute,
		SubscriberQueueSize: 4,
	})
	t.Cleanup(func() { hub.Close() })

	first := publishEvent(t, hub, EventInput{Type: "state.updated", Data: map[string]string{"value": "old"}})
	second := publishEvent(t, hub, EventInput{Type: "state.updated", Data: map[string]string{"value": "new"}})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	server := httptest.NewServer(hub.Handler(EventSubscriptionRequest{
		InstanceID: "instance-1",
		UserID:     "user-1",
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/_kandev/v1/events", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() unexpected error: %v", err)
	}
	request.Header.Set("Last-Event-ID", first.ID)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("event request failed: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", got)
	}

	streamEvent := receiveSSEEvent(t, bufio.NewReader(response.Body))
	if streamEvent["id"] != second.ID || streamEvent["event"] != second.Type {
		t.Fatalf("SSE event = %+v, want id %q and type %q", streamEvent, second.ID, second.Type)
	}
	var payload Event
	if err := json.Unmarshal([]byte(streamEvent["data"]), &payload); err != nil {
		t.Fatalf("SSE data is not an event: %v", err)
	}
	if payload.Scope.InstanceID != "instance-1" || string(payload.Data) != `{"value":"new"}` {
		t.Fatalf("SSE payload = %+v", payload)
	}
	cancel()
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.3
func TestEventHubSendsResyncForHistoryGapAndGenerationMismatch(t *testing.T) {
	tests := []struct {
		name       string
		lastID     string
		publish    int
		wantReason string
	}{
		{name: "history gap", lastID: "generation-1:1", publish: 4, wantReason: ResyncReasonHistoryGap},
		{name: "generation mismatch", lastID: "old-generation:1", publish: 1, wantReason: ResyncReasonGenerationMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewEventHub(EventHubConfig{
				Generation:          "generation-1",
				MaxEvents:           2,
				SubscriberQueueSize: 4,
			})
			t.Cleanup(func() { hub.Close() })
			var last Event
			for range tc.publish {
				last = publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"ok": "true"}})
			}
			if tc.name == "generation mismatch" && last.ID == "" {
				t.Fatal("expected a published event")
			}

			sub, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{
				InstanceID:  "instance-1",
				UserID:      "user-1",
				LastEventID: tc.lastID,
			})
			if err != nil {
				t.Fatalf("Subscribe() unexpected error: %v", err)
			}
			t.Cleanup(func() { _ = sub.Close() })

			got := receiveEvent(t, sub)
			if got.Type != RuntimeResyncRequired || got.ID != "" {
				t.Fatalf("resync event = %+v", got)
			}
			var resync ResyncInfo
			if err := json.Unmarshal(got.Data, &resync); err != nil {
				t.Fatalf("resync payload is not JSON: %v", err)
			}
			if resync.Reason != tc.wantReason || resync.Generation != "generation-1" || !resync.Reset {
				t.Fatalf("resync payload = %+v, want reason %q and reset", resync, tc.wantReason)
			}
		})
	}
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.5 AC-PLUGINS-ISOLATED-WEB-APPS-006.6
func TestEventHubExpiresReplayHistory(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	hub := NewEventHub(EventHubConfig{
		Generation:          "generation-1",
		ReplayWindow:        5 * time.Minute,
		Now:                 func() time.Time { return now },
		SubscriberQueueSize: 4,
	})
	t.Cleanup(func() { hub.Close() })

	old := publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"value": "old"}})
	publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"value": "expired-too"}})
	now = now.Add(6 * time.Minute)
	publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"value": "fresh-1"}})
	publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"value": "fresh-2"}})

	sub, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{
		InstanceID:  "instance-1",
		UserID:      "user-1",
		LastEventID: old.ID,
	})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	got := receiveEvent(t, sub)
	if got.Type != RuntimeResyncRequired {
		t.Fatalf("expired history event = %+v, want resync", got)
	}
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.6
func TestEventHubHandlerFlushesHeadersAndSendsHeartbeat(t *testing.T) {
	hub := NewEventHub(EventHubConfig{
		Generation:        "generation-1",
		HeartbeatInterval: 20 * time.Millisecond,
		StreamLifetime:    time.Minute,
	})
	t.Cleanup(func() { hub.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(hub.Handler(EventSubscriptionRequest{
		InstanceID: "instance-1",
		UserID:     "user-1",
	}))
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() unexpected error: %v", err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("event request failed: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })

	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("heartbeat read failed: %v", err)
	}
	if strings.TrimSpace(line) != ": heartbeat" {
		t.Fatalf("heartbeat line = %q, want ': heartbeat'", line)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("heartbeat terminator read failed: %v", err)
	}
	if strings.TrimSpace(line) != "" {
		t.Fatalf("heartbeat terminator = %q, want blank line", line)
	}
	cancel()
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.6
func TestEventHubCancellationReleasesConnectionAdmission(t *testing.T) {
	hub := NewEventHub(EventHubConfig{
		MaxStreamsPerUser:         1,
		MaxStreamsPerUserInstance: 1,
	})
	t.Cleanup(func() { hub.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	first, err := hub.Subscribe(ctx, EventSubscriptionRequest{InstanceID: "instance-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("first Subscribe() unexpected error: %v", err)
	}
	cancel()
	waitForClosed(t, first.Done())

	second, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{InstanceID: "instance-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("second Subscribe() after cancellation: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close() unexpected error: %v", err)
	}
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.6
func TestEventHubDisconnectsSlowSubscriberWithoutBlockingPublish(t *testing.T) {
	hub := NewEventHub(EventHubConfig{
		SubscriberQueueSize:       1,
		MaxStreamsPerUser:         1,
		MaxStreamsPerUserInstance: 1,
	})
	t.Cleanup(func() { hub.Close() })

	sub, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{InstanceID: "instance-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"n": "1"}})

	finished := make(chan struct{})
	go func() {
		publishEvent(t, hub, EventInput{Type: "task.updated", Data: map[string]string{"n": "2"}})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Publish() blocked on a full subscriber queue")
	}
	if !errors.Is(sub.Err(), ErrSlowSubscriber) {
		t.Fatalf("subscriber error = %v, want ErrSlowSubscriber", sub.Err())
	}
	waitForClosed(t, sub.Done())

	replacement, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{InstanceID: "instance-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("replacement Subscribe() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
}

// @covers AC-PLUGINS-ISOLATED-WEB-APPS-006.2 AC-PLUGINS-ISOLATED-WEB-APPS-006.6
func TestEventHubConcurrentPublishSubscribeAndClose(t *testing.T) {
	hub := NewEventHub(EventHubConfig{
		Generation:          "generation-1",
		MaxEvents:           1000,
		SubscriberQueueSize: 128,
		MaxStreamsPerUser:   64,
	})

	const publishers = 8
	const eventsPerPublisher = 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make(map[string]struct{}, publishers*eventsPerPublisher)
	start := make(chan struct{})
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < eventsPerPublisher; j++ {
				event, err := hub.Publish("instance-1", EventInput{Type: "task.updated", Data: map[string]int{"n": j}})
				if err != nil {
					if errors.Is(err, ErrEventHubClosed) {
						return
					}
					t.Errorf("Publish() unexpected error: %v", err)
					return
				}
				mu.Lock()
				if _, exists := ids[event.ID]; exists {
					t.Errorf("duplicate event id %q", event.ID)
				}
				ids[event.ID] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	close(start)
	var subscriberWG sync.WaitGroup
	subscriberWG.Add(1)
	go func() {
		defer subscriberWG.Done()
		for i := 0; i < 16; i++ {
			sub, err := hub.Subscribe(context.Background(), EventSubscriptionRequest{
				InstanceID: "instance-1",
				UserID:     "user-1",
			})
			if err == nil {
				_ = sub.Close()
			}
		}
	}()
	wg.Wait()
	subscriberWG.Wait()
	hub.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(ids) != publishers*eventsPerPublisher {
		t.Fatalf("published event count = %d, want %d", len(ids), publishers*eventsPerPublisher)
	}
	sequences := make([]uint64, 0, len(ids))
	for id := range ids {
		_, sequence, ok := ParseEventID(id)
		if !ok {
			t.Fatalf("event id %q is not parseable", id)
		}
		sequences = append(sequences, sequence)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for i, sequence := range sequences {
		if sequence != uint64(i+1) {
			t.Fatalf("event sequence[%d] = %d, want %d", i, sequence, i+1)
		}
	}
}

func publishEvent(t *testing.T, hub *EventHub, input EventInput) Event {
	t.Helper()
	event, err := hub.Publish("instance-1", input)
	if err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	return event
}

func receiveEvent(t *testing.T, sub *EventSubscription) Event {
	t.Helper()
	select {
	case event, ok := <-sub.Events():
		if !ok {
			t.Fatalf("subscription closed with error %v", sub.Err())
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func waitForClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscription did not close")
	}
}

func receiveSSEEvent(t *testing.T, reader *bufio.Reader) map[string]string {
	t.Helper()
	fields := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			return fields
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
		}
		if ok {
			fields[key] = value
		}
	}
}
