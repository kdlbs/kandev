package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// parkedSessionState is the session-scoped parked projection (spec:
// docs/specs/disambiguate-waiting/spec.md, "Data model -> Parked
// projection"). There is deliberately no `sampling` field — see the spec's
// own justification for why that field was removed rather than restated.
type parkedSessionState struct {
	parked        bool
	revision      uint64
	lastSample    executor.ProbeResult
	lastSampledAt time.Time

	// loopCancel stops this session's periodic sampling goroutine (AC-53).
	// Non-nil only while parked is true and a loop is running for it.
	loopCancel context.CancelFunc
	// loopToken identifies the worker that owns loopCancel. A worker clears
	// both fields on exit only when it still owns the slot, so a newer loop
	// cannot be cleared by an older worker finishing late.
	loopToken *struct{}
}

// taskParkedState is the task-level parked_on_background_work OR-aggregate
// (spec: "Data model -> Task-level projection"). sessions tracks only the
// member sessions currently parked (true); the aggregate is len(sessions) > 0
// so no session ever needs an explicit false entry removed on task deletion.
type taskParkedState struct {
	sessions map[string]bool
	parked   bool
	revision uint64
}

type parkedProjectionTransition struct {
	parked       bool
	revision     uint64
	taskParked   bool
	taskRevision uint64
	taskChanged  bool
}

// serviceBackgroundProbeAdapter is the production BackgroundProbe: it
// forwards to Service.ProbeBackgroundWorkloads (the F6-guarded, budget-bound
// port from task-04). A distinct type keeps *Service from needing its own
// Probe method, which would collide with the more specific
// ProbeBackgroundWorkloads name used across the executor/lifecycle chain.
type serviceBackgroundProbeAdapter struct{ s *Service }

func (a serviceBackgroundProbeAdapter) Probe(ctx context.Context, sessionID string) (executor.ProbeResult, error) {
	return a.s.ProbeBackgroundWorkloads(ctx, sessionID)
}

func (s *Service) parkedProbeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := s.backgroundProbeConfig.Budget
	if budget <= 0 {
		budget = defaultParkedProbeBudget
	}
	return context.WithTimeout(ctx, budget)
}

// SetBackgroundProbe overrides the BackgroundProbe port. Test-only seam: the
// projection tests script a fixed live/settled/unknown sequence against this
// interface rather than depending on the real transport or process walk (see
// docs/plans/disambiguate-waiting/task-05-parked-projection.md).
func (s *Service) SetBackgroundProbe(probe BackgroundProbe) {
	s.backgroundProbe = probe
}

// ParkedSnapshot returns sessionID's parked_on_background_work projection and
// its process-local transition revision from one critical section (D1),
// mirroring CancellationPendingSnapshot (task_operations.go). A session with
// no recorded transition reads as (false, 0) per D9.
func (s *Service) ParkedSnapshot(sessionID string) (bool, uint64) {
	if sessionID == "" {
		return false, 0
	}
	s.parkedMu.Lock()
	defer s.parkedMu.Unlock()
	st := s.parkedStates[sessionID]
	if st == nil {
		return false, 0
	}
	return st.parked, st.revision
}

// ParkedEpoch returns this process's parked-projection epoch: its start time
// in Unix nanoseconds, fixed for the process's life (spec: "Revision epoch").
func (s *Service) ParkedEpoch() uint64 {
	return s.parkedEpoch
}

// TaskParkedSnapshot returns taskID's task-level parked_on_background_work
// OR-aggregate and its own monotonic revision, read from one critical section
// (spec: "Data model -> Task-level projection"). A task with no tracked
// transition reads as (false, 0) per D9 — this covers both "no sessions" and
// "sessions exist but none ever parked" (AC-50).
func (s *Service) TaskParkedSnapshot(taskID string) (bool, uint64) {
	if taskID == "" {
		return false, 0
	}
	s.parkedMu.Lock()
	defer s.parkedMu.Unlock()
	tst := s.taskParkedStates[taskID]
	if tst == nil {
		return false, 0
	}
	return tst.parked, tst.revision
}

