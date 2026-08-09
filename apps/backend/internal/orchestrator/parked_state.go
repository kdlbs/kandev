package orchestrator

import "context"

// backgroundProbePort is the orchestrator's narrow view of the BackgroundProbe
// port defined in the lifecycle package. Using a local interface instead of
// importing lifecycle directly keeps the dependency direction correct and lets
// tests inject a stub without the full lifecycle stack.
type backgroundProbePort interface {
	ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error)
}

// sessionParkedState holds the per-session tracking state for the
// parked-on-background-work projection. Managed under Service.parkedStatesMu.
//
// Lock order: parkedStatesMu must be acquired before any per-session mutex
// (not the other way around). Never hold parkedStatesMu while calling into
// the session repository or workflow engine.
type sessionParkedState struct {
	observedDetached bool   // true once a Kind==shell detached launch was seen
	turnMarker       uint64 // increments per turn_started event via the FIFO consumer
}

// getOrCreateParkedState returns the session's parked state, creating it if absent.
func (s *Service) getOrCreateParkedState(sessionID string) *sessionParkedState {
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	if s.parkedStates == nil {
		s.parkedStates = make(map[string]*sessionParkedState)
	}
	if ps, ok := s.parkedStates[sessionID]; ok {
		return ps
	}
	ps := &sessionParkedState{}
	s.parkedStates[sessionID] = ps
	return ps
}

// parkedStateFor returns the session's parked state or nil if not present.
func (s *Service) parkedStateFor(sessionID string) *sessionParkedState {
	s.parkedStatesMu.RLock()
	defer s.parkedStatesMu.RUnlock()
	return s.parkedStates[sessionID]
}

// deleteParkedState removes the session's parked state entry on execution
// retirement, so the next execution starts fresh.
func (s *Service) deleteParkedState(sessionID string) {
	if sessionID == "" {
		return
	}
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	delete(s.parkedStates, sessionID)
}
