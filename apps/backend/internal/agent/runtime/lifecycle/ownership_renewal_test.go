package lifecycle

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

// countingClaimer records every ClaimOwnership call, optionally failing the
// first N calls to exercise the retry-on-next-interval path, or returning a
// specific error (nextErr, consumed once) to exercise a distinguished
// rejection such as a superseded credential.
type countingClaimer struct {
	mu       sync.Mutex
	calls    int
	failNext int
	nextErr  error
}

func (c *countingClaimer) ClaimOwnership(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.nextErr != nil {
		err := c.nextErr
		c.nextErr = nil
		return err
	}
	if c.failNext > 0 {
		c.failNext--
		return context.DeadlineExceeded
	}
	return nil
}

func (c *countingClaimer) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestOwnershipRenewerStartStopIdempotent(t *testing.T) {
	r := NewOwnershipRenewer(&countingClaimer{}, time.Second, logger.Default())

	r.Start(context.Background())
	t.Cleanup(r.Stop)
	r.Start(context.Background()) // second start is a no-op
	r.Stop()
	r.Stop() // second stop is a no-op
}

func TestOwnershipRenewerStopSafeWithoutStart(t *testing.T) {
	r := NewOwnershipRenewer(&countingClaimer{}, time.Second, logger.Default())
	r.Stop()
}

// TestOwnershipRenewerDoesNotClaimImmediately pins that Start does not issue
// an immediate claim: the caller's rotation/handshake that just happened is
// already the first renewal per AC-EXECUTORS-CONTROL-OWNERSHIP-003.8, so an
// immediate claim here would be a redundant fourth renewing operation.
func TestOwnershipRenewerDoesNotClaimImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())
		t.Cleanup(r.Stop)

		synctest.Wait()
		if got := claimer.callCount(); got != 0 {
			t.Fatalf("callCount immediately after Start = %d, want 0", got)
		}
	})
}

// TestOwnershipRenewerClaimsOnInterval pins that the loop issues one claim
// per elapsed interval, on the interval computed by the caller.
func TestOwnershipRenewerClaimsOnInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())
		t.Cleanup(r.Stop)

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after one interval = %d, want 1", got)
		}

		time.Sleep(time.Minute)
		synctest.Wait()
		if got := claimer.callCount(); got != 2 {
			t.Fatalf("callCount after two intervals = %d, want 2", got)
		}
	})
}

// TestOwnershipRenewerRetriesAfterFailure pins that a failed claim does not
// stop the loop: the next interval issues another attempt.
func TestOwnershipRenewerRetriesAfterFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{failNext: 1}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())
		t.Cleanup(r.Stop)

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after failed interval = %d, want 1", got)
		}

		time.Sleep(time.Minute)
		synctest.Wait()
		if got := claimer.callCount(); got != 2 {
			t.Fatalf("callCount after retry interval = %d, want 2", got)
		}
	})
}

// TestOwnershipRenewerStopsWithoutRetryOnSupersededCredential pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.3: a superseded-credential rejection
// (distinct from a transient failure) must not be retried on the next
// interval -- unlike TestOwnershipRenewerRetriesAfterFailure, the loop ends
// instead of ticking again.
func TestOwnershipRenewerStopsWithoutRetryOnSupersededCredential(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{nextErr: agentctl.ErrOwnershipCredentialSuperseded}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())
		t.Cleanup(r.Stop)

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after superseded rejection = %d, want 1", got)
		}

		time.Sleep(3 * time.Minute)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after further intervals = %d, want 1 (no retry)", got)
		}
	})
}

// TestOwnershipRenewerSupersededCredentialAllowsRestart pins that the loop
// self-stopping on a superseded credential leaves the renewer in a state a
// later Start (after the manager re-adopts and obtains a current
// credential) can reuse -- AC-002.3's "shall not acquire a fresh credential
// without adopting again" is the caller's job, but this renewer must not be
// left stuck "started" forever, which would make a legitimate restart a
// silent no-op.
func TestOwnershipRenewerSupersededCredentialAllowsRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{nextErr: agentctl.ErrOwnershipCredentialSuperseded}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after superseded rejection = %d, want 1", got)
		}

		// Simulate re-adoption obtaining a fresh credential and restarting
		// the renewer against it.
		r.Start(context.Background())
		t.Cleanup(r.Stop)

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if got := claimer.callCount(); got != 2 {
			t.Fatalf("callCount after restart interval = %d, want 2 (restart took effect)", got)
		}
	})
}

// TestOwnershipRenewerExplicitStopAfterSupersededCredentialIsSafe pins that
// an explicit Stop() call racing with (or arriving after) the loop's own
// self-stop neither panics nor hangs.
func TestOwnershipRenewerExplicitStopAfterSupersededCredentialIsSafe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{nextErr: agentctl.ErrOwnershipCredentialSuperseded}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()

		r.Stop()
		r.Stop() // still safe a second time
	})
}

// TestOwnershipRenewerStopEndsLoop pins that Stop halts further claims.
func TestOwnershipRenewerStopEndsLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		claimer := &countingClaimer{}
		r := NewOwnershipRenewer(claimer, time.Minute, logger.Default())
		r.Start(context.Background())

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		r.Stop()

		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if got := claimer.callCount(); got != 1 {
			t.Fatalf("callCount after Stop = %d, want 1 (no further claims)", got)
		}
	})
}

func TestOwnershipRenewerNoOpWithoutClaimerOrInterval(t *testing.T) {
	r := NewOwnershipRenewer(nil, time.Minute, logger.Default())
	r.Start(context.Background())
	r.Stop()

	r2 := NewOwnershipRenewer(&countingClaimer{}, 0, logger.Default())
	r2.Start(context.Background())
	r2.Stop()
}
