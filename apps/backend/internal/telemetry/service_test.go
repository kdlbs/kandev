package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func publish(t *testing.T, b bus.EventBus, subject string) {
	t.Helper()
	if err := b.Publish(context.Background(), subject, bus.NewEvent(subject, "test", nil)); err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
}

func sentNames(sink *fakeSink) map[string]int {
	names := map[string]int{}
	for _, e := range sink.sent() {
		names[e.Name]++
	}
	return names
}

// The off-path is the trust-critical path: nothing may reach the sink
// while consent is unasked or denied, or when the env kill switch is on.
func TestNoEmissionWhileUnaskedOrDenied(t *testing.T) {
	for _, status := range []ConsentStatus{ConsentUnasked, ConsentDenied} {
		eventBus := bus.NewMemoryEventBus(newTestLogger())
		svc, store, sink := newTestService(t, eventBus, Options{})
		if status == ConsentDenied {
			_ = store.Save(context.Background(), consentKey, []byte(ConsentDenied))
		}
		if err := svc.loadConsent(context.Background()); err != nil {
			t.Fatalf("loadConsent: %v", err)
		}
		svc.subscribeCollector()

		publish(t, eventBus, events.TaskCreated)
		svc.EnqueueUI([]UIEventSubmission{{Name: EventUIPageViewed, Properties: map[string]string{"page": "settings"}}})
		svc.flushOnce(context.Background())

		if got := sink.sent(); len(got) != 0 {
			t.Fatalf("status %q: expected zero events sent, got %d", status, len(got))
		}
	}
}

func TestEnvKillSwitchBlocksEvenWhenGranted(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, store, sink := newTestService(t, eventBus, Options{EnvDisabled: true})
	// Simulate a store that already carries a grant: env must still win.
	_ = store.Save(context.Background(), consentKey, []byte(ConsentGranted))
	_ = store.Save(context.Background(), installIDKey, []byte("11111111-1111-1111-1111-111111111111"))

	svc.Start(context.Background())
	defer svc.Stop()

	publish(t, eventBus, events.TaskCreated)
	svc.EnqueueUI([]UIEventSubmission{{Name: EventUIPageViewed, Properties: map[string]string{"page": "settings"}}})
	svc.flushOnce(context.Background())

	if got := sink.sent(); len(got) != 0 {
		t.Fatalf("env kill switch on: expected zero events sent, got %d", len(got))
	}
	if state := svc.Consent(); !state.EnvDisabled {
		t.Fatal("consent state must report env_disabled")
	}
}

func TestCollectorMapsOnlyAllowlistedBusEvents(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, _, sink := newTestService(t, eventBus, Options{})
	grantConsent(t, svc)
	svc.subscribeCollector()

	publish(t, eventBus, events.TaskCreated)
	publish(t, eventBus, events.AgentStarted)
	publish(t, eventBus, events.MessageAdded) // not allowlisted
	svc.flushOnce(context.Background())

	names := sentNames(sink)
	if names[EventTaskCreated] != 1 || names[EventAgentRunStarted] != 1 {
		t.Fatalf("expected mapped events, got %v", names)
	}
	for name := range names {
		switch name {
		case EventTaskCreated, EventAgentRunStarted, EventTelemetryEnabled, EventInstallHeartbeat:
		default:
			t.Fatalf("unexpected event %q sent (names: %v)", name, names)
		}
	}
}

func TestCollectorForwardsNothingFromPayload(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, _, sink := newTestService(t, eventBus, Options{})
	grantConsent(t, svc)
	svc.drainQueue() // discard the grant-time events for a clean slate
	svc.subscribeCollector()

	payload := map[string]any{"title": "SECRET TASK TITLE", "repository": "org/secret-repo"}
	if err := eventBus.Publish(context.Background(), events.TaskCreated,
		bus.NewEvent(events.TaskCreated, "test", payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	svc.flushOnce(context.Background())

	sent := sink.sent()
	if len(sent) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(sent))
	}
	for key, value := range sent[0].Properties {
		if value == "SECRET TASK TITLE" || value == "org/secret-repo" {
			t.Fatalf("payload leaked into property %q=%q", key, value)
		}
	}
}

// agent.failed is the one subject with allowlisted payload keys: the
// routingerr classification enums come through, everything else —
// including the free-text error message — must not.
func TestCollectorForwardsOnlyClassifiedFailureEnums(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, _, sink := newTestService(t, eventBus, Options{})
	grantConsent(t, svc)
	svc.drainQueue()
	svc.subscribeCollector()

	payload := map[string]any{
		"error_code":    "rate_limited",
		"error_phase":   "prompt_send",
		"error_message": "secret /home/user/repo stack trace",
		"task_id":       "some-task-uuid",
	}
	if err := eventBus.Publish(context.Background(), events.AgentFailed,
		bus.NewEvent(events.AgentFailed, "test", payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	svc.flushOnce(context.Background())

	sent := sink.sent()
	if len(sent) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(sent))
	}
	props := sent[0].Properties
	if props["error_code"] != "rate_limited" || props["error_phase"] != "prompt_send" {
		t.Fatalf("classification enums missing: %v", props)
	}
	for key, value := range props {
		if key == "error_message" || key == "task_id" || value == payload["error_message"] {
			t.Fatalf("non-allowlisted payload field leaked: %q=%q", key, value)
		}
	}
}

