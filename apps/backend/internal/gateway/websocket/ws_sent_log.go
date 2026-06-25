package websocket

import (
	"sort"
	"sync"
	"time"
)

const wsSentLogCapacity = 5000

type wsSentEntry = WsSentEvent

type wsSentLog struct {
	mu               sync.RWMutex
	entries          []wsSentEntry
	head             int
	size             int
	maxConnectionSeq int64
}

func newWsSentLog() *wsSentLog {
	return newWsSentLogWithCapacity(wsSentLogCapacity)
}

func newWsSentLogWithCapacity(capacity int) *wsSentLog {
	if capacity <= 0 {
		capacity = wsSentLogCapacity
	}
	return &wsSentLog{entries: make([]wsSentEntry, capacity)}
}

func (l *wsSentLog) Append(
	connectionSeq int64,
	sessionSeq int64,
	sessionID string,
	msgType string,
	action string,
	sentAt time.Time,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries[l.head] = wsSentEntry{
		ConnectionSeq: connectionSeq,
		SessionSeq:    sessionSeq,
		SessionID:     sessionID,
		Type:          msgType,
		Action:        action,
		SentAt:        sentAt,
	}
	l.head = (l.head + 1) % len(l.entries)
	if l.size < len(l.entries) {
		l.size++
	}
	if connectionSeq > l.maxConnectionSeq {
		l.maxConnectionSeq = connectionSeq
	}
}

func (l *wsSentLog) Since(sinceConnectionSeq int64) []wsSentEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]wsSentEntry, 0, l.size)
	l.eachLocked(func(e wsSentEntry) {
		if e.ConnectionSeq > sinceConnectionSeq {
			out = append(out, e)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].ConnectionSeq < out[j].ConnectionSeq
	})
	return out
}

func (l *wsSentLog) SinceForSession(sessionID string) []wsSentEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if sessionID == "" {
		return nil
	}
	out := make([]wsSentEntry, 0, l.size)
	l.eachLocked(func(e wsSentEntry) {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionSeq < out[j].SessionSeq
	})
	return out
}

func (l *wsSentLog) MaxConnectionSeq() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.maxConnectionSeq
}

func (l *wsSentLog) eachLocked(fn func(wsSentEntry)) {
	if l.size == 0 {
		return
	}
	start := 0
	if l.size == len(l.entries) {
		start = l.head
	}
	for i := 0; i < l.size; i++ {
		fn(l.entries[(start+i)%len(l.entries)])
	}
}
