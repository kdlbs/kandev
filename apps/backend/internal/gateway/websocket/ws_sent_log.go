package websocket

import (
	"sort"
	"sync"
	"time"
)

// wsSentLogCapacity is the maximum number of recent outbound envelopes retained
// per connection. The ring buffer overwrites the oldest entry when full.
const wsSentLogCapacity = 5000

// wsSentEntry is a record of one outbound envelope written to a connection.
// Aliased to the public WsSentEvent (declared in hub.go) so callers can avoid
// a per-element struct conversion when reading back via Hub.GetSentEventsFor.
type wsSentEntry = WsSentEvent

// wsSentLog is a bounded ring buffer of recent outbound envelopes for a single
// connection. Used by the E2E /api/v1/e2e/ws-sent endpoint so tests can verify
// the FE received every seq the BE sent — gaps indicate WS regressions.
type wsSentLog struct {
	mu      sync.RWMutex
	entries []wsSentEntry // ring; len == capacity once warm
	head    int           // next write index
	size    int           // current number of entries (≤ cap)
	maxSeq  int64
}

// newWsSentLog returns a log with the default capacity.
func newWsSentLog() *wsSentLog {
	return newWsSentLogWithCapacity(wsSentLogCapacity)
}

// newWsSentLogWithCapacity is exposed for tests that want a smaller buffer.
func newWsSentLogWithCapacity(capacity int) *wsSentLog {
	return &wsSentLog{entries: make([]wsSentEntry, capacity)}
}

// Append records an outbound envelope. Discards the oldest entry when full.
// sessionSeq and sessionID are zero/"" for envelopes that aren't routed to a
// specific session (handshake, connection-wide notifications, etc.).
func (l *wsSentLog) Append(seq, sessionSeq int64, sessionID, msgType, action string, sentAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[l.head] = wsSentEntry{
		Seq:        seq,
		SessionSeq: sessionSeq,
		SessionID:  sessionID,
		Type:       msgType,
		Action:     action,
		SentAt:     sentAt,
	}
	l.head = (l.head + 1) % len(l.entries)
	if l.size < len(l.entries) {
		l.size++
	}
	if seq > l.maxSeq {
		l.maxSeq = seq
	}
}

// Since returns all entries with seq > sinceSeq, in ascending seq order. Pass
// sinceSeq=0 to get everything currently in the buffer.
func (l *wsSentLog) Since(sinceSeq int64) []wsSentEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.size == 0 {
		return nil
	}
	out := make([]wsSentEntry, 0, l.size)
	start := 0
	if l.size == len(l.entries) {
		start = l.head // oldest entry sits at head when full
	}
	for i := range l.size {
		e := l.entries[(start+i)%len(l.entries)]
		if e.Seq > sinceSeq {
			out = append(out, e)
		}
	}
	// Defensive sort: capture order is monotonic, but if a future caller
	// stamps out-of-order this keeps the response stable for tests.
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Max returns the highest seq ever appended.
func (l *wsSentLog) Max() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.maxSeq
}

// SinceForSession returns entries for the given sessionID with
// SessionSeq > sinceSessionSeq, sorted by SessionSeq ascending. Entries with
// an empty SessionID (connection-wide notifications, task/run-routed
// broadcasts) are skipped so the result is exactly the per-session stream a
// subscriber should have observed.
func (l *wsSentLog) SinceForSession(sinceSessionSeq int64, sessionID string) []wsSentEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.size == 0 || sessionID == "" {
		return nil
	}
	out := make([]wsSentEntry, 0, l.size)
	start := 0
	if l.size == len(l.entries) {
		start = l.head
	}
	for i := range l.size {
		e := l.entries[(start+i)%len(l.entries)]
		if e.SessionID != sessionID {
			continue
		}
		if e.SessionSeq > sinceSessionSeq {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionSeq < out[j].SessionSeq })
	return out
}
