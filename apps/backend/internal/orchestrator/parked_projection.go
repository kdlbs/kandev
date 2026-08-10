package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/common/parkedprobe"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// probe result strings — match agentctl/server/process probe constants.
const (
	probeResultLive    = "live"
	probeResultSettled = "settled"
	probeResultUnknown = "unknown"
)

// computeParked returns true when all three projection terms are satisfied
// (spec §7.1's three-term formula).
func (ps *sessionParkedState) computeParked(sessionState models.TaskSessionState) bool {
	return ps.observedDetached &&
		ps.lastSample == probeResultLive &&
		sessionState == models.TaskSessionStateWaitingForInput
}

// probeRevalidationSnapshot captures the two values §7.2's revalidation rule
// compares against at probe completion.
type probeRevalidationSnapshot struct {
	observedDetached bool
	stateGeneration  uint64
}

// captureProbeSnapshot returns the session's current (observedDetached,
// stateGeneration) tuple, and ok=false when the session has no parkedState
// row or is not currently attested. AC-40a: a probe is issued only when the
// settling turn actually attested a detached launch, so the common turn end
// pays nothing.
func (s *Service) captureProbeSnapshot(sessionID string) (probeRevalidationSnapshot, bool) {
	s.parkedStatesMu.RLock()
	defer s.parkedStatesMu.RUnlock()
	ps := s.parkedStates[sessionID]
	if ps == nil || !ps.observedDetached {
		return probeRevalidationSnapshot{}, false
	}
	return probeRevalidationSnapshot{observedDetached: true, stateGeneration: ps.stateGeneration}, true
}

// currentSessionState reads the session's live TaskSessionState immediately
// before a parked write is applied, rather than trusting the state at
// hook-entry time. The settle hook that fires onSessionParkedHook only
// guarantees WAITING_FOR_INPUT at the instant it fires — the probe's own
// round trip (up to KANDEV_PARKED_PROBE_BUDGET) can outlast that window, and
// a session that leaves WAITING_FOR_INPUT while the probe is in flight must
// not be re-parked by a write that still assumes the state it had at entry.
// A lookup failure returns the zero value, which computeParked never treats
// as WAITING_FOR_INPUT — the cheap, non-parking direction.
func (s *Service) currentSessionState(ctx context.Context, sessionID string) models.TaskSessionState {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	return session.State
}

// runProbe calls the background probe port. Every error, an out-of-domain
// result, and a panicking implementation all resolve to "unknown" (AC-46) —
// the caller MUST NOT read the port's returned value when it also returned a
// non-nil error, so that rule is applied once here rather than trusted to
// every caller.
//
// Applies its own context.WithTimeout(ctx, budget) around the port call
// (spec §7.3: "the synchronous probe runs under context.WithTimeout(ctx,
// budget)"). This orchestrator-level bound is necessary, not speculative:
// process.Manager.ProbeProcessTree's own context.WithTimeout only wraps the
// OS-level walk on the agentctl side of the WebSocket boundary — it cannot
// bound the round trip back to this call, which blocks on
// client_stream.go's select{respCh/ctx.Done()} for as long as ctx itself
// allows. Production settle-path contexts have no deadline of their own (the
// event bus publishes with context.Background()), so without this timeout a
// wedged/half-open agentctl connection would block the settle transition
// indefinitely (Review round 2 MUST-FIX 1) — violating AC-40's "not delayed
// beyond the budget."
func (s *Service) runProbe(ctx context.Context, sessionID string) (result string) {
	result = probeResultUnknown
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Warn("background probe panicked, treating as unknown",
					zap.String("session_id", sessionID),
					zap.Any("panic", r))
			}
			result = probeResultUnknown
		}
	}()
	probeCtx, cancel := context.WithTimeout(ctx, parkedprobe.ParseEnvBudget(s.logger))
	defer cancel()
	value, err := s.backgroundProbe.ProbeBackgroundWorkloads(probeCtx, sessionID)
	if err != nil {
		return probeResultUnknown
	}
	switch value {
	case probeResultLive, probeResultSettled:
		return value
	default:
		return probeResultUnknown
	}
}

// onSessionParkedHook fires synchronously after a session transitions to
// WAITING_FOR_INPUT. It runs the one synchronous settle-hook probe — but
// only when the settling turn actually attested a detached launch (AC-40a) —
// and updates the parked projection. Gate placement: the settle hook, and
// only the settle hook (§7.5) — when the flag is off this returns before
// capturing any snapshot and before issuing any probe, so lastSample is
// never written and every DTO serializes false.
func (s *Service) onSessionParkedHook(ctx context.Context, taskID, sessionID string) {
	if !s.config.ParkedOnBackgroundWork {
		return
	}
	if s.backgroundProbe == nil {
		return
	}
	snap, ok := s.captureProbeSnapshot(sessionID)
	if !ok {
		return
	}
	sample := s.runProbe(ctx, sessionID)
	sessionState := s.currentSessionState(ctx, sessionID)

	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil || ps.observedDetached != snap.observedDetached || ps.stateGeneration != snap.stateGeneration {
		// §7.2/V1-03 revalidation, applied in the SAME critical section as
		// the write below rather than as a separate, unlocked check
		// beforehand. Discarding writes nothing: no transition, no publish.
		s.parkedStatesMu.Unlock()
		return
	}
	ps.lastSample = sample
	oldParked := ps.parked
	newParked := ps.computeParked(sessionState)
	ps.parked = newParked
	s.parkedStatesMu.Unlock()

	if newParked != oldParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID)
	}
}