// parkedStateLocked returns sessionID's parked state, lazily creating the map
// and the entry. Callers must hold parkedMu. The map is not guaranteed
// non-nil at construction — *Service values built outside NewService (tests,
// or any future direct-literal construction) must not panic here.
func (s *Service) parkedStateLocked(sessionID string) *parkedSessionState {
	if s.parkedStates == nil {
		s.parkedStates = make(map[string]*parkedSessionState)
	}
	st, ok := s.parkedStates[sessionID]
	if !ok {
		st = &parkedSessionState{}
		s.parkedStates[sessionID] = st
	}
	return st
}

// recomputeTaskParkedLocked applies sessionID's freshly-recomputed parked
// value to taskID's OR-aggregate and returns the resulting task-level
// projection. Callers must hold parkedMu, and must call this only when the
// session-level value actually changed — the task-level revision increments
// once per observed change of the task-level boolean, never per session
// transition (AC-49: two sessions toggling independently must not let the
// task's counter run ahead of its own actual flips).
func (s *Service) recomputeTaskParkedLocked(taskID, sessionID string, sessionParked bool) (parked bool, revision uint64, changed bool) {
	if taskID == "" {
		return false, 0, false
	}
	if s.taskParkedStates == nil {
		s.taskParkedStates = make(map[string]*taskParkedState)
	}
	tst, ok := s.taskParkedStates[taskID]
	if !ok {
		tst = &taskParkedState{sessions: make(map[string]bool)}
		s.taskParkedStates[taskID] = tst
	}
	if sessionParked {
		tst.sessions[sessionID] = true
	} else {
		delete(tst.sessions, sessionID)
	}
	newParked := len(tst.sessions) > 0
	changed = newParked != tst.parked
	if changed {
		tst.revision++
		tst.parked = newParked
	}
	return tst.parked, tst.revision, changed
}

// recomputeParkedLocked applies the three-term formula (spec: "Data model ->
// Parked projection") to st and returns the resulting projection. Callers
// must hold parkedMu. hasSample controls whether sample/sampledAt overwrite
// st's stored values — false is how D8's session-state term clears the
// projection without forcing (or waiting for) a new probe sample.
func recomputeParkedLocked(
	st *parkedSessionState,
	attested bool,
	sample executor.ProbeResult,
	hasSample bool,
	state models.TaskSessionState,
) (parked bool, revision uint64, changed bool) {
	if hasSample {
		st.lastSample = sample
		st.lastSampledAt = time.Now()
	}
	newParked := attested && st.lastSample == executor.ProbeResultLive && state == models.TaskSessionStateWaitingForInput
	changed = newParked != st.parked
	if changed {
		st.revision++
		st.parked = newParked
	}
	return st.parked, st.revision, changed
}

