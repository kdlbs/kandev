package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
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
// excluded — this path must not advance an Office task's step. Passthrough
// (PTY) sessions are excluded too, mirroring the guard
// flipStaleRunningToWaiting and markIdleAfterReset already apply
// (event_handlers_workflow.go): a passthrough session manages its own
// RUNNING/idle transitions, and none of the ACP-only lastActivityAt writers
// ever run for it, so stuckSignalInactiveLongEnough has no real activity
// signal to read for one. Making lastActivityAt meaningful for passthrough
// is out of scope here — it needs an activity source that doesn't exist yet.
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

	// stuckSignalScanBudget bounds how long a single
	// reclaimStuckSignalSessionsOnce tick may run. Reclaiming one session can
	// block for several seconds settling a stuck execution (see
	// cancelAgentWhileUnlocked's CancelAgent wait); without a cap, enough
	// simultaneously-stuck sessions could stall the shared reaper goroutine's
	// other duty (reclaimIdleSessionsOnce) far past its own 30s tick cadence
	// (idleReaperInterval, idle_session_reaper.go). Chosen comfortably under
	// that cadence so a capped tick still leaves room before the next one.
	stuckSignalScanBudget = 20 * time.Second
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
	scanCtx, cancel := context.WithTimeout(ctx, stuckSignalScanBudget)
	defer cancel()
	now := time.Now().UTC()
	for i, session := range sessions {
		if scanCtx.Err() != nil {
			s.logger.Warn("stuck-signal watchdog: per-tick scan budget exceeded; deferring remaining candidates to the next tick",
				zap.Int("candidates_scanned", i),
				zap.Int("candidates_deferred", len(sessions)-i))
			return
		}
		if s.reconcileWaitingStuckSignalSessionIfDue(scanCtx, session, now) {
			continue
		}
		s.reclaimStuckSignalSessionIfDue(scanCtx, session, now)
	}
}

