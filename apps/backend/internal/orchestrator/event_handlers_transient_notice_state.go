package orchestrator

import (
	"sync"
	"time"
)

func (s *Service) transientRetryNoticeFenceDuration() time.Duration {
	if s.transientRetryNoticeFenceTTL > 0 {
		return s.transientRetryNoticeFenceTTL
	}
	return defaultTransientRetryNoticeFenceTTL
}

// acquireTransientRetryNoticeState returns the one state object for a
// session. The release function keeps the map entry alive until no caller can
// still hold its mutex, even when a lifecycle operation is waiting behind
// another caller.
func (s *Service) acquireTransientRetryNoticeState(sessionID string) (*transientRetryNoticeState, func()) {
	if sessionID == "" {
		return nil, func() {}
	}

	s.transientRetryNoticeStatesMu.Lock()
	if s.transientRetryNoticeStates == nil {
		s.transientRetryNoticeStates = make(map[string]*transientRetryNoticeState)
	}
	state := s.transientRetryNoticeStates[sessionID]
	if state == nil {
		state = &transientRetryNoticeState{}
		s.transientRetryNoticeStates[sessionID] = state
	}
	state.refs++
	s.transientRetryNoticeStatesMu.Unlock()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			s.releaseTransientRetryNoticeState(sessionID, state)
		})
	}
	return state, release
}

func (s *Service) releaseTransientRetryNoticeState(sessionID string, state *transientRetryNoticeState) {
	s.transientRetryNoticeStatesMu.Lock()
	if state.refs > 0 {
		state.refs--
	}
	s.reclaimTransientRetryNoticeStateLocked(sessionID, state)
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) reclaimTransientRetryNoticeStateLocked(sessionID string, state *transientRetryNoticeState) {
	if state.refs != 0 || state.owned.Load() {
		return
	}
	if state.retired.Load() && state.retiredUntil.Load() > time.Now().UnixNano() {
		return
	}
	if current, ok := s.transientRetryNoticeStates[sessionID]; !ok || current != state {
		return
	}
	if state.fenceTimer != nil {
		state.fenceTimer.Stop()
		state.fenceTimer = nil
	}
	delete(s.transientRetryNoticeStates, sessionID)
}

func (s *Service) retireTransientRetryNoticeLocked(sessionID string, state *transientRetryNoticeState) {
	// Callers hold state.mu. Keep this order, state.mu -> statesMu, everywhere
	// that touches the lifecycle map so a waiter cannot observe a replacement
	// state while another caller still owns this state's mutex.
	state.retired.Store(true)
	ttl := s.transientRetryNoticeFenceDuration()
	state.retiredUntil.Store(time.Now().Add(ttl).UnixNano())

	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; ok && current == state {
		if state.fenceTimer != nil {
			state.fenceTimer.Stop()
		}
		state.fenceTimer = time.AfterFunc(ttl, func() {
			s.expireTransientRetryNoticeFence(sessionID, state)
		})
	}
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) expireTransientRetryNoticeFence(sessionID string, state *transientRetryNoticeState) {
	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; !ok || current != state {
		s.transientRetryNoticeStatesMu.Unlock()
		return
	}
	state.fenceTimer = nil
	if state.retired.Load() {
		if remaining := time.Until(time.Unix(0, state.retiredUntil.Load())); remaining > 0 {
			state.fenceTimer = time.AfterFunc(remaining, func() {
				s.expireTransientRetryNoticeFence(sessionID, state)
			})
		} else {
			s.reclaimTransientRetryNoticeStateLocked(sessionID, state)
		}
	}
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) clearTransientRetryNoticeFenceLocked(sessionID string, state *transientRetryNoticeState) {
	state.retired.Store(false)
	state.retiredUntil.Store(0)
	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; ok && current == state && state.fenceTimer != nil {
		state.fenceTimer.Stop()
		state.fenceTimer = nil
	}
	s.transientRetryNoticeStatesMu.Unlock()
}
