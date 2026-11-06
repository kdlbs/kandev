package orchestrator

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	idleQueueDrainInitialBackoff = time.Minute
	idleQueueDrainMaxBackoff     = 15 * time.Minute
)

// idleQueueDrainBackoff spaces repeated drain attempts for one session so a
// session whose dispatch keeps failing is not relaunched on every sweep tick.
type idleQueueDrainBackoff struct {
	mu    sync.Mutex
	state map[string]idleQueueDrainAttempt
}

type idleQueueDrainAttempt struct {
	next  time.Time
	delay time.Duration
}

// retain forgets sessions whose queue emptied since the last pass. A session
// whose dispatch is still in flight keeps its delay: its reserved message is
// absent from the pending list but may yet be returned to the queue.
func (b *idleQueueDrainBackoff) retain(pending []string, inFlight func(sessionID string) bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	live := make(map[string]struct{}, len(pending))
	for _, id := range pending {
		live[id] = struct{}{}
	}
	for id := range b.state {
		if _, ok := live[id]; !ok && (inFlight == nil || !inFlight(id)) {
			delete(b.state, id)
		}
	}
}

func (b *idleQueueDrainBackoff) due(sessionID string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	attempt, seen := b.state[sessionID]
	return !seen || !now.Before(attempt.next)
}

func (b *idleQueueDrainBackoff) attempted(sessionID string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == nil {
		b.state = make(map[string]idleQueueDrainAttempt)
	}
	attempt := b.state[sessionID]
	switch {
	case attempt.delay == 0:
		attempt.delay = idleQueueDrainInitialBackoff
	case attempt.delay < idleQueueDrainMaxBackoff:
		attempt.delay = min(attempt.delay*2, idleQueueDrainMaxBackoff)
	}
	attempt.next = now.Add(attempt.delay)
	b.state[sessionID] = attempt
}

// drainIdleQueues is the sweep's backstop for queued messages that no event
// will deliver. Queue drains run on turn end, boot, enqueue and explicit
// requests; a dispatch that timed out and was returned to the queue, or a row
// left on a session that went idle, otherwise waits until the next such event.
// It reuses DrainQueuedMessageIfAutoRun, so the Auto-run policy,
// promptability, recovery-block and in-flight guards all still apply.
func (s *Service) drainIdleQueues(ctx context.Context) {
	if s.messageQueue == nil {
		return
	}
	pending, err := s.messageQueue.ListSessionsWithPending(ctx)
	if err != nil {
		s.logger.Debug("idle queue drain skipped", zap.Error(err))
		return
	}
	s.idleQueueDrain.retain(pending, s.isQueuedDispatchInFlight)
	now := time.Now()
	for _, sessionID := range pending {
		if ctx.Err() != nil {
			return
		}
		if !s.idleQueueDrain.due(sessionID, now) {
			continue
		}
		dispatched, busy, drainErr := s.tryDrainQueuedMessageIfAutoRun(ctx, sessionID)
		if busy {
			continue
		}
		if drainErr != nil {
			s.logger.Debug("idle queue drain failed",
				zap.String("session_id", sessionID), zap.Error(drainErr))
		}
		if dispatched {
			s.logger.Info("dispatched queued message for idle session",
				zap.String("session_id", sessionID))
		}
		s.idleQueueDrain.attempted(sessionID, now)
	}
}

// tryDrainQueuedMessageIfAutoRun is DrainQueuedMessageIfAutoRun for the sweep:
// it never waits on the session's cancel-in-flight guard. A busy guard means a
// cancel, steer or dispatch owns the session; the sweep reports it busy and
// revisits it on a later pass instead of blocking every session behind it.
func (s *Service) tryDrainQueuedMessageIfAutoRun(ctx context.Context, sessionID string) (dispatched, busy bool, err error) {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	if !lock.TryLock() {
		return false, true, nil
	}
	defer lock.Unlock()
	dispatched, err = s.drainQueuedMessageIfAutoRunLocked(ctx, sessionID)
	return dispatched, false, err
}
