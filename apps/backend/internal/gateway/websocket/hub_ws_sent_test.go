package websocket

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHub_GetSentEventsFor_UnknownConnection returns ok=false rather than an
// empty list so callers (the E2E endpoint) can distinguish "unknown" from
// "known but quiet".
func TestHub_GetSentEventsFor_UnknownConnection(t *testing.T) {
	h := newTestHub(t)
	events, maxSeq, ok := h.GetSentEventsFor("does-not-exist", 0)
	if ok {
		t.Errorf("ok=true, want false; events=%v maxSeq=%d", events, maxSeq)
	}
	if events != nil {
		t.Errorf("events=%v, want nil", events)
	}
	if maxSeq != 0 {
		t.Errorf("maxSeq=%d, want 0", maxSeq)
	}
}

// TestHub_GetSentEventsFor_ReturnsRingBufferContents covers the happy path:
// register a client, push frames through sendMessage, read the log back.
func TestHub_GetSentEventsFor_ReturnsRingBufferContents(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-known")
	c.hub = h
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	for i := 1; i <= 3; i++ {
		msg, _ := ws.NewNotification("evt", map[string]int{"i": i})
		c.sendMessage(msg)
	}

	events, maxSeq, ok := h.GetSentEventsFor("c-known", 0)
	if !ok {
		t.Fatal("ok=false for known connection")
	}
	if maxSeq != 3 {
		t.Errorf("maxSeq=%d, want 3", maxSeq)
	}
	if len(events) != 3 {
		t.Fatalf("len(events)=%d, want 3", len(events))
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Errorf("events[%d].Seq=%d, want %d", i, e.Seq, i+1)
		}
		if e.Action != "evt" {
			t.Errorf("events[%d].Action=%q, want evt", i, e.Action)
		}
		if e.Type != string(ws.MessageTypeNotification) {
			t.Errorf("events[%d].Type=%q, want notification", i, e.Type)
		}
		if e.SentAt.IsZero() {
			t.Errorf("events[%d].SentAt is zero", i)
		}
	}
}

// TestHub_GetSentEventsFor_SinceFilter mirrors the endpoint's incremental
// polling behavior — the FE accountant pulls deltas, not the whole buffer.
func TestHub_GetSentEventsFor_SinceFilter(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-since")
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	for range 5 {
		msg, _ := ws.NewNotification("evt", nil)
		c.sendMessage(msg)
	}

	events, maxSeq, ok := h.GetSentEventsFor("c-since", 3)
	if !ok {
		t.Fatal("ok=false")
	}
	if maxSeq != 5 {
		t.Errorf("maxSeq=%d, want 5", maxSeq)
	}
	if len(events) != 2 {
		t.Fatalf("len=%d, want 2 (seqs 4,5)", len(events))
	}
	if events[0].Seq != 4 || events[1].Seq != 5 {
		t.Errorf("got seqs=[%d,%d], want [4,5]", events[0].Seq, events[1].Seq)
	}
}

// TestHub_BroadcastToTask_StampsEachClientIndependently verifies the
// fan-out path correctly assigns per-connection seqs. Two clients each
// subscribed should get seq=1 individually rather than 1 then 2.
func TestHub_BroadcastToTask_StampsEachClientIndependently(t *testing.T) {
	h := newTestHub(t)
	a := newTestClient("a")
	b := newTestClient("b")
	h.mu.Lock()
	h.clients[a] = true
	h.clients[b] = true
	h.taskSubscribers["t1"] = map[*Client]bool{a: true, b: true}
	a.subscriptions["t1"] = true
	b.subscriptions["t1"] = true
	h.mu.Unlock()

	msg, _ := ws.NewNotification("task.evt", nil)
	h.BroadcastToTask("t1", msg)

	for _, c := range []*Client{a, b} {
		events, maxSeq, ok := h.GetSentEventsFor(c.ID, 0)
		if !ok {
			t.Fatalf("client %s not registered", c.ID)
		}
		if maxSeq != 1 {
			t.Errorf("client %s maxSeq=%d, want 1", c.ID, maxSeq)
		}
		if len(events) != 1 || events[0].Seq != 1 {
			t.Errorf("client %s events=%+v, want one entry with seq=1", c.ID, events)
		}
	}

	// The shared input envelope must NOT have been mutated.
	if msg.Seq != 0 {
		t.Errorf("input msg.Seq=%d, broadcast leaked stamp into shared envelope", msg.Seq)
	}
}
