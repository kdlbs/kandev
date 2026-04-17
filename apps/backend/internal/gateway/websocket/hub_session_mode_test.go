package websocket

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	h := NewHub(nil, log)
	// Short debounce so tests don't block for 5+ real seconds.
	h.setDebounceForTest(50 * time.Millisecond)
	// Cancel any pending debounce timers when the test exits — otherwise the
	// timer can fire after t.Run returns and race the next test.
	t.Cleanup(func() {
		h.stopAllPendingTransitions()
	})
	return h
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
	synctest.Test(t, func(t *testing.T) {
		h := newTestHub(t)
		rec := newModeRecorder()
		h.AddSessionModeListener(rec.listener())

		c := newTestClient("c1")
		h.SubscribeToSession(c, "sess-1")
		h.FocusSession(c, "sess-1")

		// Let all goroutines settle so the two synchronous listener calls complete.
		synctest.Wait()

		rec.mu.Lock()
		if len(rec.events) != 2 {
			t.Fatalf("setup: expected 2 events, got %d", len(rec.events))
		}
		rec.mu.Unlock()

		// Unfocus — schedules a debounced down-transition timer. synctest's fake
		// clock doesn't advance until we ask, so the timer cannot fire.
		h.UnfocusSession(c, "sess-1")
		synctest.Wait()

		rec.mu.Lock()
		gotCount := len(rec.events)
		rec.mu.Unlock()
		if gotCount != 2 {
			t.Errorf("down-transition fired immediately; expected 2 events, got %d (%+v)", gotCount, rec.events)
		}
	})
}

func TestHub_RefocusWithinDebounceCancelsDownTransition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestHub(t)
		rec := newModeRecorder()
		h.AddSessionModeListener(rec.listener())

		c := newTestClient("c1")
		h.SubscribeToSession(c, "sess-1")
		h.FocusSession(c, "sess-1")
		synctest.Wait()

		rec.mu.Lock()
		if len(rec.events) != 2 {
			t.Fatalf("setup: expected 2 events, got %d", len(rec.events))
		}
		rec.mu.Unlock()

		// Unfocus then re-focus quickly — cancelling the pending down-transition
		// timer clears lastMode, so the re-focus fires a fresh up-transition
		// (3rd event: fast). This is intentional: it ensures new clients after a
		// page refresh receive the current mode immediately.
		h.UnfocusSession(c, "sess-1")
		h.FocusSession(c, "sess-1")
		synctest.Wait()

		// Advance fake clock past the debounce window to prove the timer was
		// cancelled and won't fire a down-transition.
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		rec.mu.Lock()
		got := rec.events
		rec.mu.Unlock()
		if len(got) != 3 {
			t.Errorf("expected 3 events (slow, fast, fast); got %d: %+v", len(got), got)
		}
		if len(got) == 3 && got[2].mode != SessionModeFast {
			t.Errorf("expected 3rd event to be fast (re-focus); got %v", got[2].mode)
		}
	})
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
	// Wait test debounce (50ms) plus margin.
	got := rec.waitForCount(2, 1*time.Second)

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
	synctest.Test(t, func(t *testing.T) {
		h := newTestHub(t)
		rec := newModeRecorder()
		h.AddSessionModeListener(rec.listener())

		c1 := newTestClient("c1")
		c2 := newTestClient("c2")

		h.SubscribeToSession(c1, "sess-1")
		h.FocusSession(c1, "sess-1")
		h.SubscribeToSession(c2, "sess-1")
		h.FocusSession(c2, "sess-1")
		synctest.Wait()

		rec.mu.Lock()
		startCount := len(rec.events)
		rec.mu.Unlock()

		// One client unfocuses — the other still has focus, so mode should stay fast.
		h.UnfocusSession(c1, "sess-1")
		synctest.Wait()

		// Advance fake clock past the debounce window.
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		rec.mu.Lock()
		got := len(rec.events)
		rec.mu.Unlock()
		if got != startCount {
			t.Errorf("expected no new events while another client still focused; got %d new (events: %+v)", got-startCount, rec.events)
		}
	})
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
