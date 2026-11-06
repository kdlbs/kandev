package orchestrator

import (
	"sync"
	"time"
)

// The existing restart pass owns scheduling; this only spaces retry attempts.
type automaticHistoryBackoff struct {
	mu    sync.Mutex
	state map[string]automaticHistoryAttemptState
}

type automaticHistoryAttemptState struct {
	next  time.Time
	delay time.Duration
}

func (b *automaticHistoryBackoff) due(sessionID string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	attempt, seen := b.state[sessionID]
	return !seen || !now.Before(attempt.next)
}

func (b *automaticHistoryBackoff) attempted(sessionID string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == nil {
		b.state = make(map[string]automaticHistoryAttemptState)
	}
	attempt := b.state[sessionID]
	switch {
	case attempt.delay == 0:
		attempt.delay = time.Minute
	case attempt.delay < 15*time.Minute:
		attempt.delay = min(attempt.delay*2, 15*time.Minute)
	}
	attempt.next = now.Add(attempt.delay)
	b.state[sessionID] = attempt
}
