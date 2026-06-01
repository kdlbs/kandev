package websocket

import (
	"encoding/json"
	"sync"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// subscribeClientLockedForTest registers a client on a sessionSubscribers
// entry without touching focus/mode bookkeeping. The session_seq lifecycle
// only depends on the subscription, not on focus.
func subscribeClientForTest(t *testing.T, h *Hub, c *Client, sessionID string) {
	t.Helper()
	c.hub = h
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	h.SubscribeToSession(c, sessionID)
}

// TestHub_BroadcastToSession_StampsIndependentSessionSeqs is the core
// per-session sequencing contract. Two sessions broadcasting concurrently to
// the same client must produce two strictly monotonic SessionSeq streams that
// are independent of each other — interleaving per-connection seq, monotonic
// per-session seq per session.
func TestHub_BroadcastToSession_StampsIndependentSessionSeqs(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-multi")
	c.send = make(chan []byte, 64)
	subscribeClientForTest(t, h, c, "sess-A")
	subscribeClientForTest(t, h, c, "sess-B")

	// Interleave broadcasts across A and B.
	for i := range 5 {
		msgA, _ := ws.NewNotification("a.evt", map[string]int{"i": i})
		h.BroadcastToSession("sess-A", msgA)
		msgB, _ := ws.NewNotification("b.evt", map[string]int{"i": i})
		h.BroadcastToSession("sess-B", msgB)
	}

	// Read all 10 frames off the send channel and group by session.
	type framed struct {
		seq        int64
		sessionSeq int64
		action     string
	}
	frames := make([]framed, 0, 10)
	for range 10 {
		raw := <-c.send
		var m ws.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// SessionID isn't on the envelope wire format — derive it from the
		// authoritative ring buffer for the assertion below.
		frames = append(frames, framed{
			seq:        m.Seq,
			sessionSeq: m.SessionSeq,
			action:     m.Action,
		})
	}

	// Verify per-connection seq is strictly monotonic across BOTH sessions
	// (1..10) — this is the per-connection invariant from Phase 1.
	for i, f := range frames {
		want := int64(i + 1)
		if f.seq != want {
			t.Errorf("frame %d: connection seq=%d, want %d", i, f.seq, want)
		}
		if f.sessionSeq <= 0 {
			t.Errorf("frame %d: SessionSeq=%d, want > 0 (session-routed event)", i, f.sessionSeq)
		}
	}

	// Cross-check against the ring buffer: per-session streams must be 1..5
	// for both A and B independently.
	for _, sid := range []string{"sess-A", "sess-B"} {
		entries, maxSessionSeq, ok := h.GetSentEventsForSession(c.ID, sid)
		if !ok {
			t.Fatalf("session %q: GetSentEventsForSession ok=false", sid)
		}
		if maxSessionSeq != 5 {
			t.Errorf("session %q: maxSessionSeq=%d, want 5", sid, maxSessionSeq)
		}
		if len(entries) != 5 {
			t.Fatalf("session %q: len(entries)=%d, want 5", sid, len(entries))
		}
		for i, e := range entries {
			wantSeq := int64(i + 1)
			if e.SessionSeq != wantSeq {
				t.Errorf("session %q entries[%d].SessionSeq=%d, want %d", sid, i, e.SessionSeq, wantSeq)
			}
			if e.SessionID != sid {
				t.Errorf("session %q entries[%d].SessionID=%q, want %q", sid, i, e.SessionID, sid)
			}
		}
	}
}

// TestHub_BroadcastToSession_SharedCounterAcrossFanout covers the
// hub-level-counter contract: the session_seq counter lives on the Hub, not
// on the client, so each fan-out clone consumes a distinct SessionSeq value.
// With two clients both subscribed to the same session, a single broadcast
// produces two stamped frames whose SessionSeqs span the next two counter
// values. (Per-(connection, session) gap detection on the FE is the primary
// design driver — multi-subscriber per-session is not a guaranteed-monotonic
// case from a single client's perspective, and that's acceptable: production
// has one tab per user per session.)
func TestHub_BroadcastToSession_SharedCounterAcrossFanout(t *testing.T) {
	h := newTestHub(t)
	a := newTestClient("ca")
	b := newTestClient("cb")
	a.send = make(chan []byte, 4)
	b.send = make(chan []byte, 4)
	subscribeClientForTest(t, h, a, "sess-shared")
	subscribeClientForTest(t, h, b, "sess-shared")

	msg, _ := ws.NewNotification("shared.evt", nil)
	h.BroadcastToSession("sess-shared", msg)

	for _, c := range []*Client{a, b} {
		entries, _, ok := h.GetSentEventsForSession(c.ID, "sess-shared")
		if !ok || len(entries) == 0 {
			t.Fatalf("client %s: no entries", c.ID)
		}
	}

	// The shared counter is at 2 (one Add per fan-out client).
	v, ok := h.sessionSeqs.Load("sess-shared")
	if !ok {
		t.Fatal("session_seq counter not created")
	}
	if got := v.(interface{ Load() int64 }).Load(); got != 2 {
		t.Errorf("session_seq counter=%d, want 2 (one Add per fan-out client)", got)
	}
}

// TestHub_SessionSeq_CounterLifecycle is the leak-prevention regression test
// the plan calls out: 1000 subscribe/unsubscribe cycles must leave both the
// session_seq counter map AND the subscriber-count map at len 0.
func TestHub_SessionSeq_CounterLifecycle(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-lifecycle")
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	c.hub = h

	const cycles = 1000
	for i := range cycles {
		// Use a unique sessionID per cycle so a stale entry from one cycle
		// can't masquerade as the next cycle's entry.
		sid := "sess-cycle-" + intToStr(i)
		h.SubscribeToSession(c, sid)
		h.UnsubscribeFromSession(c, sid)
	}

	if got := h.sessionSeqCountForTest(); got != 0 {
		t.Errorf("sessionSeqs leaked: len=%d, want 0", got)
	}
	if got := h.sessionSubscriberCountForTest(); got != 0 {
		t.Errorf("sessionSubscriberCounts leaked: len=%d, want 0", got)
	}
}

// TestHub_SessionSeq_CounterDeletedOnDisconnect covers the implicit-cleanup
// path: a client that disconnects without explicit session.unsubscribe still
// drains the lifecycle maps via removeClient.
func TestHub_SessionSeq_CounterDeletedOnDisconnect(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-disconnect")
	c.send = make(chan []byte, 8)
	subscribeClientForTest(t, h, c, "sess-disco")

	if got := h.sessionSeqCountForTest(); got != 1 {
		t.Errorf("after subscribe sessionSeqs len=%d, want 1", got)
	}

	h.removeClient(c)

	if got := h.sessionSeqCountForTest(); got != 0 {
		t.Errorf("after disconnect sessionSeqs len=%d, want 0", got)
	}
	if got := h.sessionSubscriberCountForTest(); got != 0 {
		t.Errorf("after disconnect sessionSubscriberCounts len=%d, want 0", got)
	}
}

// TestHub_SessionSeq_TwoSubscribersOneUnsubscribe verifies the refcount: as
// long as ONE subscriber remains, the counter stays alive so its in-flight
// SessionSeq stream doesn't reset to 1 mid-conversation.
func TestHub_SessionSeq_TwoSubscribersOneUnsubscribe(t *testing.T) {
	h := newTestHub(t)
	a := newTestClient("ka")
	b := newTestClient("kb")
	a.send = make(chan []byte, 4)
	b.send = make(chan []byte, 4)
	subscribeClientForTest(t, h, a, "sess-rc")
	subscribeClientForTest(t, h, b, "sess-rc")

	// Burn a few session_seq values.
	for range 3 {
		msg, _ := ws.NewNotification("e", nil)
		h.BroadcastToSession("sess-rc", msg)
	}

	// Unsubscribe one — the counter must still be live with its current value.
	h.UnsubscribeFromSession(a, "sess-rc")

	v, ok := h.sessionSeqs.Load("sess-rc")
	if !ok {
		t.Fatal("session_seq counter dropped while b is still subscribed")
	}
	// 3 broadcasts × 2 fan-out clients = 6 Add calls; after the first
	// unsubscribe the counter remains at 6 (subsequent broadcasts go only to b).
	if got := v.(interface{ Load() int64 }).Load(); got != 6 {
		t.Errorf("session_seq counter=%d, want 6", got)
	}

	// Final unsubscribe drops both maps.
	h.UnsubscribeFromSession(b, "sess-rc")
	if got := h.sessionSeqCountForTest(); got != 0 {
		t.Errorf("after final unsubscribe sessionSeqs len=%d, want 0", got)
	}
}

// TestHub_GetSentEventsForSession_ConnectionWideEventsAreFiltered ensures the
// per-session view only returns session-routed events. A connection-wide
// notification (BroadcastToTask, broadcast) emitted in between must not
// appear in the session filter even though it has a per-connection seq.
func TestHub_GetSentEventsForSession_ConnectionWideEventsAreFiltered(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-mixed")
	c.send = make(chan []byte, 16)
	subscribeClientForTest(t, h, c, "sess-mix")

	// 1: session-routed.
	a, _ := ws.NewNotification("s.evt", nil)
	h.BroadcastToSession("sess-mix", a)
	// 2: connection-wide (BroadcastToTask with no subscribers reaches no one
	// — use the lower-level helper to stamp without filtering).
	wide, _ := ws.NewNotification("conn.evt", nil)
	c.sendStampedCopy(wide)
	// 3: session-routed again.
	b, _ := ws.NewNotification("s.evt", nil)
	h.BroadcastToSession("sess-mix", b)

	entries, maxSessionSeq, ok := h.GetSentEventsForSession(c.ID, "sess-mix")
	if !ok {
		t.Fatal("ok=false")
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2 (only session-routed)", len(entries))
	}
	if maxSessionSeq != 2 {
		t.Errorf("maxSessionSeq=%d, want 2", maxSessionSeq)
	}
	for i, e := range entries {
		if e.SessionSeq != int64(i+1) {
			t.Errorf("entries[%d].SessionSeq=%d, want %d", i, e.SessionSeq, i+1)
		}
		if e.SessionID != "sess-mix" {
			t.Errorf("entries[%d].SessionID=%q, want sess-mix", i, e.SessionID)
		}
	}

	// The full per-connection log still has all three entries.
	all, _, _ := h.GetSentEventsFor(c.ID, 0)
	if len(all) != 3 {
		t.Errorf("len(all)=%d, want 3 (session + connection + session)", len(all))
	}
}

// TestHub_BroadcastToSession_ConcurrentBroadcastsStayMonotonic guards the
// atomic counter under concurrent fan-out — every emitted SessionSeq is
// distinct, contiguous, and within [1, N].
func TestHub_BroadcastToSession_ConcurrentBroadcastsStayMonotonic(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-concurrent-session")
	c.send = make(chan []byte, 512)
	subscribeClientForTest(t, h, c, "sess-conc")

	const N = 200
	var wg sync.WaitGroup
	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg, _ := ws.NewNotification("e", nil)
			h.BroadcastToSession("sess-conc", msg)
		}()
	}
	wg.Wait()

	entries, maxSessionSeq, ok := h.GetSentEventsForSession(c.ID, "sess-conc")
	if !ok {
		t.Fatal("ok=false")
	}
	if len(entries) != N {
		t.Fatalf("len(entries)=%d, want %d", len(entries), N)
	}
	if maxSessionSeq != int64(N) {
		t.Errorf("maxSessionSeq=%d, want %d", maxSessionSeq, N)
	}
	seen := make(map[int64]bool, N)
	for _, e := range entries {
		if e.SessionSeq < 1 || e.SessionSeq > int64(N) {
			t.Errorf("SessionSeq=%d out of range [1,%d]", e.SessionSeq, N)
		}
		if seen[e.SessionSeq] {
			t.Errorf("SessionSeq=%d emitted twice", e.SessionSeq)
		}
		seen[e.SessionSeq] = true
	}
	if len(seen) != N {
		t.Errorf("got %d distinct SessionSeqs, want %d", len(seen), N)
	}
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
