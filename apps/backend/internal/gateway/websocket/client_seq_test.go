package websocket

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestSendMessage_StampsMonotonicSeq verifies that each outbound envelope
// receives a per-connection seq starting at 1 and incrementing by one. This
// is the contract E2E gap-detection depends on.
func TestSendMessage_StampsMonotonicSeq(t *testing.T) {
	c := newTestClient("conn-seq")

	for i := 1; i <= 5; i++ {
		msg, err := ws.NewNotification("test.event", map[string]int{"i": i})
		if err != nil {
			t.Fatalf("build msg: %v", err)
		}
		if !c.sendMessage(msg) {
			t.Fatalf("sendMessage %d returned false", i)
		}
		if msg.Seq != int64(i) {
			t.Errorf("envelope.Seq=%d, want %d", msg.Seq, i)
		}
		if msg.ConnectionID != "conn-seq" {
			t.Errorf("envelope.ConnectionID=%q, want conn-seq", msg.ConnectionID)
		}
	}

	// Frames on the wire must carry the seq too.
	for i := 1; i <= 5; i++ {
		select {
		case raw := <-c.send:
			var m ws.Message
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("decode frame %d: %v", i, err)
			}
			if m.Seq != int64(i) {
				t.Errorf("frame %d Seq=%d, want %d", i, m.Seq, i)
			}
			if m.ConnectionID != "conn-seq" {
				t.Errorf("frame %d ConnectionID=%q, want conn-seq", i, m.ConnectionID)
			}
		default:
			t.Fatalf("expected frame %d on send channel", i)
		}
	}
}

// TestSendStampedCopy_DoesNotMutateInput guards the broadcast fan-out path —
// the input *ws.Message is shared across many client.sendStampedCopy calls,
// so stamping must not leak into the shared envelope.
func TestSendStampedCopy_DoesNotMutateInput(t *testing.T) {
	c1 := newTestClient("c1")
	c2 := newTestClient("c2")

	msg, err := ws.NewNotification("shared.event", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("build msg: %v", err)
	}

	if !c1.sendStampedCopy(msg) {
		t.Fatal("sendStampedCopy c1 returned false")
	}
	if !c2.sendStampedCopy(msg) {
		t.Fatal("sendStampedCopy c2 returned false")
	}

	if msg.Seq != 0 {
		t.Errorf("input msg.Seq mutated to %d (broadcast must not modify shared envelope)", msg.Seq)
	}
	if msg.ConnectionID != "" {
		t.Errorf("input msg.ConnectionID mutated to %q (broadcast must not modify shared envelope)", msg.ConnectionID)
	}
}

// TestSendMessage_ConcurrentStampsStayMonotonic guards the atomic Add path —
// concurrent sends should produce contiguous distinct seqs.
func TestSendMessage_ConcurrentStampsStayMonotonic(t *testing.T) {
	c := newTestClient("c-concurrent")
	// Resize send buffer so concurrent sends don't drop.
	c.send = make(chan []byte, 200)

	const N = 100
	var wg sync.WaitGroup
	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg, _ := ws.NewNotification("e", nil)
			c.sendMessage(msg)
		}()
	}
	wg.Wait()

	seen := make(map[int64]bool, N)
	for range N {
		raw := <-c.send
		var m ws.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.Seq < 1 || m.Seq > int64(N) {
			t.Errorf("seq=%d out of expected range [1,%d]", m.Seq, N)
		}
		if seen[m.Seq] {
			t.Errorf("seq=%d emitted twice", m.Seq)
		}
		seen[m.Seq] = true
	}
	if len(seen) != N {
		t.Errorf("got %d distinct seqs, want %d", len(seen), N)
	}
}

// TestWsSentLog_Append_StoresOldestToNewest exercises the basic ring case
// where the buffer is not yet full.
func TestWsSentLog_Append_StoresOldestToNewest(t *testing.T) {
	l := newWsSentLogWithCapacity(4)
	base := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		l.Append(int64(i), 0, "", "notification", "a.b", base.Add(time.Duration(i)*time.Second))
	}

	entries := l.Since(0)
	if len(entries) != 3 {
		t.Fatalf("len=%d, want 3", len(entries))
	}
	for i, e := range entries {
		want := int64(i + 1)
		if e.Seq != want {
			t.Errorf("entries[%d].Seq=%d, want %d", i, e.Seq, want)
		}
	}
	if got := l.Max(); got != 3 {
		t.Errorf("Max=%d, want 3", got)
	}
}

// TestWsSentLog_RingDiscardsOldestWhenFull is the critical invariant: a
// connection that lives long enough should still report at least the last
// 5000 events without leaking memory.
func TestWsSentLog_RingDiscardsOldestWhenFull(t *testing.T) {
	const capacity = 5
	l := newWsSentLogWithCapacity(capacity)
	base := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		l.Append(int64(i), 0, "", "notification", "a.b", base)
	}

	entries := l.Since(0)
	if len(entries) != capacity {
		t.Fatalf("len=%d, want %d", len(entries), capacity)
	}
	// Newest five = seqs 8..12, oldest first.
	for i, e := range entries {
		want := int64(8 + i)
		if e.Seq != want {
			t.Errorf("entries[%d].Seq=%d, want %d", i, e.Seq, want)
		}
	}
	if got := l.Max(); got != 12 {
		t.Errorf("Max=%d, want 12", got)
	}
}

// TestWsSentLog_SinceFiltersStrictlyGreater confirms the since_seq query
// semantics — the contract the E2E endpoint exposes.
func TestWsSentLog_SinceFiltersStrictlyGreater(t *testing.T) {
	l := newWsSentLogWithCapacity(8)
	for i := 1; i <= 5; i++ {
		l.Append(int64(i), 0, "", "notification", "a.b", time.Now())
	}

	cases := []struct {
		since   int64
		wantLen int
		wantMin int64
	}{
		{since: 0, wantLen: 5, wantMin: 1},
		{since: 2, wantLen: 3, wantMin: 3},
		{since: 5, wantLen: 0, wantMin: 0},
		{since: 99, wantLen: 0, wantMin: 0},
	}
	for _, tc := range cases {
		got := l.Since(tc.since)
		if len(got) != tc.wantLen {
			t.Errorf("Since(%d) len=%d, want %d", tc.since, len(got), tc.wantLen)
			continue
		}
		if tc.wantLen > 0 && got[0].Seq != tc.wantMin {
			t.Errorf("Since(%d)[0].Seq=%d, want %d", tc.since, got[0].Seq, tc.wantMin)
		}
	}
}