// settleParkedProjectionSync is D2's synchronous first sample: taken during
// turn-settle handling, only when observed_detached is true for the turn
// that just settled (AC-40a — otherwise zero probe latency), bounded by
// KANDEV_PARKED_PROBE_BUDGET via the BackgroundProbe port's own
// context.WithTimeout (task-04). Must be called only after this turn's
// session.turn_finished has already been published (F7) — see the call site
// in updateTaskSessionStateWithHook, which runs after completeTurnForTaskSession.
//
// F9: a concurrent D8 transition (self-resume, cancel, stop, delete) can
// land while this synchronous probe is still in flight. The dispatch
// revision is captured before the probe call
// and re-checked after under the same lock discipline sampleParkedSessionTick
// already uses, and the session's state is re-read fresh at apply time rather
// than trusted from the moment this settle began — a session that left
// WAITING_FOR_INPUT during the probe must not be re-parked by a late result.
func (s *Service) settleParkedProjectionSync(ctx context.Context, taskID, sessionID string) {
	if sessionID == "" {
		return
	}
	attested := s.ObservedDetachedLaunch(sessionID)
	if !attested {
		s.applyParkedTransition(ctx, taskID, sessionID, false, executor.ProbeResultUnknown, false, models.TaskSessionStateWaitingForInput)
		return
	}
	probe := s.backgroundProbe
	if probe == nil {
		return
	}

	s.parkedMu.Lock()
	stateAtDispatch, hadState := s.parkedStates[sessionID]
	dispatchRevision := uint64(0)
	if stateAtDispatch != nil {
		dispatchRevision = stateAtDispatch.revision
	}
	s.parkedMu.Unlock()

	probeCtx, cancel := s.parkedProbeContext(ctx)
	defer cancel()
	result, err := probe.Probe(probeCtx, sessionID)
	if err != nil {
		s.logger.Warn("background probe failed during turn-settle",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	session, sessErr := s.repo.GetTaskSession(probeCtx, sessionID)
	if sessErr != nil || session == nil {
		// The session is gone or unreadable by the time the probe returned;
		// there is nothing current to apply this sample against.
		return
	}

	transition, changed := s.applySettledParkedSample(
		taskID,
		sessionID,
		result,
		session.State,
		hadState,
		dispatchRevision,
	)
	if !changed {
		return
	}
	s.publishParkedTransition(ctx, taskID, sessionID, transition.parked, transition.revision)
	if transition.taskChanged {
		s.publishTaskParkedTransition(ctx, taskID, transition.taskParked, transition.taskRevision)
	}
	if transition.parked {
		s.maybeStartParkedSamplingLoop(taskID, sessionID)
	} else {
		s.stopParkedSamplingLoopFor(sessionID)
	}
}

func (s *Service) applySettledParkedSample(
	taskID, sessionID string,
	sample executor.ProbeResult,
	state models.TaskSessionState,
	hadState bool,
	dispatchRevision uint64,
) (parkedProjectionTransition, bool) {
	s.parkedMu.Lock()
	defer s.parkedMu.Unlock()

	st, ok := s.parkedStates[sessionID]
	if !ok {
		if hadState || sample != executor.ProbeResultLive || state != models.TaskSessionStateWaitingForInput {
			return parkedProjectionTransition{}, false
		}
		st = s.parkedStateLocked(sessionID)
	} else if hadState && st.revision != dispatchRevision {
		return parkedProjectionTransition{}, false
	}

	parked, revision, changed := recomputeParkedLocked(st, true, sample, true, state)
	transition := parkedProjectionTransition{parked: parked, revision: revision}
	if !changed {
		return transition, false
	}
	transition.taskParked, transition.taskRevision, transition.taskChanged = s.recomputeTaskParkedLocked(taskID, sessionID, parked)
	return transition, true
}

// clearParkedOnSessionStateLeft is D8's session-state term: when a session
// leaves WAITING_FOR_INPUT (self-resume, an admitted prompt, or the session
// stopping/ending), the projection clears immediately without a new probe
// sample — last_sample may still read live and is never re-read for this
// transition (AC-68).
func (s *Service) clearParkedOnSessionStateLeft(ctx context.Context, taskID, sessionID string, newState models.TaskSessionState) {
	if sessionID == "" {
		return
	}
	if newState == "" {
		s.clearParkedProjectionOnSessionDeleted(ctx, taskID, sessionID)
		return
	}
	attested := s.ObservedDetachedLaunch(sessionID)
	s.applyParkedTransition(ctx, taskID, sessionID, attested, "", false, newState)
}

// clearParkedProjectionOnSessionDeleted removes all session-level projection
// state after the repository row is deleted. The task-level state keeps its
// last revision as a tombstone so a later session on the same task cannot
// publish an event with a revision lower than the deletion update.
func (s *Service) clearParkedProjectionOnSessionDeleted(ctx context.Context, taskID, sessionID string) {
	if sessionID == "" {
		return
	}
	s.clearObservedDetachedLaunch(sessionID)

	s.parkedMu.Lock()
	st := s.parkedStates[sessionID]
	var cancel context.CancelFunc
	if st != nil {
		cancel = st.loopCancel
		delete(s.parkedStates, sessionID)
	}

	var taskParked bool
	var taskRevision uint64
	var taskChanged bool
	if taskID != "" {
		if tst := s.taskParkedStates[taskID]; tst != nil {
			delete(tst.sessions, sessionID)
			newParked := len(tst.sessions) > 0
			if newParked != tst.parked {
				tst.parked = newParked
				tst.revision++
				taskParked, taskRevision, taskChanged = newParked, tst.revision, true
			}
		}
	}
	s.parkedMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if taskChanged {
		s.publishTaskParkedTransition(ctx, taskID, taskParked, taskRevision)
	}
}

// onSessionStateChangedForParkedProjection is the single dispatcher wired
// into updateTaskSessionStateWithHook, the chokepoint every session-state
// transition passes through. It covers AC-21 (entering WAITING_FOR_INPUT)
// regardless of which of the many call sites drove the transition, and
// AC-68 (leaving WAITING_FOR_INPUT) the same way.
func (s *Service) onSessionStateChangedForParkedProjection(
	ctx context.Context,
	taskID, sessionID string,
	oldState, nextState models.TaskSessionState,
) {
	if sessionID == "" {
		return
	}
	switch {
	case nextState == models.TaskSessionStateWaitingForInput && oldState != models.TaskSessionStateWaitingForInput:
		s.settleParkedProjectionSync(ctx, taskID, sessionID)
	case oldState == models.TaskSessionStateWaitingForInput && nextState != models.TaskSessionStateWaitingForInput:
		s.clearParkedOnSessionStateLeft(ctx, taskID, sessionID, nextState)
	}
}

// applyParkedTransition is the shared locked-recompute-then-publish path for
// both the synchronous settle sample and the D8 session-state clear. The
// periodic sampling loop uses its own variant (sampleParkedSessionTick)
// because it also needs the F9 stale-sample discard.
func (s *Service) applyParkedTransition(
	ctx context.Context,
	taskID, sessionID string,
	attested bool,
	sample executor.ProbeResult,
	hasSample bool,
	state models.TaskSessionState,
) {
	s.parkedMu.Lock()
	st := s.parkedStates[sessionID]
	if st == nil && (!attested || !hasSample || sample != executor.ProbeResultLive || state != models.TaskSessionStateWaitingForInput) {
		s.parkedMu.Unlock()
		return
	}
	if st == nil {
		st = s.parkedStateLocked(sessionID)
	}
	parked, revision, changed := recomputeParkedLocked(st, attested, sample, hasSample, state)
	var taskParked, taskChanged bool
	var taskRevision uint64
	if changed {
		taskParked, taskRevision, taskChanged = s.recomputeTaskParkedLocked(taskID, sessionID, parked)
	}
	s.parkedMu.Unlock()

	if !changed {
		return
	}
	s.publishParkedTransition(ctx, taskID, sessionID, parked, revision)
	if taskChanged {
		s.publishTaskParkedTransition(ctx, taskID, taskParked, taskRevision)
	}
	if parked {
		s.maybeStartParkedSamplingLoop(taskID, sessionID)
	} else {
		s.stopParkedSamplingLoopFor(sessionID)
		if state == "" || isTerminalSessionState(state) {
			s.pruneParkedTerminalState(taskID, sessionID)
		}
	}
}

// pruneParkedTerminalState drops session state after a terminal or deleted
// session has stopped sampling. Task state remains as a revision tombstone to
// preserve event ordering if the task later receives another session.
func (s *Service) pruneParkedTerminalState(taskID, sessionID string) {
	s.parkedMu.Lock()
	if st := s.parkedStates[sessionID]; st != nil && !st.parked && st.loopCancel == nil {
		delete(s.parkedStates, sessionID)
	}
	if taskID != "" {
		if tst := s.taskParkedStates[taskID]; tst != nil {
			delete(tst.sessions, sessionID)
		}
	}
	s.parkedMu.Unlock()
}

// publishParkedTransition emits task_session.activity_changed (wire:
// session.activity_changed) for a parked transition (AC-68, D1). Task-06
// adds the conditional task.updated publish and the full DTO wiring; this is
// the session-only half AC-68 itself observes.
func (s *Service) publishParkedTransition(ctx context.Context, taskID, sessionID string, parked bool, revision uint64) {
	if s.eventBus == nil || sessionID == "" {
		return
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:               taskID,
		metaKeySessionID:            sessionID,
		"parked_on_background_work": parked,
		"revision":                  revision,
		"parked_epoch":              s.parkedEpoch,
	}
	if err := s.eventBus.Publish(ctx, events.TaskSessionActivityChanged,
		bus.NewEvent(events.TaskSessionActivityChanged, "task-session", eventData)); err != nil {
		s.logger.Warn("publish parked_on_background_work transition failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Bool("parked_on_background_work", parked),
			zap.Uint64("revision", revision),
			zap.Error(err))
	}
}

// publishTaskParkedTransition emits task.updated when the task-level
// parked_on_background_work OR-aggregate changes (AC-62, AC-78). Unlike
// publishTaskActivityIfChanged (task/service), the caller here already knows
// from the locked recompute that the aggregate changed, so this always
// publishes when called rather than recomputing and deduplicating again.
func (s *Service) publishTaskParkedTransition(ctx context.Context, taskID string, taskParked bool, taskRevision uint64) {
	if s.taskEvents == nil || taskID == "" || s.repo == nil {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			s.logger.Warn("failed to load task for parked transition",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return
	}
	s.logger.Debug("task-level parked_on_background_work transition",
		zap.String("task_id", taskID),
		zap.Bool("parked_on_background_work", taskParked),
		zap.Uint64("parked_revision", taskRevision))
	s.publishTaskUpdated(ctx, task)
}

// maybeStartParkedSamplingLoop starts the per-session sampling goroutine
// (AC-53) when the configured interval is positive. KANDEV_PARKED_PROBE_INTERVAL
// = 0 disables periodic sampling entirely (D9); the projection then clears
// only via the session-state or attestation terms.
func (s *Service) maybeStartParkedSamplingLoop(taskID, sessionID string) {
	interval := s.backgroundProbeConfig.Interval
	if interval <= 0 {
		return
	}
	s.parkedMu.Lock()
	st := s.parkedStates[sessionID]
	if st == nil || st.loopCancel != nil {
		s.parkedMu.Unlock()
		return
	}
	s.parkedLoopMu.Lock()
	if s.parkedLoopStopped {
		s.parkedLoopMu.Unlock()
		s.parkedMu.Unlock()
		return
	}
	rootCtx := s.parkedLoopCtx
	loopCtx, cancel := context.WithCancel(rootCtx)
	// Add while parkedLoopMu is held. Service.Stop takes the same mutex before
	// waiting, so it cannot observe a zero count and return before this worker
	// is registered.
	s.parkedLoopWorkers.Add(1)
	s.parkedLoopMu.Unlock()

	st.loopCancel = cancel
	st.loopToken = &struct{}{}
	loopToken := st.loopToken
	s.parkedMu.Unlock()

	go func() {
		defer s.parkedLoopWorkers.Done()
		defer s.releaseParkedSamplingLoopSlot(sessionID, loopToken, cancel)
		s.runParkedSamplingLoop(loopCtx, taskID, sessionID, interval)
	}()
}

func (s *Service) releaseParkedSamplingLoopSlot(sessionID string, loopToken *struct{}, cancel context.CancelFunc) {
	s.parkedMu.Lock()
	if st := s.parkedStates[sessionID]; st != nil && st.loopToken == loopToken {
		st.loopToken = nil
		st.loopCancel = nil
	}
	s.parkedMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// stopParkedSamplingLoopFor cancels sessionID's sampling goroutine, if any.
// Safe to call whether or not a loop is currently running.
func (s *Service) stopParkedSamplingLoopFor(sessionID string) {
	s.parkedMu.Lock()
	st := s.parkedStates[sessionID]
	var cancel context.CancelFunc
	if st != nil {
		cancel = st.loopCancel
		st.loopCancel = nil
		st.loopToken = nil
	}
	s.parkedMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// stopParkedSamplingLoops cancels every running sampling loop and waits for
// them to drain (AC-53's backend-shutdown exit). Called from Service.Stop.
func (s *Service) stopParkedSamplingLoops() {
	s.parkedLoopMu.Lock()
	s.parkedLoopStopped = true
	cancel := s.parkedLoopCancel
	s.parkedLoopMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.parkedLoopWorkers.Wait()
}

// runParkedSamplingLoop is the per-session sampling goroutine body: one
// probe per configured interval until a tick reports the session should no
// longer be sampled, or the loop's context is cancelled (session left
// WAITING_FOR_INPUT, was stopped, or the backend is shutting down).
func (s *Service) runParkedSamplingLoop(ctx context.Context, taskID, sessionID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.sampleParkedSessionTick(ctx, taskID, sessionID) {
				return
			}
		}
	}
}

// sampleParkedSessionTick takes one periodic sample and applies it, and
// reports whether the loop should keep running. F9: the session state and
// the parked-state revision are both re-read fresh after the probe
// completes, and the sample is discarded (loop stops, nothing is published)
// if the revision moved while the probe was in flight — e.g. a self-resume
// raced ahead and already cleared the projection via the session-state term.
func (s *Service) sampleParkedSessionTick(ctx context.Context, taskID, sessionID string) bool {
	s.parkedMu.Lock()
	st, ok := s.parkedStates[sessionID]
	if !ok || !st.parked {
		s.parkedMu.Unlock()
		return false
	}
	dispatchRevision := st.revision
	s.parkedMu.Unlock()

	probe := s.backgroundProbe
	if probe == nil {
		return false
	}
	probeCtx, cancel := s.parkedProbeContext(ctx)
	defer cancel()
	result, err := probe.Probe(probeCtx, sessionID)
	if err != nil {
		s.logger.Warn("background probe failed during periodic sampling",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	session, sessErr := s.repo.GetTaskSession(probeCtx, sessionID)
	if sessErr != nil || session == nil {
		// The session is gone: stop looping and clear the projection rather
		// than keep sampling a deleted session (AC-53).
		s.applyParkedTransition(ctx, taskID, sessionID, false, executor.ProbeResultUnknown, true, "")
		return false
	}
	attested := s.ObservedDetachedLaunch(sessionID)

	s.parkedMu.Lock()
	st, ok = s.parkedStates[sessionID]
	if !ok || st.revision != dispatchRevision {
		s.parkedMu.Unlock()
		return false
	}
	parked, revision, changed := recomputeParkedLocked(st, attested, result, true, session.State)
	var taskParked, taskChanged bool
	var taskRevision uint64
	if changed {
		taskParked, taskRevision, taskChanged = s.recomputeTaskParkedLocked(taskID, sessionID, parked)
	}
	s.parkedMu.Unlock()

	if changed {
		s.publishParkedTransition(ctx, taskID, sessionID, parked, revision)
		if taskChanged {
			s.publishTaskParkedTransition(ctx, taskID, taskParked, taskRevision)
		}
	}
	return parked
}
