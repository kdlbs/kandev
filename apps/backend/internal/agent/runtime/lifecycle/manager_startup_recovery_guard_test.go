package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func alwaysNotPassthrough(context.Context, string) (bool, bool) { return false, true }

// TestTakeStartupRecoveryGuardsGuardsLiveNonPassthroughSessions pins Review
// round 3, finding 1 / AC-EXECUTORS-SURVIVAL-002.8: the returned guard must
// already hold every live standalone recovery-inventory session before
// backend startup composition contacts any control server (adoption
// included), not just by the time Manager.Start gets around to it.
func TestTakeStartupRecoveryGuardsGuardsLiveNonPassthroughSessions(t *testing.T) {
	writer := &listingWriter{rows: []*models.ExecutorRunning{
		{SessionID: "session-1"}, {SessionID: "session-2"},
	}}

	guard := TakeStartupRecoveryGuards(context.Background(), writer, alwaysNotPassthrough, newTestLogger())

	if err := guard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("session-1 CheckLaunchAllowed = %v, want ErrSessionRecoveryGuarded", err)
	}
	if err := guard.CheckLaunchAllowed("session-2"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("session-2 CheckLaunchAllowed = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestTakeStartupRecoveryGuardsExcludesConfirmedPassthroughSessions pins
// AC-EXECUTORS-SURVIVAL-005.3's exclusion still applying at this earlier
// guard-taking point.
func TestTakeStartupRecoveryGuardsExcludesConfirmedPassthroughSessions(t *testing.T) {
	writer := &listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-passthrough"}}}
	isPassthrough := func(context.Context, string) (bool, bool) { return true, true }

	guard := TakeStartupRecoveryGuards(context.Background(), writer, isPassthrough, newTestLogger())

	if err := guard.CheckLaunchAllowed("session-passthrough"); err != nil {
		t.Fatalf("CheckLaunchAllowed = %v, want nil (excluded)", err)
	}
}

// TestTakeStartupRecoveryGuardsGuardsWhenPassthroughLookupFails pins the
// fail-closed default: a lookup that could not read the durable record
// guards the session rather than excluding it.
func TestTakeStartupRecoveryGuardsGuardsWhenPassthroughLookupFails(t *testing.T) {
	writer := &listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-unknown"}}}
	failedLookup := func(context.Context, string) (bool, bool) { return false, false }

	guard := TakeStartupRecoveryGuards(context.Background(), writer, failedLookup, newTestLogger())

	if err := guard.CheckLaunchAllowed("session-unknown"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestTakeStartupRecoveryGuardsReturnsEmptyGuardOnListError pins the
// best-effort fallback already established for
// ListLiveStandaloneExecutorsRunning: a failed inventory read yields an
// empty (but usable) guard rather than a nil one or a panic.
func TestTakeStartupRecoveryGuardsReturnsEmptyGuardOnListError(t *testing.T) {
	writer := &listingWriter{err: errors.New("db unavailable")}

	guard := TakeStartupRecoveryGuards(context.Background(), writer, alwaysNotPassthrough, newTestLogger())

	if guard == nil {
		t.Fatal("guard = nil, want a usable empty guard")
	}
	if err := guard.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("CheckLaunchAllowed = %v, want nil (nothing to guard)", err)
	}
}

// TestTakeStartupRecoveryGuardsAcceptsNilPassthroughLookup pins the same
// nil-lookup fallback Start applies: no lookup configured guards every
// session, same as a failed read.
func TestTakeStartupRecoveryGuardsAcceptsNilPassthroughLookup(t *testing.T) {
	writer := &listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}}

	guard := TakeStartupRecoveryGuards(context.Background(), writer, nil, newTestLogger())

	if err := guard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestSetRecoveryGuardReplacesDefaultGuard pins that a guard populated
// before Start (by TakeStartupRecoveryGuards) is the one Start's own
// (idempotent) guard-taking and the launch-refusal path both observe --
// not a fresh, still-empty guard the Manager created for itself.
func TestSetRecoveryGuardReplacesDefaultGuard(t *testing.T) {
	mgr := newTestManager(t)
	pre := NewRecoveryGuard()
	pre.AcquireOrObserve("session-pretaken")

	mgr.SetRecoveryGuard(pre)

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-pretaken"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestSetRecoveryGuardIgnoresNil pins that a nil guard leaves the manager's
// existing guard (its own default, or one installed earlier) in place
// instead of leaving the manager without a guard at all.
func TestSetRecoveryGuardIgnoresNil(t *testing.T) {
	mgr := newTestManager(t)
	existing := mgr.recoveryGuard

	mgr.SetRecoveryGuard(nil)

	if mgr.recoveryGuard != existing {
		t.Fatal("SetRecoveryGuard(nil) replaced the manager's existing guard")
	}
}
