package api

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// resolveUnownedPeriod's own coverage (AC-EXECUTORS-CONTROL-OWNERSHIP-003.4/
// .6/.7) moved to internal/common/ownershipperiod, which both this package
// and the backend's ownership-renewal loop now import, so the computation
// cannot drift between the two sides that must agree on it.

// --- reaper goroutine lifecycle ---

func newUnownedReaperTestServer(t *testing.T) *ControlServer {
	t.Helper()
	cfg := &config.Config{AuthToken: "test-token"}
	cs := NewControlServer(cfg, &instance.Manager{}, logger.Default())
	t.Cleanup(cs.StopUnownedReaper)
	return cs
}

// TestUnownedReaperFiresShutdownAfterPeriodElapses pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-003.1/.9: once no ownership claim has been
// current for the resolved period, the reaper latches the one-way shutdown
// door and signals the run loop through the SAME mechanism the
// ownership-shutdown operation uses, not a second path.
func TestUnownedReaperFiresShutdownAfterPeriodElapses(t *testing.T) {
	cs := newUnownedReaperTestServer(t)
	backdateOwnership(cs, time.Hour)

	cs.reaperWG.Add(1)
	go cs.runUnownedReaper(20*time.Millisecond, 5*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cs.ownership.IsShuttingDown() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !cs.ownership.IsShuttingDown() {
		t.Fatal("ownership.IsShuttingDown() = false after the unowned period elapsed, want true")
	}

	select {
	case <-cs.ShutdownRequested():
	default:
		t.Fatal("ShutdownRequested() channel not closed after the reaper fired")
	}
}

// TestUnownedReaperDoesNotFireWhileOwnershipIsRenewed pins that a
// continuously-renewed ownership claim keeps the reaper quiet: it must
// measure elapsed time since the last renewal, not since the reaper itself
// started.
func TestUnownedReaperDoesNotFireWhileOwnershipIsRenewed(t *testing.T) {
	cs := newUnownedReaperTestServer(t)

	cs.reaperWG.Add(1)
	go cs.runUnownedReaper(200*time.Millisecond, 10*time.Millisecond)

	renewDeadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(renewDeadline) {
		cs.ownership.Renew()
		time.Sleep(5 * time.Millisecond)
	}

	if cs.ownership.IsShuttingDown() {
		t.Fatal("ownership.IsShuttingDown() = true despite continuous renewal, want false")
	}
	select {
	case <-cs.ShutdownRequested():
		t.Fatal("ShutdownRequested() channel closed despite continuous renewal")
	default:
	}
}

// TestStopUnownedReaperSafeWithoutStart pins that Stop is a safe no-op when
// the reaper was never started, mirroring the goroutine-ownership
// convention's idempotent-on-both-ends requirement.
func TestStopUnownedReaperSafeWithoutStart(t *testing.T) {
	cs := newUnownedReaperTestServer(t)

	done := make(chan struct{})
	go func() {
		cs.StopUnownedReaper()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopUnownedReaper() blocked when the reaper was never started")
	}
}

// TestStopUnownedReaperIsIdempotent pins that calling Stop twice after a real
// Start does not panic or deadlock.
func TestStopUnownedReaperIsIdempotent(t *testing.T) {
	cs := newUnownedReaperTestServer(t)
	cs.reaperWG.Add(1)
	go cs.runUnownedReaper(time.Hour, 5*time.Millisecond)

	cs.StopUnownedReaper()
	cs.StopUnownedReaper()
}
