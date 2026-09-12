package lifecycle

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// ErrSessionRecoveryGuarded is returned when a launch is requested for a
// session whose recovery guard is currently held (AC-EXECUTORS-SURVIVAL-002.8):
// re-tracking for that session may still be in flight, so a concurrent launch
// is refused rather than queued or blocked. Retryable: the guard is released
// either when that session's recovery outcome is published, or (absent the
// AC-EXECUTORS-SURVIVAL-002.16 exception) when the recovery deadline elapses.
var ErrSessionRecoveryGuarded = errors.New("session is guarded pending recovery")

// ErrSessionUnstoppableAgent is returned when a launch is requested for a
// session whose recovery guard was retained under
// AC-EXECUTORS-SURVIVAL-002.16: at least one live instance for that session
// could not be stopped, so nothing in this backend's remaining lifetime will
// resolve the condition. Distinct from ErrSessionRecoveryGuarded: this is NOT
// retryable within the backend's lifetime -- only a backend restart clears
// it, since guards do not survive one and the next start re-evaluates the
// instance.
var ErrSessionUnstoppableAgent = errors.New("session has an unstoppable agent from a prior launch")

// RecoveryGuard is the in-memory, per-session acquire-or-observe map of
// AC-EXECUTORS-SURVIVAL-002.8. It exists for exactly one backend process
// lifetime and needs no durable representation. A guard is normally released
// when that session's recovery outcome is published, or in bulk when the
// recovery deadline (AC-EXECUTORS-SURVIVAL-003.7) elapses -- except a session
// retained as unstoppable (AC-EXECUTORS-SURVIVAL-002.16) or one whose stop is
// still in flight at the deadline, both of which ReleaseAllExceptRetained
// leaves held.
//
// Safe for concurrent use. Does not itself decide which sessions to guard at
// startup (see SessionsToGuard) or when the recovery deadline elapses (that
// clock starts at first control-server contact, AC-EXECUTORS-SURVIVAL-003.7,
// and is owned by the caller driving adoption/re-tracking).
type RecoveryGuard struct {
	mu    sync.Mutex
	state map[string]guardState
}

type guardState int

const (
	guardHeld guardState = iota
	guardStopInFlight
	guardRetainedUnstoppable
)

// NewRecoveryGuard returns an empty guard map.
func NewRecoveryGuard() *RecoveryGuard {
	return &RecoveryGuard{state: make(map[string]guardState)}
}

// AcquireOrObserve takes a guard for sessionID if it is not already guarded.
// Returns true when this call newly acquired the guard (the caller may begin
// re-tracking it); false when the session was already guarded (the caller
// must not begin re-tracking it -- AC-EXECUTORS-SURVIVAL-002.8's "a session
// already guarded ... is observed, not guarded twice").
//
// Callers that also need to check the in-memory execution store for an
// already-tracked session must fold that check into the same critical
// section as their own call to this method to preserve the AC's atomicity
// requirement; RecoveryGuard alone cannot enforce that across packages.
func (g *RecoveryGuard) AcquireOrObserve(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, held := g.state[sessionID]; held {
		return false
	}
	g.state[sessionID] = guardHeld
	return true
}

// Release drops a session's guard, normally called once that session's
// recovery outcome has been published. A no-op for a session that isn't
// guarded, or one retained under RetainAsUnstoppable (only a backend restart
// clears that case, per AC-EXECUTORS-SURVIVAL-002.16).
func (g *RecoveryGuard) Release(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state[sessionID] == guardRetainedUnstoppable {
		return
	}
	delete(g.state, sessionID)
}

// MarkStopInFlight records that a stop for this session's losing/orphaned
// instance is still retrying when the recovery deadline elapses
// (AC-EXECUTORS-SURVIVAL-003.7): ReleaseAllExceptRetained must leave it held
// until the stop resolves, rather than releasing it at the bound. A no-op
// for a session that isn't guarded.
func (g *RecoveryGuard) MarkStopInFlight(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, held := g.state[sessionID]; !held {
		return
	}
	g.state[sessionID] = guardStopInFlight
}

// IsStopInFlight reports whether sessionID's guard is currently in the
// guardStopInFlight state (see MarkStopInFlight): its losing/orphaned
// instance's stop was still retrying when the recovery deadline elapsed and
// has not yet resolved. Consulted by standalone liveness classification
// (design 02 "Persistence": "the snapshot must post-date recovery's own
// stops") so a row whose stop is still in flight when an adopted-server
// enumeration is taken classifies unknown rather than alive, since the
// enumeration is a snapshot that predates the stop's actual completion.
func (g *RecoveryGuard) IsStopInFlight(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state[sessionID] == guardStopInFlight
}

