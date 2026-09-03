package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRecoveryGuardAcquireOrObserveIsAtomicPerSession(t *testing.T) {
	g := NewRecoveryGuard()

	if acquired := g.AcquireOrObserve("session-1"); !acquired {
		t.Fatal("first AcquireOrObserve = false, want true (newly acquired)")
	}
	if acquired := g.AcquireOrObserve("session-1"); acquired {
		t.Fatal("second AcquireOrObserve on the same session = true, want false (observed, not guarded twice)")
	}
	if acquired := g.AcquireOrObserve("session-2"); !acquired {
		t.Fatal("AcquireOrObserve on a different session = false, want true")
	}
}

func TestRecoveryGuardReleaseAllowsReacquisition(t *testing.T) {
	g := NewRecoveryGuard()
	g.AcquireOrObserve("session-1")
	g.Release("session-1")

	if acquired := g.AcquireOrObserve("session-1"); !acquired {
		t.Fatal("AcquireOrObserve after Release = false, want true")
	}
}

func TestRecoveryGuardReleaseOfUnguardedSessionIsSafe(t *testing.T) {
	g := NewRecoveryGuard()
	g.Release("never-guarded") // must not panic
}

func TestRecoveryGuardCheckLaunchAllowedForUnguardedSession(t *testing.T) {
	g := NewRecoveryGuard()
	if err := g.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("CheckLaunchAllowed for an unguarded session = %v, want nil", err)
	}
}

// TestRecoveryGuardCheckLaunchAllowedRefusesWithRetryableReason pins
// AC-EXECUTORS-SURVIVAL-002.8: a guarded session's launch is refused with
// the distinct retryable recovery reason, not queued or blocked.
func TestRecoveryGuardCheckLaunchAllowedRefusesWithRetryableReason(t *testing.T) {
	g := NewRecoveryGuard()
	g.AcquireOrObserve("session-1")

	err := g.CheckLaunchAllowed("session-1")
	if !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("CheckLaunchAllowed for a guarded session = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestRecoveryGuardRetainAsUnstoppableRefusesNonRetryably pins
// AC-EXECUTORS-SURVIVAL-002.16: a session with an unstoppable instance is
// refused with a distinct, non-retryable reason, and stays that way even
// across a Release call (only a backend restart clears it).
func TestRecoveryGuardRetainAsUnstoppableRefusesNonRetryably(t *testing.T) {
	g := NewRecoveryGuard()
	g.AcquireOrObserve("session-1")
	g.RetainAsUnstoppable("session-1")

	err := g.CheckLaunchAllowed("session-1")
	if !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("CheckLaunchAllowed for a retained session = %v, want ErrSessionUnstoppableAgent", err)
	}

	g.Release("session-1")
	err = g.CheckLaunchAllowed("session-1")
	if !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("CheckLaunchAllowed after Release on a retained session = %v, want ErrSessionUnstoppableAgent (Release must not clear retention)", err)
	}
}

// TestRecoveryGuardReleaseAllExceptRetainedHonoursBothExceptions pins
// AC-EXECUTORS-SURVIVAL-003.7: the deadline-triggered mass release must
// leave a retained-unstoppable session AND a stop-in-flight session held,
// releasing only an ordinary guarded session.
func TestRecoveryGuardReleaseAllExceptRetainedHonoursBothExceptions(t *testing.T) {
	g := NewRecoveryGuard()
	g.AcquireOrObserve("ordinary")
	g.AcquireOrObserve("retained")
	g.RetainAsUnstoppable("retained")
	g.AcquireOrObserve("in-flight")
	g.MarkStopInFlight("in-flight")

	g.ReleaseAllExceptRetained()

	if err := g.CheckLaunchAllowed("ordinary"); err != nil {
		t.Fatalf("ordinary session after deadline release = %v, want nil (released)", err)
	}
	if err := g.CheckLaunchAllowed("retained"); !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("retained session after deadline release = %v, want ErrSessionUnstoppableAgent (must stay held)", err)
	}
	if err := g.CheckLaunchAllowed("in-flight"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("in-flight session after deadline release = %v, want ErrSessionRecoveryGuarded (must stay held)", err)
	}
}