// handleParkedStateTransition implements D-3's attestation-clearing rule and
// the session-state term of the formula clearing on leaving
// WAITING_FOR_INPUT (AC-68), off the one seam updateTaskSessionStateWithHook
// observes every CAS-confirmed session-state transition through. Called for
// every transition EXCEPT into WAITING_FOR_INPUT, which onSessionParkedHook
// (the settle probe) owns instead — the two are mutually exclusive per call
// since oldState != nextState is already guaranteed by the caller.
//
// stateGeneration is incremented on entering RUNNING or STARTING AND
// separately on leaving WAITING_FOR_INPUT — a single transition (e.g.
// WAITING_FOR_INPUT -> RUNNING) can therefore increment it twice, which is
// harmless and intended (§7.2): only inequality is ever observed.
func (s *Service) handleParkedStateTransition(ctx context.Context, taskID, sessionID string, oldState, nextState models.TaskSessionState) {
	enteringRunningOrStarting := nextState == models.TaskSessionStateRunning || nextState == models.TaskSessionStateStarting
	leavingWaiting := oldState == models.TaskSessionStateWaitingForInput
	if !enteringRunningOrStarting && !leavingWaiting {
		return
	}

	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.Unlock()
		return
	}
	if enteringRunningOrStarting {
		// D-3: clears the attestation on the backend's own observed
		// transition into RUNNING or STARTING — the substitute for the
		// deferred turn_started event (V1-01, V1-02).
		ps.observedDetached = false
		ps.stateGeneration++
	}
	var wasParked bool
	if leavingWaiting {
		ps.stateGeneration++
		wasParked = ps.parked
		if wasParked {
			ps.parked = false
		}
	}
	s.parkedStatesMu.Unlock()

	if wasParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID)
	}
}

// publishParkedChanged re-publishes session.activity_changed so a parked
// transition rides on the spec's named carrier (§7.6) — no new event type,
// no new WebSocket action. publishForegroundActivityNow re-derives the
// parked value itself via ParkedProjectionSnapshot, so it reads back exactly
// what was just written. foreground_activity and active_subagent_count are
// re-derived fresh too, so the frame is complete regardless of what
// triggered it.
func (s *Service) publishParkedChanged(ctx context.Context, taskID, sessionID string) {
	if s.eventBus == nil || sessionID == "" {
		return
	}
	s.publishForegroundActivityNow(
		ctx, taskID, sessionID,
		string(s.ForegroundActivity(sessionID)),
		s.ActiveSubagentCount(sessionID),
	)
}

// updateTaskParkedState updates the task-level OR and publishes a task
// update if the OR changed. V1 carries no revision/ordering guard (D-2):
// there is no sampler and therefore no concurrent, asynchronous transition
// stream to reorder — the state CAS in updateTaskSessionStateWithHook is
// already the single serialization point for a given session (§7.2).
//
// Reads the CURRENT session-level projection via ParkedProjectionSnapshot at
// call time rather than accepting a caller-captured bool: onSessionParkedHook
// and handleParkedStateTransition each release parkedStatesMu before calling
// this method, so two racing transitions for the same session can call it in
// either physical order. A captured argument would let the physically-later
// call carry an already-superseded value and permanently clobber the correct
// one in taskParkedState.members, since that map is never re-synced.
// Re-reading here means whichever call actually executes last always writes
// whatever ps.parked currently holds, converging correctly regardless of
// which logical transition triggered it first (see the review round 1
// finding this closes).
func (s *Service) updateTaskParkedState(ctx context.Context, taskID, sessionID string) {
	if taskID == "" {
		return
	}
	parked := s.ParkedProjectionSnapshot(sessionID)
	s.taskParkedStatesMu.Lock()
	ts := s.getOrCreateTaskParkedStateLocked(taskID)
	ts.mu.Lock()
	ts.members[sessionID] = parked
	newTaskParked := anyParkedMember(ts.members)
	changed := newTaskParked != ts.parked
	if changed {
		ts.parked = newTaskParked
	}
	ts.mu.Unlock()
	s.taskParkedStatesMu.Unlock()

	if changed {
		s.publishTaskParkedChanged(ctx, taskID)
	}
}

// anyParkedMember returns true if any member value is true. Order-free and
// idempotent — member iteration order is not observable (§7.1).
func anyParkedMember(members map[string]bool) bool {
	for _, v := range members {
		if v {
			return true
		}
	}
	return false
}

// publishTaskParkedChanged triggers a task.updated event so the task DTO
// carries the refreshed task-level parked projection to clients.
func (s *Service) publishTaskParkedChanged(ctx context.Context, taskID string) {
	s.publishTaskActivityIfChanged(ctx, taskID)
}

// ParkedProjectionSnapshot implements dto.ParkedProjectionProvider — the
// boolean-only form (D-2: no epoch, no revision).
func (s *Service) ParkedProjectionSnapshot(sessionID string) bool {
	s.parkedStatesMu.RLock()
	defer s.parkedStatesMu.RUnlock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		return false
	}
	return ps.parked
}

// TaskParkedProjectionSnapshot implements dto.TaskParkedProjectionProvider —
// the boolean-only form (D-2: no epoch, no revision).
func (s *Service) TaskParkedProjectionSnapshot(taskID string) bool {
	s.taskParkedStatesMu.Lock()
	defer s.taskParkedStatesMu.Unlock()
	ts, ok := s.taskParkedStates[taskID]
	if !ok {
		return false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.parked
}
