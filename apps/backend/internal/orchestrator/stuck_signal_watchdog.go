package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Stuck-signal watchdog.
//
// A session can accept a step_complete_kandev signal (written into the
// session's metadata bag) and then never reach turn-end AND never fail —
// the agent process simply stops producing events. Neither existing
// watchdog covers this: the in-process stall ticker
// (agent/runtime/lifecycle/session.go) dies with the goroutine/process and
// is advisory only even when it fires, and the idle-session reaper
// (idle_session_reaper.go) excludes RUNNING by construction. The session
// row sits at RUNNING forever, holding a WIP slot, with an
// accepted-but-unapplied completion signal.
//
// Recovery is also blocked by the same state: flipStaleRunningToWaiting
// declines while activeTurns still has an entry for the session (the stuck
// turn never deregisters), and queueAutoStartPromptIfRunning queues the
// auto-start prompt into a turn-end that never arrives instead of sending
// it. A card in this state cannot be rescued by an ordinary board move.
//
// This watchdog scans for that exact state — RUNNING/STARTING, a pending
// signal for the task's CURRENT step, older than
// stuckSignalWatchdogThreshold — and reclaims it: force-close the stale
// turn (which also drops the activeTurns entry), flip the session to
// WAITING_FOR_INPUT (a state the idle reaper already knows how to release,
// and that unblocks queueAutoStartPromptIfRunning's send-not-queue branch),
// then hand off to reconcileStepCompletionSignalLocked so the transition
// the agent asked for is actually applied rather than just unblocked.
//
// Deliberately narrow, matching the exclusions already used by
// reconcileStepCompletionSignalLocked's other caller
// (handleRecoverableFailureLocked, PR #2963): only a session holding a
// signal for the task's current step is touched. A long turn that never
// signalled at all is out of scope and keeps running. Office sessions are
// excluded — this path must not advance an Office task's step.
const (
	// stuckSignalWatchdogThreshold is how long a pending completion signal
	// may sit unapplied on a RUNNING/STARTING session before the watchdog
	// treats it as a reclaim candidate. Chosen as 2x the in-process stall
	// ticker's 5-minute threshold (so this watchdog never races that
	// advisory report), above the 0-9 minute range measured for healthy
	// sessions, and roughly 100x the expected signal-to-turn-end latency
	// (step_complete_kandev is documented as the agent's last action in a
	// turn, so that gap should normally be seconds).
	//
	// Signal age alone is not inactivity: a session can clear this gate
	// while its agent process is still genuinely producing events (a long
	// tool call, a provider retry/backoff that outlasts the threshold).
	// Candidacy on age is therefore only the first filter — the actual
	// reclaim additionally requires that the session's tracked prompt
	// activity has been quiet for at least this same threshold; see
	// stuckSignalInactiveLongEnough.
	stuckSignalWatchdogThreshold = 10 * time.Minute
)

// stuckSignalSessionLister is implemented by the durable repository via
// ListActiveTaskSessions. A narrow optional interface — mirrors
// idleExecutorCandidateLister in idle_session_reaper.go — so lightweight
// test doubles that don't exercise this scan stay compatible.
type stuckSignalSessionLister interface {
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
}

