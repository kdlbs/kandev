package orchestrator

import (
	"context"
	"sync"
)

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
	sessionID        string
	observedDetached bool   // true once a Kind==shell detached launch was seen
	turnMarker       uint64 // increments per turn_started event via the FIFO consumer
	parked           bool
	revision         uint64
	lastSample       string             // "live" | "settled" | "unknown" | ""
	samplerCancel    context.CancelFunc // non-nil when a sampler goroutine is running
}

// taskParkedState holds the task-level OR of all session parked states.
type taskParkedState struct {
	mu       sync.Mutex
	members  map[string]bool // sessionID → parked bool
	parked   bool
	revision uint64
}

// stopSampler cancels the running sampler goroutine (if any).
// Must be called with parkedStatesMu held.
func (ps *sessionParkedState) stopSampler() {
	if ps.samplerCancel != nil {
		ps.samplerCancel()
		ps.samplerCancel = nil
	}
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
	ps := &sessionParkedState{sessionID: sessionID}
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
	if ps, ok := s.parkedStates[sessionID]; ok {
		ps.stopSampler()
	}
	delete(s.parkedStates, sessionID)
}

// getOrCreateTaskParkedState returns the task's parked state, creating it if absent.
func (s *Service) getOrCreateTaskParkedState(taskID string) *taskParkedState {
	s.taskParkedStatesMu.Lock()
	defer s.taskParkedStatesMu.Unlock()
	if s.taskParkedStates == nil {
		s.taskParkedStates = make(map[string]*taskParkedState)
	}
	if ts, ok := s.taskParkedStates[taskID]; ok {
		return ts
	}
	ts := &taskParkedState{members: make(map[string]bool)}
	s.taskParkedStates[taskID] = ts
	return ts
}
