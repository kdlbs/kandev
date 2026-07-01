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
	bySession        map[string][]wsSentEntry
	head             int
	size             int
	maxConnectionSeq int64
}

func newWsSentLog() *wsSentLog {
	return newWsSentLogWithCapacity(wsSentLogCapacity)
}

func newWsSentLogWithCapacity(capacity int) *wsSentLog {
	if capacity <= 0 {
		panic("websocket sent log capacity must be positive")
	}
	return &wsSentLog{
		entries:   make([]wsSentEntry, capacity),
		bySession: make(map[string][]wsSentEntry),
	}
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

	if l.size == len(l.entries) {
		l.removeSessionEntryLocked(l.entries[l.head])
	}

	entry := wsSentEntry{
		ConnectionSeq: connectionSeq,
		SessionSeq:    sessionSeq,
		SessionID:     sessionID,
		Type:          msgType,
		Action:        action,
		SentAt:        sentAt,
	}
	l.entries[l.head] = entry
	if sessionID != "" {
		l.bySession[sessionID] = append(l.bySession[sessionID], entry)
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
	out := append([]wsSentEntry(nil), l.bySession[sessionID]...)
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

func (l *wsSentLog) removeSessionEntryLocked(entry wsSentEntry) {
	if entry.SessionID == "" {
		return
	}
	entries := l.bySession[entry.SessionID]
	for i, candidate := range entries {
		if candidate.ConnectionSeq != entry.ConnectionSeq {
			continue
		}
		last := len(entries) - 1
		copy(entries[i:], entries[i+1:])
		entries[last] = wsSentEntry{}
		entries = entries[:last]
		break
	}
	if len(entries) == 0 {
		delete(l.bySession, entry.SessionID)
		return
	}
	l.bySession[entry.SessionID] = entries
}
