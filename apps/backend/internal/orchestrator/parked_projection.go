package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// probe result strings — match agentctl/server/process probe constants.
const (
	probeResultLive    = "live"
	probeResultSettled = "settled"
	probeResultUnknown = "unknown"
)

// computeParked returns true when all three projection terms are satisfied.
func (ps *sessionParkedState) computeParked(sessionState models.TaskSessionState) bool {
	return ps.observedDetached &&
		ps.lastSample == probeResultLive &&
		sessionState == models.TaskSessionStateWaitingForInput
}

// probeRevalidationSnapshot captures the values D2's revalidation rule
// compares against at probe completion. generation is the anti-ABA guard
// documented on sessionParkedState — it is not part of the spec's Data
// model and travels on no carrier.
type probeRevalidationSnapshot struct {
	observedDetached bool
	turnMarker       uint64
	generation       uint64
}

// captureProbeSnapshot returns the session's current (observedDetached,
// turnMarker, generation) triple, and ok=false when the session has no
// parkedState row or is not currently attested. AC-40a: a probe is issued
// only when the settling turn actually attested a detached launch, so the
// common turn end pays nothing.
func (s *Service) captureProbeSnapshot(sessionID string) (probeRevalidationSnapshot, bool) {
	s.parkedStatesMu.RLock()
	defer s.parkedStatesMu.RUnlock()
	ps := s.parkedStates[sessionID]
	if ps == nil || !ps.observedDetached {
		return probeRevalidationSnapshot{}, false
	}
	return probeRevalidationSnapshot{observedDetached: true, turnMarker: ps.turnMarker, generation: ps.generation}, true
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

// onSessionParkedHook fires synchronously after a session transitions to
// WAITING_FOR_INPUT. It runs the first probe — but only when the settling
// turn actually attested a detached launch (AC-40a) — updates the parked
// projection, and starts the background sampler goroutine if the session is
// parked.
func (s *Service) onSessionParkedHook(ctx context.Context, taskID, sessionID string) {
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
	if ps == nil || ps.observedDetached != snap.observedDetached || ps.turnMarker != snap.turnMarker || ps.generation != snap.generation {
		// D2 revalidation, applied in the SAME critical section as the write
		// below rather than as a separate, unlocked check beforehand — a
		// concurrent turn_started (or an eviction) cannot slip a state change
		// into the gap between validating and applying. Discarding writes
		// nothing: no transition, no revision move.
		s.parkedStatesMu.Unlock()
		return
	}
	ps.lastSample = sample
	oldParked := ps.parked
	newParked := ps.computeParked(sessionState)
	var startSampler bool
	var samplerCtx context.Context
	if newParked != oldParked {
		ps.parked = newParked
		ps.revision++
	}
	if newParked {
		ps.stopSampler()
		samplerCtx, ps.samplerCancel = context.WithCancel(context.Background())
		startSampler = true
	} else {
		ps.stopSampler()
	}
	s.parkedStatesMu.Unlock()

	if newParked != oldParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID, newParked)
	}

	if startSampler {
		go s.runParkingSampler(samplerCtx, taskID, sessionID)
	}
}

// runProbe calls the background probe with the configured budget. Every
// error, an out-of-domain result, and a panicking implementation all resolve
// to "unknown" (AC-46) — the caller MUST NOT read the port's returned value
// when it also returned a non-nil error, so that rule is applied once here
// rather than trusted to every caller.
func (s *Service) runProbe(parent context.Context, sessionID string) (result string) {
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
	probeCtx, cancel := context.WithTimeout(parent, s.config.BackgroundProbeBudget)
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

// sampleAndPublishParked runs one probe, updates the session parked state,
// and publishes if the parked value changed. Returns true if the sampler
// should continue running.
func (s *Service) sampleAndPublishParked(ctx context.Context, taskID, sessionID string) bool {
	if s.backgroundProbe == nil {
		return false
	}
	snap, ok := s.captureProbeSnapshot(sessionID)
	if !ok {
		return false
	}
	sample := s.runProbe(ctx, sessionID)
	sessionState := s.currentSessionState(ctx, sessionID)

	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil || ps.observedDetached != snap.observedDetached || ps.turnMarker != snap.turnMarker || ps.generation != snap.generation {
		// Discarded per D2, revalidated in the same critical section as the
		// write below (see onSessionParkedHook). Nothing is written; the
		// loop's next tick re-issues against whatever the new turn's state
		// actually is.
		s.parkedStatesMu.Unlock()
		return true
	}
	ps.lastSample = sample
	oldParked := ps.parked
	newParked := ps.computeParked(sessionState)
	if newParked != oldParked {
		ps.parked = newParked
		ps.revision++
	}
	if !newParked {
		ps.stopSampler()
	}
	s.parkedStatesMu.Unlock()

	if newParked != oldParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID, newParked)
	}

	return newParked
}

// unparkOnStateLeave implements AC-53/AC-68: when a session leaves
// WAITING_FOR_INPUT, sampling stops immediately — the session-state term of
// the formula is now false regardless of last_sample — and if the session
// was parked, the un-park is published without taking a further sample;
// last_sample is never re-read for this transition.
func (s *Service) unparkOnStateLeave(ctx context.Context, taskID, sessionID string) {
	s.parkedStatesMu.Lock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.Unlock()
		return
	}
	ps.stopSampler()
	wasParked := ps.parked
	if wasParked {
		ps.parked = false
		ps.revision++
	}
	s.parkedStatesMu.Unlock()

	if wasParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID, false)
	}
}