// RetainAsUnstoppable marks sessionID's guard to survive
// ReleaseAllExceptRetained and outlive recovery entirely
// (AC-EXECUTORS-SURVIVAL-002.16): at least one live instance for it could
// not be stopped, so only a backend restart may clear the guard. Idempotent.
func (g *RecoveryGuard) RetainAsUnstoppable(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state[sessionID] = guardRetainedUnstoppable
}

// ReleaseAllExceptRetained releases every guard currently held, except a
// session marked in-flight via MarkStopInFlight (until its stop resolves and
// the caller explicitly calls Release or RetainAsUnstoppable) or retained via
// RetainAsUnstoppable. Called once the recovery deadline
// (AC-EXECUTORS-SURVIVAL-003.7) elapses, so "no guard outlives recovery"
// holds except for those two documented exceptions.
func (g *RecoveryGuard) ReleaseAllExceptRetained() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for sessionID, state := range g.state {
		if state == guardHeld {
			delete(g.state, sessionID)
		}
	}
}

// CheckLaunchAllowed reports whether a launch may proceed for sessionID,
// returning the distinct refusal errors AC-EXECUTORS-SURVIVAL-002.8 (retryable)
// and AC-EXECUTORS-SURVIVAL-002.16 (not retryable) require, or nil when the
// session carries no guard.
func (g *RecoveryGuard) CheckLaunchAllowed(sessionID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	state, held := g.state[sessionID]
	if !held {
		return nil
	}
	if state == guardRetainedUnstoppable {
		return ErrSessionUnstoppableAgent
	}
	return ErrSessionRecoveryGuarded
}

// PassthroughLookup reads whether sessionID is a passthrough session from
// its durable record. ok is false when the read failed (not when the
// session is simply not passthrough) -- AC-EXECUTORS-SURVIVAL-005.3 requires
// treating a failed read as "guard it" rather than "exclude it."
type PassthroughLookup func(ctx context.Context, sessionID string) (isPassthrough bool, ok bool)

// SessionsToGuard applies AC-EXECUTORS-SURVIVAL-005.3's exclusion to the
// live standalone recovery-inventory session identities read at startup step
// 3 (2.1's ListLiveStandaloneExecutorsRunning), returning the subset
// AC-EXECUTORS-SURVIVAL-002.8 requires a guard taken for: every session
// except a confirmed passthrough one. A session whose passthrough mode
// couldn't be read is guarded, not excluded -- failing to guard a
// re-tracking candidate risks two live agents for one session, while a
// wrongly guarded passthrough session only costs one refused launch until
// the deadline elapses.
func SessionsToGuard(ctx context.Context, liveSessionIDs []string, isPassthrough PassthroughLookup) []string {
	guarded := make([]string, 0, len(liveSessionIDs))
	for _, sessionID := range liveSessionIDs {
		passthrough, ok := isPassthrough(ctx, sessionID)
		if ok && passthrough {
			continue
		}
		guarded = append(guarded, sessionID)
	}
	return guarded
}

// TakeStartupRecoveryGuards reads the live standalone recovery-inventory
// records and takes a guard for every session AC-EXECUTORS-SURVIVAL-002.8
// requires one for, exactly as Manager.Start does, but callable before a
// Manager exists at all. Backend startup composition calls this ahead of
// its own control-server adoption attempt (design 02 "Startup" step 3 must
// run before step 4): by the time Start would otherwise take these same
// guards, that adoption contact has already happened. The returned guard is
// meant to be installed via Manager.SetRecoveryGuard before Start runs.
//
// A nil passthroughLookup, or one that fails a given session's read, is
// treated as "guard it" rather than "exclude it" -- the same fail-closed
// default Start applies. A failed inventory read yields an empty guard
// rather than an error, matching ListLiveStandaloneExecutorsRunning's own
// best-effort contract.
func TakeStartupRecoveryGuards(
	ctx context.Context,
	runningWriter ExecutorRunningWriter,
	passthroughLookup PassthroughLookup,
	log *logger.Logger,
) *RecoveryGuard {
	guard := NewRecoveryGuard()
	lister, ok := runningWriter.(executorRunningLister)
	if !ok {
		return guard
	}
	records, err := lister.ListExecutorsRunningLiveStandalone(ctx)
	if err != nil {
		log.Warn("failed to read live standalone recovery-inventory records", zap.Error(err))
		return guard
	}
	if passthroughLookup == nil {
		passthroughLookup = func(context.Context, string) (bool, bool) { return false, false }
	}
	for _, sessionID := range SessionsToGuard(ctx, sessionIDsFromExecutorRunning(records), passthroughLookup) {
		guard.AcquireOrObserve(sessionID)
	}
	return guard
}
