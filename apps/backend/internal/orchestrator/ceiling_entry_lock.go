package orchestrator

import "context"

type ceilingEntryAdmissionLock struct {
	mu   chan struct{}
	refs int
}

type ceilingEntryAdmissionContextKey struct{}

// acquireCeilingEntryAdmissionLock serializes operations that read or write a
// task's committed workflow route and its ceiling deferred-launch record. A
// channel is used instead of a mutex so the lock can be acquired with the same
// simple release shape as the other orchestrator guards.
func (s *Service) acquireCeilingEntryAdmissionLock(taskID string) func() {
	if s == nil || taskID == "" {
		return func() {}
	}
	s.ceilingEntryAdmissionLocksMu.Lock()
	if s.ceilingEntryAdmissionLocks == nil {
		s.ceilingEntryAdmissionLocks = make(map[string]*ceilingEntryAdmissionLock)
	}
	entry := s.ceilingEntryAdmissionLocks[taskID]
	if entry == nil {
		entry = &ceilingEntryAdmissionLock{mu: make(chan struct{}, 1)}
		s.ceilingEntryAdmissionLocks[taskID] = entry
	}
	entry.refs++
	s.ceilingEntryAdmissionLocksMu.Unlock()

	entry.mu <- struct{}{}
	return func() {
		<-entry.mu
		s.ceilingEntryAdmissionLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.ceilingEntryAdmissionLocks, taskID)
		}
		s.ceilingEntryAdmissionLocksMu.Unlock()
	}
}

func ceilingEntryAdmissionLockHeld(ctx context.Context, taskID string) bool {
	if ctx == nil || taskID == "" {
		return false
	}
	owned, _ := ctx.Value(ceilingEntryAdmissionContextKey{}).(string)
	return owned == taskID
}

// lockCeilingEntryAdmission makes nested helpers re-entrant while keeping the
// ownership marker scoped to the context used inside the critical section.
func (s *Service) lockCeilingEntryAdmission(ctx context.Context, taskID string) (context.Context, func()) {
	if taskID == "" || ceilingEntryAdmissionLockHeld(ctx, taskID) {
		return ctx, func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release := s.acquireCeilingEntryAdmissionLock(taskID)
	return context.WithValue(ctx, ceilingEntryAdmissionContextKey{}, taskID), release
}