// evictParkedState implements the Entry-lifecycle eviction rule (AC-85): if
// the session is currently parked, it performs the ordinary true→false
// transition — flip parked, bump revision, publish session.activity_changed,
// update task-level membership — before touching the row itself. Only after
// that publish does it either reduce the row in place (remove=false: used on
// execution end and agent-context reset, where the session continues to
// exist — observedDetached/turnMarker/lastSample are cleared but revision is
// retained, so a later serialization still carries the retained revision
// rather than resetting to 0) or drop it outright (remove=true, used only
// when the session itself is being deleted — nothing will ever read this row
// again). A row that is already unparked, or absent, publishes nothing.
func (s *Service) evictParkedState(ctx context.Context, taskID, sessionID string, remove bool) {
	if sessionID == "" {
		return
	}
	s.parkedStatesMu.Lock()
	ps, ok := s.parkedStates[sessionID]
	if !ok {
		s.parkedStatesMu.Unlock()
		return
	}
	wasParked := ps.parked
	if wasParked {
		ps.parked = false
		ps.revision++
	}
	ps.stopSampler()
	ps.observedDetached = false
	ps.turnMarker = 0
	ps.lastSample = ""
	// F2 (Review round 2): bump the anti-ABA generation on every eviction of
	// an existing row, not just the parked ones. turnMarker restarts at 0
	// here and again on revival (spec: Data model → Entry lifecycle), so a
	// stale in-flight sample captured before this eviction could otherwise
	// pass revalidation against a later, unrelated execution's freshly
	// revived row that happens to read the same (observedDetached=true,
	// turnMarker=0) pair. generation, never reset, breaks that tie.
	ps.generation++
	if remove {
		delete(s.parkedStates, sessionID)
	}
	s.parkedStatesMu.Unlock()

	if wasParked {
		s.publishParkedChanged(ctx, taskID, sessionID)
		s.updateTaskParkedState(ctx, taskID, sessionID, false)
	}
}

// runParkingSampler ticks at BackgroundSampleInterval until context is done
// or the session is no longer parked.
func (s *Service) runParkingSampler(ctx context.Context, taskID, sessionID string) {
	interval := s.config.BackgroundSampleInterval
	// AC-74: zero disables periodic sampling entirely — not "use the
	// default". The session stays parked at whatever the synchronous first
	// sample decided until it leaves WAITING_FOR_INPUT (unparkOnStateLeave).
	// Negative is already rejected at config-load time (parseParkedProbeInterval),
	// so this can only be reached with interval==0 or interval>0.
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			keepRunning := s.sampleAndPublishParked(ctx, taskID, sessionID)
			if !keepRunning {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// publishParkedChanged re-publishes session.activity_changed so a parked
// transition rides on the spec's named carrier (Review round 2, F3: the API
// surface section and AC-62/68/78/85 all name session.activity_changed, not
// a dedicated event). The caller has already written the new
// parked/revision under parkedStatesMu and released it before calling this;
// publishForegroundActivityNow re-derives the parked triple itself via
// ParkedProjectionSnapshot, so it reads back exactly what was just written.
// foreground_activity and active_subagent_count are re-derived fresh too, so
// the frame is complete regardless of what triggered it.
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

// updateTaskParkedState updates the task-level OR and publishes a task update
// if the OR changed.
func (s *Service) updateTaskParkedState(ctx context.Context, taskID, sessionID string, parked bool) {
	if taskID == "" {
		return
	}
	ts := s.getOrCreateTaskParkedState(taskID)
	ts.mu.Lock()
	if ts.members == nil {
		ts.members = make(map[string]bool)
	}
	ts.members[sessionID] = parked
	newTaskParked := anyParkedMember(ts.members)
	changed := newTaskParked != ts.parked
	if changed {
		ts.parked = newTaskParked
		ts.revision++
	}
	ts.mu.Unlock()

	if changed {
		s.publishTaskParkedChanged(ctx, taskID)
	}
}

// anyParkedMember returns true if any member value is true.
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

// ParkedProjectionSnapshot implements dto.ParkedProjectionProvider.
func (s *Service) ParkedProjectionSnapshot(sessionID string) (bool, int64, uint64) {
	s.parkedStatesMu.RLock()
	ps := s.parkedStates[sessionID]
	if ps == nil {
		s.parkedStatesMu.RUnlock()
		return false, s.parkedEpoch, 0
	}
	parked, revision := ps.parked, ps.revision
	s.parkedStatesMu.RUnlock()
	return parked, s.parkedEpoch, revision
}

// TaskParkedProjectionSnapshot implements dto.TaskParkedProjectionProvider.
func (s *Service) TaskParkedProjectionSnapshot(taskID string) (bool, uint64) {
	s.taskParkedStatesMu.Lock()
	ts := s.taskParkedStates[taskID]
	s.taskParkedStatesMu.Unlock()
	if ts == nil {
		return false, 0
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.parked, ts.revision
}
