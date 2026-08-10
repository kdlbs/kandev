package orchestrator

import (
	"context"
	"sync"
)

// backgroundProbePort is the orchestrator's narrow view of the
// lifecycle.Manager BackgroundProbe port. Using a local interface instead of
// importing lifecycle directly keeps the dependency direction correct and
// lets tests inject a stub without the full lifecycle stack.
type backgroundProbePort interface {
	ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error)
}

// sessionParkedState holds V1's per-session tracking state for the
// parked-on-background-work projection (docs/specs/parked-board-mvp/spec.md
// §7). Managed under Service.parkedStatesMu.
//
// V1 deliberately carries none of the parent spec's epoch/revision/
// turnMarker/generation/sampler machinery (D-2, D-5) — that protocol exists
// to order concurrent, asynchronous parked transitions produced by a sampler
// loop, which V1 does not have.
//
// Lock order: parkedStatesMu must be acquired before any per-task mutex, and
// never held across a call into the session repository, the workflow
// engine, the event bus, or the probe port (§7.2).
type sessionParkedState struct {
	sessionID string
	// observedDetached is true once a Kind==shell detached launch was
	// attested during the current turn. Set by markObservedDetached, cleared
	// on the backend's own observed transition into RUNNING or STARTING
	// (D-3).
	observedDetached bool
	// lastSample is the most recent settle-hook probe result: "live",
	// "settled", "unknown", or "" (never probed).
	lastSample string
	// parked is the last computed value of the three-term formula (§7.1).
	parked bool
	// stateGeneration is a per-session, process-local, unserialized counter
	// incremented on every observed transition this slice handles: into
	// RUNNING, into STARTING, and out of WAITING_FOR_INPUT. Used, together
	// with observedDetached, to revalidate a completed probe sample against
	// what the session looked like when the probe was issued — a mismatch
	// discards the sample (§7.2, V1-03). Only inequality is ever observed,
	// never the magnitude or the delta.
	stateGeneration uint64
}

// taskParkedState holds the task-level OR of all session parked states for
// one task. V1 never evicts a member (D-2/D-5's "no eviction" — out of
// scope, deferred to V2/V3): the map grows for the life of the process,
// which the spec accepts as an in-memory, restart-cleared projection (AC-36).
type taskParkedState struct {
	mu      sync.Mutex
	members map[string]bool
	parked  bool
}

// getOrCreateParkedStateLocked returns the session's parked state, creating
// it if absent. Caller must already hold s.parkedStatesMu.
func (s *Service) getOrCreateParkedStateLocked(sessionID string) *sessionParkedState {
	if s.parkedStates == nil {
		s.parkedStates = make(map[string]*sessionParkedState)
	}
	ps, ok := s.parkedStates[sessionID]
	if !ok {
		ps = &sessionParkedState{sessionID: sessionID}
		s.parkedStates[sessionID] = ps
	}
	return ps
}

// parkedStateFor returns the session's parked state or nil if not present.
func (s *Service) parkedStateFor(sessionID string) *sessionParkedState {
	s.parkedStatesMu.RLock()
	defer s.parkedStatesMu.RUnlock()
	return s.parkedStates[sessionID]
}

// markObservedDetached records that the ordered tool-call consumer attested a
// Kind==shell detached launch during the current turn. Creates the session's
// parkedState row if this is the first attestation ever seen for it. The
// lookup-or-create and the field write happen inside one critical section —
// never through getOrCreateParkedState followed by an unlocked field write,
// which would race a concurrent settle-hook read of the same field. Set-to-
// true is idempotent: a repeated attestation within one turn is a no-op.
func (s *Service) markObservedDetached(sessionID string) {
	s.parkedStatesMu.Lock()
	defer s.parkedStatesMu.Unlock()
	ps := s.getOrCreateParkedStateLocked(sessionID)
	ps.observedDetached = true
}

// getOrCreateTaskParkedStateLocked returns the task's parked state, creating
// it if absent. Caller MUST already hold s.taskParkedStatesMu.
func (s *Service) getOrCreateTaskParkedStateLocked(taskID string) *taskParkedState {
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

// SetBackgroundProbe wires the injectable port used by the
// parked-on-background-work projection to determine whether background
// processes are still alive. The production implementation is
// *lifecycle.Manager. Nil-safe: without it, the projection never probes and
// never parks any session (safe default for tests and setups that do not
// configure an agent runtime).
func (s *Service) SetBackgroundProbe(probe backgroundProbePort) {
	s.backgroundProbe = probe
}
