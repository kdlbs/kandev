package messagequeue

import (
	"testing"
	"time"
)

// SetQueuedAtForTesting overwrites the queued_at timestamp of every entry on
// a session in the underlying memory repository. Test-only helper: lets
// external-package tests seed a stale queue entry without exposing the
// repository or adding production code paths that mutate queued_at.
//
// No-op when the service is not backed by the in-memory repository. Callers
// must guarantee the repo is a memoryRepository (NewServiceMemory).
func (s *Service) SetQueuedAtForTesting(_ testing.TB, sessionID string, at time.Time) {
	mem, ok := s.repo.(*memoryRepository)
	if !ok {
		return
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	for _, m := range mem.entries[sessionID] {
		m.QueuedAt = at
	}
}