// reconcileWaitingStuckSignalSessionIfDue retries an accepted signal after a
// prior reclaim already moved the session to WAITING_FOR_INPUT but could not
// complete the workflow transition. The pending signal is durable, so a later
// tick can safely retry without cancelling another agent turn.
func (s *Service) reconcileWaitingStuckSignalSessionIfDue(ctx context.Context, session *models.TaskSession, now time.Time) bool {
	if session == nil || session.State != models.TaskSessionStateWaitingForInput {
		return false
	}
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	if !ok || now.Sub(signal.SignaledAt) < stuckSignalWatchdogThreshold {
		return false
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil || task.WorkflowStepID != signal.StepID || task.IsFromOffice || session.IsPassthrough {
		return false
	}
	guard := s.lockCancelInFlightGuard(session.ID)
	defer guard.release()
	if s.isCancelInFlight(session.ID) {
		return true
	}
	_, stillPending := s.stuckSignalWaitingStillPending(ctx, session.ID, signal.StepID)
	if !stillPending {
		return true
	}
	s.reconcileStepCompletionSignalLocked(ctx, task.ID, session.ID, signal.StepID)
	return true
}

func (s *Service) stuckSignalWaitingStillPending(ctx context.Context, sessionID, stepID string) (*models.TaskSession, bool) {
	latest, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || latest == nil || latest.State != models.TaskSessionStateWaitingForInput {
		return nil, false
	}
	signal, ok := models.LoadPendingStepSignal(latest.Metadata)
	if !ok || signal.StepID != stepID {
		return nil, false
	}
	return latest, true
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

	guard := s.lockCancelInFlightGuard(session.ID)
	defer guard.release()
	if s.isCancelInFlight(session.ID) {
		return
	}
	if _, ok := s.stuckSignalStillPending(ctx, session.ID, signal.StepID); !ok {
		return
	}
	activity, inactive := s.stuckSignalInactiveLongEnough(ctx, session.ID, now)
	if !inactive {
		s.logger.Debug("stuck-signal watchdog: skipping reclaim, agent activity observed within the inactivity window",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.String("step_id", signal.StepID))
		return
	}

	// Register this reclaim as the session's cancellation owner before
	// yielding the guard below (mirroring runSilentCancellationOwned's use of
	// claimCancellationWithActionExclusive in event_handlers_clarification.go).
	// Without this, a user's concurrent Cancel click
	// (Service.CancelAgent -> claimExplicitCancellation) finds an empty
	// cancellationOperations entry for the session and independently
	// re-invokes agentManager.CancelAgent against the same execution while
	// this reclaim's own call is still in flight below — the
	// isCancelInFlight check above only reads that same map, so without
	// registering here it can never observe this reclaim's own operation
	// either. Exclusive (not joining): a session that already has an owner
	// registered resolved the isCancelInFlight race above in the other
	// operation's favor, so this tick backs off rather than attaching to a
	// cancellation it did not initiate.
	operation, owner, _, accepted := s.claimCancellationWithActionExclusive(session.ID, cancellationKindInternal, nil)
	if !accepted || !owner {
		return
	}

	// reclaimStuckSignalSessionOwned always releases guard itself (its own
	// defer) before returning, so by the time finishCancellationWithActions
	// runs below the per-session cancelInFlight mutex is already free.
	// This ordering matters: finishCancellationWithActions re-acquires that
	// same mutex to run any action a concurrent joiner (e.g. a user's
	// Service.CancelAgent click joining this operation) registered on it
	// (task_operations.go). Calling finishCancellationWithActions while this
	// goroutine still held the mutex — as a single defer registered here
	// alongside guard's — would self-deadlock the very first time SEC-001's
	// join path actually has an action to run. Mirrors runSilentCancellation
	// calling finishCancellationWithActions only after
	// runSilentCancellationOwned's own deferred guard.release() has already
	// run (event_handlers_clarification.go); the outer defer guard.release()
	// above is a harmless no-op once the owned call already released it.
	reclaimErr := s.reclaimStuckSignalSessionOwned(ctx, task, session, signal, activity, now, operation, guard)
	s.finishCancellationWithActions(ctx, session.ID, operation, reclaimErr)
}

// reclaimStuckSignalSessionOwned performs the reclaim once
// reclaimStuckSignalSessionIfDue has established exclusive cancellation
// ownership of the session. It always releases guard before returning; see
// the caller for why finishCancellationWithActions must not run until this
// has happened.
func (s *Service) reclaimStuckSignalSessionOwned(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	signal models.PendingStepCompletionSignal,
	activity stuckSignalActivitySnapshot,
	now time.Time,
	operation *cancelOperation,
	guard *lockedCancelInFlightGuard,
) error {
	defer guard.release()

	identity, err := s.prepareStuckSignalCancellation(ctx, session, activity, operation)
	if err != nil {
		s.logger.Warn("stuck-signal watchdog: failed to prepare reclaim; skipping this tick",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return err
	}

	// The session's own lifecycle-level prompt wait (promptMu/promptDoneCh/
	// promptFinished in agent/runtime/lifecycle) is never released by the
	// DB-only mutations below. Settle the stuck execution via the existing
	// CancelAgent/escalateStuckCancel primitive first — unlocking the guard
	// around the call exactly like the orchestrator's other cancel callers
	// (see cancelAgentWhileUnlocked) — so a subsequent auto-start re-prompt
	// targets a fresh execution instead of deadlocking behind this one.
	// Any error beyond what cancelAgentWhileUnlocked already tolerates
	// (ErrNoExecutionForSession, ErrCancelEscalated) means the execution
	// could not be confirmed settled; fail closed and skip this tick rather
	// than force-closing the turn anyway.
	if err := s.cancelAgentWhileUnlockedForPrompt(ctx, session.ID, identity, guard.unlock, guard.relock); err != nil {
		s.logger.Warn("stuck-signal watchdog: failed to settle stuck execution before reclaim; skipping this tick",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return err
	}

	// Re-verify under the reacquired lock: the unlock/relock window above
	// gave a live agent a chance to resolve the session on its own (turn-end
	// or failure event), same as the other cancelAgentWhileUnlocked callers
	// re-read fresh state after their own unlock/relock.
	latest, ok := s.stuckSignalStillPending(ctx, session.ID, signal.StepID)
	if !ok {
		return nil
	}

	s.logger.Warn("stuck-signal watchdog: reclaiming session stuck RUNNING with an unapplied completion signal",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("step_id", signal.StepID),
		zap.Duration("signal_age", now.Sub(signal.SignaledAt)))
	if identity.turnID == "" && s.turnService != nil {
		activeTurnID, err := s.peekActiveTurnID(ctx, session.ID)
		if err != nil {
			return fmt.Errorf("verify successor turn after unscoped capture: %w", err)
		}
		if activeTurnID != "" {
			return fmt.Errorf("a successor turn %s appeared after no turn was captured", activeTurnID)
		}
	}

	// Checked completion: close exactly the turn captured above, failing
	// closed if a different turn is now active — e.g. a brand-new legitimate
	// turn that started during the CancelAgent unlock window above. The
	// unchecked completeTurnForSession takes an empty-expectedTurnID
	// "complete every open turn" path that cannot tell the two apart and
	// would force-close the new turn too.
	if err := s.completeTurnForTaskSessionCheckedOwned(ctx, task.ID, session.ID, identity.turnID); err != nil {
		s.logger.Warn("stuck-signal watchdog: captured turn was superseded before it could be settled; skipping reclaim",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.String("step_id", signal.StepID),
			zap.Error(err))
		return err
	}
	updated := s.updateTaskSessionState(ctx, task.ID, session.ID, models.TaskSessionStateWaitingForInput, "", false, latest)
	if updated == nil || updated.State != models.TaskSessionStateWaitingForInput {
		return errors.New("stuck-signal watchdog: failed to move session to WAITING_FOR_INPUT")
	}
	s.reconcileStepCompletionSignalLocked(ctx, task.ID, session.ID, signal.StepID)
	// The watchdog has consumed this operation's workflow completion. A joined
	// explicit cancel still records its user-facing artifact, but must not
	// evaluate the successor step as a second cancellation completion.
	s.setCancellationCompletionEligible(session.ID, operation, false)
	return nil
}

func (s *Service) prepareStuckSignalCancellation(
	ctx context.Context,
	session *models.TaskSession,
	activity stuckSignalActivitySnapshot,
	operation *cancelOperation,
) (cancellationIdentity, error) {
	identity, err := s.captureCancellationIdentity(ctx, session.ID)
	if err != nil {
		return cancellationIdentity{}, fmt.Errorf("capture turn identity: %w", err)
	}
	if activity.executionID == "" {
		identity.executionID = ""
		identity.promptGeneration = 0
		identity.activityEpoch = 0
	} else {
		identity.executionID = activity.executionID
		identity.promptGeneration = activity.generation
		identity.activityEpoch = activity.activityEpoch
	}
	s.setCancellationIdentity(session.ID, operation, identity)
	completionEligible, err := s.cancelTurnCompletionEligible(ctx, session, session.ID)
	if err != nil {
		return cancellationIdentity{}, fmt.Errorf("compute completion eligibility: %w", err)
	}
	s.setCancellationCompletionEligible(session.ID, operation, completionEligible)
	return identity, nil
}

// stuckSignalCandidate applies the unguarded eligibility filter: RUNNING or
// STARTING state, a pending signal older than stuckSignalWatchdogThreshold,
// matching the task's CURRENT step, on a non-Office, non-passthrough task.
// Office sessions go FAILED, not WAITING_FOR_INPUT, and must not advance
// their step from this path — same exclusion PR #2963 applies to
// handleRecoverableFailureLocked's reconciliation hook. Passthrough sessions
// are excluded because this watchdog's inactivity gate cannot see them (see
// the package doc and stuckSignalInactiveLongEnough).
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
	if task.WorkflowStepID != signal.StepID || task.IsFromOffice || session.IsPassthrough {
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

type promptActivityCanceller interface {
	CancelAgentForPrompt(ctx context.Context, sessionID, executionID string, generation, activityEpoch uint64) error
}

type stuckSignalActivitySnapshot struct {
	executionID   string
	generation    uint64
	activityEpoch uint64
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
// Fail CLOSED (skip, do not reclaim) on anything this watchdog cannot read
// as genuine elapsed inactivity — "cannot tell whether the agent is alive"
// must never mean "reclaim". Reclaiming a session that is actually still
// running force-closes a live turn and, for some session kinds, writes the
// next step's prompt straight into a process that is still using its stdin.
// A skipped tick just retries next tick; a wrong reclaim corrupts a live
// session. Four cases:
//   - s.agentManager doesn't implement promptActivitySessionReader: in
//     production this cannot happen (see the compile-time pin in
//     backendapp/adapters.go), but a lightweight test double may still omit
//     it deliberately. No evidence either way — fail closed, skip.
//   - ErrNoExecutionForSession (nothing tracked for the session — process
//     gone, backend restarted): the one case where absence really is
//     information. Fail open, reclaim — there is no live turn left to
//     protect.
//   - Any other error (store failure, transient lookup error, or any future
//     error this reader ever returns): unknown, not "old". Fail closed,
//     skip. This is deliberately the general rule rather than special-casing
//     only the specific errors observed today, so a future error path does
//     not silently inherit fail-open behavior.
//   - An execution is tracked and the read succeeded: a zero lastActivityAt
//     means the tracked execution's activity was never written at all (every
//     writer — armPromptActivity, markAgentActivity, recordSteerActivity,
//     recordActivity — sits on the ACP path only; a passthrough execution
//     never reaches one). That is "unknown", not "over ten minutes ago", so
//     it does not count as inactive even though the zero value trivially
//     satisfies now.Sub(lastActivityAt) >= threshold. Otherwise, reclaim
//     only once now.Sub(lastActivityAt) >= stuckSignalWatchdogThreshold. A
//     session whose signal is old but whose agent is still emitting events
//     within the window is left alone — this is the case a signal-age-only
//     check, or an epoch-equality check across a sub-millisecond scan
//     window, both get wrong.
func (s *Service) stuckSignalInactiveLongEnough(ctx context.Context, sessionID string, now time.Time) (stuckSignalActivitySnapshot, bool) {
	reader, ok := s.agentManager.(promptActivitySessionReader)
	if !ok {
		return stuckSignalActivitySnapshot{}, false
	}
	executionID, generation, activityEpoch, lastActivityAt, err := reader.GetPromptActivityForSession(ctx, sessionID)
	if err != nil {
		return stuckSignalActivitySnapshot{}, executor.IsNoExecutionForSessionError(err)
	}
	if lastActivityAt.IsZero() {
		return stuckSignalActivitySnapshot{}, false
	}
	return stuckSignalActivitySnapshot{
		executionID:   executionID,
		generation:    generation,
		activityEpoch: activityEpoch,
	}, now.Sub(lastActivityAt) >= stuckSignalWatchdogThreshold
}

func (s *Service) cancelAgentWhileUnlockedForPrompt(
	ctx context.Context,
	sessionID string,
	identity cancellationIdentity,
	unlockGuard, relockGuard func(),
) error {
	if identity.executionID == "" {
		return s.cancelAgentWhileUnlocked(ctx, sessionID, unlockGuard, relockGuard)
	}
	canceller, ok := s.agentManager.(promptActivityCanceller)
	if !ok {
		return errors.New("agent manager cannot validate captured prompt activity")
	}
	if identity.promptGeneration == 0 || identity.activityEpoch == 0 {
		return errors.New("captured prompt activity identity is incomplete")
	}
	unlockGuard()
	cancelErr := canceller.CancelAgentForPrompt(
		ctx,
		sessionID,
		identity.executionID,
		identity.promptGeneration,
		identity.activityEpoch,
	)
	relockGuard()
	return s.normalizeCancelAgentError(cancelErr)
}

func (s *Service) normalizeCancelAgentError(cancelErr error) error {
	if cancelErr == nil {
		return nil
	}
	switch {
	case executor.IsNoExecutionForSessionError(cancelErr):
		s.logger.Error("agent process appears to have crashed: no live execution for session on cancel",
			zap.Error(cancelErr))
	case executor.IsCancelEscalatedError(cancelErr):
		s.logger.Warn("agent did not acknowledge cancel; reconciling session state",
			zap.Error(cancelErr))
	default:
		return fmt.Errorf("cancel agent: %w", cancelErr)
	}
	return nil
}
