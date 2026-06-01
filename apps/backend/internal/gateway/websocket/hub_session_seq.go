package websocket

import (
	"sync/atomic"
)

// nextSessionSeq returns the next monotonic session_seq for sessionID. If no
// counter exists yet, one is created lazily (race-free via sync.Map.LoadOrStore).
// The returned counter starts at 1 for the first stamped event.
//
// Called from stampAndMarshalForSession on every outbound envelope that is
// routed to a specific session (BroadcastToSession and the session-routed
// fan-out paths).
// A connection-wide notification (no sessionID at stamp time) gets a zero
// SessionSeq, which is correctly omitted from the JSON wire format.
func (h *Hub) nextSessionSeq(sessionID string) int64 {
	if sessionID == "" {
		return 0
	}
	v, ok := h.sessionSeqs.Load(sessionID)
	if !ok {
		v, _ = h.sessionSeqs.LoadOrStore(sessionID, &atomic.Int64{})
	}
	return v.(*atomic.Int64).Add(1)
}

// incSessionSubscribers bumps the per-session subscriber count and eagerly
// creates the matching session_seq counter so the lifecycle invariant
// (subscriberCount > 0 ⇒ sessionSeqs has an entry) holds even before any
// event is broadcast on the session. Called by SubscribeToSession after the
// client is added to sessionSubscribers.
func (h *Hub) incSessionSubscribers(sessionID string) {
	if sessionID == "" {
		return
	}
	v, ok := h.sessionSubscriberCounts.Load(sessionID)
	if !ok {
		v, _ = h.sessionSubscriberCounts.LoadOrStore(sessionID, &atomic.Int64{})
	}
	v.(*atomic.Int64).Add(1)
	// Eagerly create the per-session counter so a subscribe immediately
	// reflects in sessionSeqs. Without this, the test for
	// "subscribe → disconnect drains both maps" would race a lazy create
	// against the immediate decrement on disconnect.
	if _, exists := h.sessionSeqs.Load(sessionID); !exists {
		h.sessionSeqs.LoadOrStore(sessionID, &atomic.Int64{})
	}
}

// decSessionSubscribers decrements the per-session subscriber count. When it
// reaches zero, the counter entry AND the matching session_seq counter are
// deleted so a long-lived hub doesn't accumulate one *atomic.Int64 per session
// ever seen. Called by UnsubscribeFromSession and by removeClient's cleanup
// loop (a client disconnecting without an explicit unsubscribe).
func (h *Hub) decSessionSubscribers(sessionID string) {
	if sessionID == "" {
		return
	}
	v, ok := h.sessionSubscriberCounts.Load(sessionID)
	if !ok {
		return
	}
	n := v.(*atomic.Int64).Add(-1)
	if n <= 0 {
		h.sessionSubscriberCounts.Delete(sessionID)
		h.sessionSeqs.Delete(sessionID)
	}
}

// sessionSeqCountForTest returns the number of live per-session counters.
// Test-only helper used by the lifecycle regression test.
func (h *Hub) sessionSeqCountForTest() int {
	n := 0
	h.sessionSeqs.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// sessionSubscriberCountForTest returns the number of live subscriber-count
// entries. Test-only helper paired with sessionSeqCountForTest to confirm
// both maps drain together.
func (h *Hub) sessionSubscriberCountForTest() int {
	n := 0
	h.sessionSubscriberCounts.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
