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
	// generation is a Go-internal anti-ABA counter, not part of the spec's
	// wire-visible Data model and never serialized on any carrier. The spec
	// requires turnMarker to restart at 0 on both eviction (reduced) and
	// revival (a fresh attestation on a reduced row), so turnMarker alone
	// cannot distinguish a stale in-flight sample from a since-evicted
	// execution from a freshly revived row that happens to read the same
	// (observedDetached, turnMarker) pair. generation increments once per
	// eviction of an existing row and is compared alongside turnMarker at
	// probe-revalidation time to close that gap.
	generation uint64
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

// markObservedDetached records that a registered recogniser attested a
// detached shell launch during the current turn (D3, AC-69/AC-69a). Creates
// the session's parkedState row if this is the first attestation ever seen
// for it. The lookup-or-create and the field write happen inside one
// critical section — never through getOrCreateParkedState followed by an
// unlocked field write, which races the sampler goroutine's locked reads of
// the same field.
func (s *Service) markObservedDetached(sessionID string) {
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	if s.parkedStates == nil {
		s.parkedStates = make(map[string]*sessionParkedState)
	}
	ps, ok := s.parkedStates[sessionID]
	if !ok {
		ps = &sessionParkedState{sessionID: sessionID}
		s.parkedStates[sessionID] = ps
	}
	ps.observedDetached = true
}

// clearObservedDetachedOnTurnStarted implements D3's turn-boundary rule: on
// every turn_started for a session with an existing parkedState row, it
// clears observedDetached (idempotent — a plain set-to-false) and increments
// turnMarker (NOT idempotent — it moves on every event, duplicates included,
// per D2/AC-41a's fourth clause) in the same critical section, and resets the
// sampler's in-flight sample state since a new turn invalidates the question
// any outstanding probe was asked. A turn_started for a session with no
// parkedState row is a no-op: it creates no entry and increments no marker
// (AC-41a's final clause).
func (s *Service) clearObservedDetachedOnTurnStarted(sessionID string) {
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		return
	}
	ps.observedDetached = false
	ps.turnMarker++
	ps.stopSampler()
	ps.lastSample = ""
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
