package websocket

import (
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewHub(nil, log)
}

func newTestClient(id string) *Client {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return &Client{
		ID:                   id,
		send:                 make(chan []byte, 16),
		subscriptions:        map[string]bool{},
		sessionSubscriptions: map[string]bool{},
		sessionFocus:         map[string]bool{},
		userSubscriptions:    map[string]bool{},
		logger:               log,
	}
}

// modeRecorder collects mode transitions for assertion. Synchronised so tests
// can read what's been recorded so far without races.
type modeRecorder struct {
	mu     sync.Mutex
	events []modeEvent
	cond   *sync.Cond
}

type modeEvent struct {
	sessionID string
	mode      SessionMode
}

func newModeRecorder() *modeRecorder {
	r := &modeRecorder{}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *modeRecorder) listener() SessionModeListener {
	return func(sessionID string, mode SessionMode) {
		r.mu.Lock()
		r.events = append(r.events, modeEvent{sessionID, mode})
		r.cond.Broadcast()
		r.mu.Unlock()
	}
}

// waitForCount blocks until the recorder has at least n events or the timeout fires.
func (r *modeRecorder) waitForCount(n int, timeout time.Duration) []modeEvent {
	deadline := time.Now().Add(timeout)
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.events) < n {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		// Use a separate goroutine to wake cond on timeout.
		timer := time.AfterFunc(remaining, func() {
			r.mu.Lock()
			r.cond.Broadcast()
			r.mu.Unlock()
		})
		r.cond.Wait()
		timer.Stop()
	}
	out := make([]modeEvent, len(r.events))
	copy(out, r.events)
	return out
}

func TestHub_SubscribeFiresSlow(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	h.SubscribeToSession(c, "sess-1")

	got := rec.waitForCount(1, time.Second)
	if len(got) != 1 || got[0].sessionID != "sess-1" || got[0].mode != SessionModeSlow {
		t.Errorf("expected one slow event for sess-1, got %+v", got)
	}
}

func TestHub_FocusFiresFastImmediately(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	h.SubscribeToSession(c, "sess-1") // slow
	h.FocusSession(c, "sess-1")       // fast

	got := rec.waitForCount(2, time.Second)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 events, got %+v", got)
	}
	if got[1].mode != SessionModeFast {
		t.Errorf("expected second event to be fast, got %v", got[1])
	}
}

func TestHub_UnfocusDebouncesDownTransition(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")
	rec.waitForCount(2, time.Second)

	// Unfocus — should NOT immediately fire slow (debounce).
	h.UnfocusSession(c, "sess-1")
	time.Sleep(100 * time.Millisecond)
	if got := len(rec.events); got != 2 {
		t.Errorf("down-transition fired immediately; expected 2 events, got %d (%+v)", got, rec.events)
	}
}

func TestHub_RefocusWithinDebounceCancelsDownTransition(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")
	rec.waitForCount(2, time.Second)

	// Unfocus then re-focus quickly — net effect should be zero new events
	// (we stayed in fast).
	h.UnfocusSession(c, "sess-1")
	h.FocusSession(c, "sess-1")

	// Wait through and beyond the debounce window — no event should fire.
	time.Sleep(downTransitionDebounce + 200*time.Millisecond)

	if got := len(rec.events); got != 2 {
		t.Errorf("expected refocus to suppress down-transition; got %d events: %+v", got, rec.events)
	}
}

func TestHub_UnsubscribeFromOnlySubscriberFiresPaused(t *testing.T) {
	h := newTestHub(t)
	// Override debounce via a faster path: rely on the existing listener and
	// just wait the debounce period in the test.
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	h.SubscribeToSession(c, "sess-1") // slow
	rec.waitForCount(1, time.Second)

	h.UnsubscribeFromSession(c, "sess-1")
	// Wait debounce window plus margin.
	got := rec.waitForCount(2, downTransitionDebounce+1*time.Second)

	if len(got) < 2 {
		t.Fatalf("expected paused event after debounce, got %+v", got)
	}
	if got[1].mode != SessionModePaused {
		t.Errorf("second event mode = %v, want paused", got[1].mode)
	}
}

func TestHub_DisconnectCleansBothMaps(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c := newTestClient("c1")
	// Manually register the client (no real WS connection in tests).
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")
	rec.waitForCount(2, time.Second)

	h.removeClient(c)

	h.mu.RLock()
	subscriberCount := len(h.sessionSubscribers["sess-1"])
	focusCount := len(h.sessionMode.focusByClient["sess-1"])
	h.mu.RUnlock()

	if subscriberCount != 0 {
		t.Errorf("after disconnect subscriber count = %d, want 0", subscriberCount)
	}
	if focusCount != 0 {
		t.Errorf("after disconnect focus count = %d, want 0", focusCount)
	}
}

func TestHub_MultipleClientsKeepWorkspaceFast(t *testing.T) {
	h := newTestHub(t)
	rec := newModeRecorder()
	h.AddSessionModeListener(rec.listener())

	c1 := newTestClient("c1")
	c2 := newTestClient("c2")

	h.SubscribeToSession(c1, "sess-1")
	h.FocusSession(c1, "sess-1")
	h.SubscribeToSession(c2, "sess-1")
	h.FocusSession(c2, "sess-1")

	rec.waitForCount(2, time.Second) // slow then fast
	startCount := len(rec.events)

	// One client unfocuses — the other still has focus, so mode should stay fast.
	h.UnfocusSession(c1, "sess-1")

	// Wait through debounce. No new event should fire because mode didn't change.
	time.Sleep(downTransitionDebounce + 200*time.Millisecond)

	if got := len(rec.events); got != startCount {
		t.Errorf("expected no new events while another client still focused; got %d new (events: %+v)", got-startCount, rec.events)
	}
}

func TestIsUpTransition(t *testing.T) {
	cases := []struct {
		old, new SessionMode
		want     bool
	}{
		{SessionModePaused, SessionModeSlow, true},
		{SessionModePaused, SessionModeFast, true},
		{SessionModeSlow, SessionModeFast, true},
		{SessionModeFast, SessionModeSlow, false},
		{SessionModeSlow, SessionModePaused, false},
		{SessionModeFast, SessionModePaused, false},
		{SessionModeFast, SessionModeFast, false}, // not a transition
	}
	for _, tc := range cases {
		if got := isUpTransition(tc.old, tc.new); got != tc.want {
			t.Errorf("isUpTransition(%v, %v) = %v, want %v", tc.old, tc.new, got, tc.want)
		}
	}
}