// TestRecoveryGuardStopInFlightThenReleaseClearsIt pins that once an
// in-flight stop resolves successfully, the caller's own Release call (not
// ReleaseAllExceptRetained, which already ran) is what actually frees it.
func TestRecoveryGuardStopInFlightThenReleaseClearsIt(t *testing.T) {
	g := NewRecoveryGuard()
	g.AcquireOrObserve("session-1")
	g.MarkStopInFlight("session-1")
	g.ReleaseAllExceptRetained() // must not release it

	g.Release("session-1") // the stop resolved; caller releases explicitly

	if err := g.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("CheckLaunchAllowed after explicit Release = %v, want nil", err)
	}
}

func TestRecoveryGuardMarkStopInFlightOnUnguardedSessionIsSafe(t *testing.T) {
	g := NewRecoveryGuard()
	g.MarkStopInFlight("never-guarded") // must not panic or guard it
	if err := g.CheckLaunchAllowed("never-guarded"); err != nil {
		t.Fatalf("CheckLaunchAllowed for a never-guarded session = %v, want nil", err)
	}
}

func TestRecoveryGuardIsStopInFlight(t *testing.T) {
	g := NewRecoveryGuard()

	if g.IsStopInFlight("session-1") {
		t.Fatal("expected an unguarded session to not report stop-in-flight")
	}

	g.AcquireOrObserve("session-1")
	if g.IsStopInFlight("session-1") {
		t.Fatal("expected a plainly held guard to not report stop-in-flight")
	}

	g.MarkStopInFlight("session-1")
	if !g.IsStopInFlight("session-1") {
		t.Fatal("expected session-1 to report stop-in-flight after MarkStopInFlight")
	}

	g.Release("session-1")
	if g.IsStopInFlight("session-1") {
		t.Fatal("expected stop-in-flight to clear once the guard is released")
	}
}

func TestRecoveryGuardConcurrentAcquireOrObserveIsRace_Free(t *testing.T) {
	g := NewRecoveryGuard()
	var wg sync.WaitGroup
	var acquiredCount int
	var mu sync.Mutex

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.AcquireOrObserve("shared-session") {
				mu.Lock()
				acquiredCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if acquiredCount != 1 {
		t.Fatalf("acquiredCount = %d, want exactly 1 across 50 concurrent callers", acquiredCount)
	}
}

func TestSessionsToGuardExcludesConfirmedPassthrough(t *testing.T) {
	lookup := func(_ context.Context, sessionID string) (bool, bool) {
		return sessionID == "passthrough-session", true
	}

	got := SessionsToGuard(context.Background(), []string{"normal-session", "passthrough-session"}, lookup)

	if len(got) != 1 || got[0] != "normal-session" {
		t.Fatalf("SessionsToGuard = %v, want [normal-session]", got)
	}
}

// TestSessionsToGuardIncludesOnLookupFailure pins AC-EXECUTORS-SURVIVAL-005.3:
// when passthrough mode can't be read for a record, the session is guarded
// rather than excluded.
func TestSessionsToGuardIncludesOnLookupFailure(t *testing.T) {
	lookup := func(_ context.Context, _ string) (bool, bool) {
		return false, false // read failed
	}

	got := SessionsToGuard(context.Background(), []string{"unreadable-session"}, lookup)

	if len(got) != 1 || got[0] != "unreadable-session" {
		t.Fatalf("SessionsToGuard on lookup failure = %v, want [unreadable-session] (guarded, not excluded)", got)
	}
}

func TestSessionsToGuardIncludesOrdinarySessions(t *testing.T) {
	lookup := func(_ context.Context, _ string) (bool, bool) {
		return false, true
	}

	got := SessionsToGuard(context.Background(), []string{"session-1", "session-2"}, lookup)

	if len(got) != 2 {
		t.Fatalf("SessionsToGuard = %v, want both sessions included", got)
	}
}

func TestSessionsToGuardEmptyInputReturnsEmpty(t *testing.T) {
	got := SessionsToGuard(context.Background(), nil, func(context.Context, string) (bool, bool) { return false, true })
	if len(got) != 0 {
		t.Fatalf("SessionsToGuard(nil) = %v, want empty", got)
	}
}