func TestCollectorDropsNonEnumFailureValues(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, _, sink := newTestService(t, eventBus, Options{})
	grantConsent(t, svc)
	svc.drainQueue()
	svc.subscribeCollector()

	payload := map[string]any{
		"error_code":  "Free Text With Spaces /and/paths",
		"error_phase": 42, // not even a string
	}
	if err := eventBus.Publish(context.Background(), events.AgentFailed,
		bus.NewEvent(events.AgentFailed, "test", payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	svc.flushOnce(context.Background())

	sent := sink.sent()
	if len(sent) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(sent))
	}
	if _, ok := sent[0].Properties["error_code"]; ok {
		t.Fatalf("non-enum error_code survived: %v", sent[0].Properties)
	}
	if _, ok := sent[0].Properties["error_phase"]; ok {
		t.Fatalf("non-string error_phase survived: %v", sent[0].Properties)
	}
}

func TestDenyDropsQueuedEvents(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{})
	grantConsent(t, svc) // queues telemetry_enabled + install_heartbeat
	if _, err := svc.SetConsent(context.Background(), ConsentDenied); err != nil {
		t.Fatalf("SetConsent(denied): %v", err)
	}
	svc.flushOnce(context.Background())
	if got := sink.sent(); len(got) != 0 {
		t.Fatalf("expected queue dropped on deny, got %d events", len(got))
	}
}

func TestFlushAttachesBasePropertiesAndInstallID(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{Version: "9.9.9", DeployMode: "docker"})
	grantConsent(t, svc)
	svc.flushOnce(context.Background())

	sent := sink.sent()
	if len(sent) == 0 {
		t.Fatal("expected grant-time events to flush")
	}
	for _, e := range sent {
		if e.Properties["app_version"] != "9.9.9" || e.Properties["deploy_mode"] != "docker" {
			t.Fatalf("missing base properties: %v", e.Properties)
		}
		if e.Properties["os"] == "" || e.Properties["arch"] == "" {
			t.Fatalf("missing os/arch: %v", e.Properties)
		}
	}
	installID := svc.Consent().InstallID
	for _, id := range sink.distinctIDs() {
		if id != installID {
			t.Fatalf("distinct id %q != install id %q", id, installID)
		}
	}
}

func TestEnqueueUIValidation(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{})
	grantConsent(t, svc)
	svc.drainQueue()

	accepted := svc.EnqueueUI([]UIEventSubmission{
		{Name: EventUIPageViewed, Properties: map[string]string{"page": "settings_system"}},
		{Name: EventUIAction, Properties: map[string]string{"action": "task.create:dialog"}},
		{Name: EventFeatureUsed, Properties: map[string]string{"feature": "office"}},
		{Name: "custom_event", Properties: map[string]string{"page": "x"}},                           // unknown name
		{Name: EventUIPageViewed, Properties: map[string]string{"page": "Has Spaces And Caps"}},      // free text
		{Name: EventUIPageViewed, Properties: map[string]string{"other": "settings"}},                // wrong key
		{Name: EventUIPageViewed, Properties: map[string]string{"page": "ok", "title": "user text"}}, // extra key stripped
	})
	if accepted != 4 {
		t.Fatalf("expected 4 accepted, got %d", accepted)
	}
	svc.flushOnce(context.Background())
	for _, e := range sink.sent() {
		if _, leaked := e.Properties["title"]; leaked {
			t.Fatalf("non-allowlisted property survived: %v", e.Properties)
		}
	}
}

func TestStartStopLifecycle(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	svc, _, sink := newTestService(t, eventBus, Options{
		FlushInterval:     time.Hour, // ticks never fire; Stop's final flush delivers
		HeartbeatInterval: time.Hour,
	})
	grantConsent(t, svc)
	svc.Start(context.Background())
	publish(t, eventBus, events.TurnCompleted)
	svc.Stop()
	svc.Stop() // idempotent

	names := sentNames(sink)
	if names[EventTurnCompleted] != 1 || names[EventInstallHeartbeat] == 0 {
		t.Fatalf("expected turn_completed and heartbeat after Stop flush, got %v", names)
	}

	// After Stop, collector must be detached.
	publish(t, eventBus, events.TurnCompleted)
	svc.flushOnce(context.Background())
	if got := sentNames(sink)[EventTurnCompleted]; got != 1 {
		t.Fatalf("collector still attached after Stop: %d turn_completed", got)
	}
}

func TestQueueOverflowDropsInsteadOfBlocking(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{QueueSize: 2, MaxBatch: 10})
	grantConsent(t, svc) // two grant-time events fill the queue
	done := make(chan struct{})
	go func() {
		svc.enqueue(Event{Name: EventTaskCreated}) // must drop, not block
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue blocked on full queue")
	}
	svc.flushOnce(context.Background())
	if got := len(sink.sent()); got != 2 {
		t.Fatalf("expected the 2 queued events only, got %d", got)
	}
}

func TestEnvDisabledDetection(t *testing.T) {
	cases := []struct {
		name, envVar, value string
		want                bool
	}{
		{"unset", "", "", false},
		{"do_not_track_1", "DO_NOT_TRACK", "1", true},
		{"do_not_track_true", "DO_NOT_TRACK", "true", true},
		{"do_not_track_0", "DO_NOT_TRACK", "0", false},
		{"e2e_mode", "KANDEV_E2E_MOCK", "true", true},
		{"e2e_selector_unset_value", "KANDEV_E2E_MOCK", "false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DO_NOT_TRACK", "")
			t.Setenv("KANDEV_E2E_MOCK", "")
			if tc.envVar != "" {
				t.Setenv(tc.envVar, tc.value)
			}
			if got := EnvDisabled(); got != tc.want {
				t.Fatalf("EnvDisabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