// reclaimStuckSignalSessionsOnce is the per-tick scan, called from the
// idle-session reaper's existing ticker (idle_session_reaper.go) rather
// than owning a second background goroutine. Each candidate session is
// reclaimed independently and best-effort: a failure to reclaim one session
// is logged and does not stop the scan from checking the rest.
func (s *Service) reclaimStuckSignalSessionsOnce(ctx context.Context) {
	lister, ok := s.repo.(stuckSignalSessionLister)
	if !ok {
		return
	}
	sessions, err := lister.ListActiveTaskSessions(ctx)
	if err != nil {
		s.logger.Warn("stuck-signal watchdog: list active sessions failed; tick skipped", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, session := range sessions {
		s.reclaimStuckSignalSessionIfDue(ctx, session, now)
	}
}

// reclaimStuckSignalSessionIfDue filters a single session against the
// watchdog's criteria and, if it qualifies, reclaims it. The active-turn
// guard is re-checked under the session's cancelInFlight lock (the same
// lock onStepCompletionSignaled holds) after a fresh re-read, so a session
// that resolves itself (turn-end or failure event) between the unguarded
// scan and here is left alone. The inactivity gate is read fresh inside that
// same locked section, immediately before the reclaim fires — see
// stuckSignalInactiveLongEnough — rather than captured earlier and compared
// for change, so there is no separate scan-to-reclaim window for a live
// agent's activity to race.
func (s *Service) reclaimStuckSignalSessionIfDue(ctx context.Context, session *models.TaskSession, now time.Time) {
	task, signal, ok := s.stuckSignalCandidate(ctx, session, now)
	if !ok {
		return
	}

	lock, release := s.acquireCancelInFlightGuard(session.ID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(session.ID) {
		return
	}
	latest, ok := s.stuckSignalStillPending(ctx, session.ID, signal.StepID)
	if !ok {
		return
	}
	if !s.stuckSignalInactiveLongEnough(ctx, session.ID, now) {
		s.logger.Debug("stuck-signal watchdog: skipping reclaim, agent activity observed within the inactivity window",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.String("step_id", signal.StepID))
		return
	}

	s.logger.Warn("stuck-signal watchdog: reclaiming session stuck RUNNING with an unapplied completion signal",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("step_id", signal.StepID),
		zap.Duration("signal_age", now.Sub(signal.SignaledAt)))

	s.completeTurnForSession(ctx, session.ID)
	s.updateTaskSessionState(ctx, task.ID, session.ID, models.TaskSessionStateWaitingForInput, "", false, latest)
	s.reconcileStepCompletionSignalLocked(ctx, task.ID, session.ID, signal.StepID)
}

// stuckSignalCandidate applies the unguarded eligibility filter: RUNNING or
// STARTING state, a pending signal older than stuckSignalWatchdogThreshold,
// matching the task's CURRENT step, on a non-Office task. Office sessions go
// FAILED, not WAITING_FOR_INPUT, and must not advance their step from this
// path — same exclusion PR #2963 applies to
// handleRecoverableFailureLocked's reconciliation hook.
func (s *Service) stuckSignalCandidate(
	ctx context.Context, session *models.TaskSession, now time.Time,
) (*models.Task, models.PendingStepCompletionSignal, bool) {
	if session == nil || session.ID == "" {
		return nil, models.PendingStepCompletionSignal{}, false
	}
	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		return nil, models.PendingStepCompletionSignal{}, false
	}
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	if !ok || now.Sub(signal.SignaledAt) < stuckSignalWatchdogThreshold {
		return nil, models.PendingStepCompletionSignal{}, false
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return nil, models.PendingStepCompletionSignal{}, false
	}
	// Only a signal that matches the task's CURRENT step is this watchdog's
	// job. A stale signal (step already moved on) is left for the ordinary
	// signal-consuming paths, which clear a bag entry that no longer
	// matches the current step.
	if task.WorkflowStepID != signal.StepID || task.IsFromOffice {
		return nil, models.PendingStepCompletionSignal{}, false
	}
	return task, signal, true
}

// stuckSignalStillPending re-reads the session under the cancelInFlight
// lock and confirms it still qualifies — guards against a session that
// resolved itself (turn-end or failure event) between the unguarded scan
// and the lock being acquired.
func (s *Service) stuckSignalStillPending(ctx context.Context, sessionID, stepID string) (*models.TaskSession, bool) {
	latest, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || latest == nil {
		return nil, false
	}
	if latest.State != models.TaskSessionStateRunning && latest.State != models.TaskSessionStateStarting {
		return nil, false
	}
	if latestSignal, stillPending := models.LoadPendingStepSignal(latest.Metadata); !stillPending || latestSignal.StepID != stepID {
		return nil, false
	}
	return latest, true
}

// promptActivitySessionReader is implemented by the lifecycle manager. A
// narrow optional interface — mirrors the generation-reader interface
// task_operations.go's captureCancellationAgentIdentity asserts for — so
// lightweight test doubles that don't track prompt activity stay
// compatible. In production, backendapp.lifecycleAdapter forwards this
// method and its shape is pinned at compile time (adapters.go) precisely so
// a missing or drifted forward is a build failure rather than a silently
// dead guard here.
type promptActivitySessionReader interface {
	GetPromptActivityForSession(ctx context.Context, sessionID string) (executionID string, generation, activityEpoch uint64, lastActivityAt time.Time, err error)
}

// stuckSignalInactiveLongEnough reports whether the session's tracked prompt
// activity has been quiet for at least stuckSignalWatchdogThreshold — the
// card's actual "no activity for N minutes" requirement. Called under the
// cancelInFlight lock, immediately before the reclaim fires, so it reads the
// freshest possible activity state for whatever execution the session
// currently owns; there is no separate unguarded-capture step to race,
// because the read happens inside the same critical section as the reclaim
// itself.
//
// Three cases:
//   - s.agentManager doesn't implement promptActivitySessionReader: in
//     production this cannot happen (see the compile-time pin in
//     backendapp/adapters.go), but a lightweight test double may still omit
//     it deliberately. Treated as fail-open — no evidence either way, so the
//     signal-age filter that already ran in stuckSignalCandidate decides.
//   - ErrNoExecutionForSession (nothing tracked for the session — process
//     gone, backend restarted): fail open, reclaim. There is no live turn
//     left to protect.
//   - An execution is tracked: reclaim only once
//     now.Sub(lastActivityAt) >= stuckSignalWatchdogThreshold. A session
//     whose signal is old but whose agent is still emitting events within
//     the window is left alone — this is the case a signal-age-only check,
//     or an epoch-equality check across a sub-millisecond scan window, both
//     get wrong.
func (s *Service) stuckSignalInactiveLongEnough(ctx context.Context, sessionID string, now time.Time) bool {
	reader, ok := s.agentManager.(promptActivitySessionReader)
	if !ok {
		return true
	}
	_, _, _, lastActivityAt, err := reader.GetPromptActivityForSession(ctx, sessionID)
	if err != nil {
		return true
	}
	return now.Sub(lastActivityAt) >= stuckSignalWatchdogThreshold
}
