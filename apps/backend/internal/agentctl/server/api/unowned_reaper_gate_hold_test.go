package api

import (
	"sync"
	"testing"
	"time"
)

// reaperGateStub lets a test script exactly what live instance state the
// unowned reaper observes on each tick, without spawning a real instance.
type reaperGateStub struct {
	mu                   sync.Mutex
	hold                 bool
	latestEnforcementEnd time.Time
}

func (s *reaperGateStub) ReaperGate() (bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hold, s.latestEnforcementEnd
}

func (s *reaperGateStub) setHold(hold bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold = hold
}

// TestReaperGateHoldsDuringOfflineBudget pins
// AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5 / task-01 acceptance #3: the
// unowned reaper does not shut agentctl down while any live instance is
// detached with an unexpired offline budget (or an expired one whose
// enforcement has not ended), evaluated fresh every tick, and it can fire
// again once that instance no longer holds the gate (for example, because it
// was removed).
func TestReaperGateHoldsDuringOfflineBudget(t *testing.T) {
	cs := newUnownedReaperTestServer(t)
	backdateOwnership(cs, time.Hour)

	stub := &reaperGateStub{hold: true}
	cs.reaperGate = stub

	cs.reaperWG.Add(1)
	go cs.runUnownedReaper(20*time.Millisecond, 5*time.Millisecond)

	// The unowned period has long since elapsed, but the gate holds: the
	// reaper must not latch shutdown while it does.
	holdDeadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(holdDeadline) {
		if cs.ownership.IsShuttingDown() {
			t.Fatal("ownership.IsShuttingDown() = true while the reaper gate holds, want false")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Once nothing holds the gate (the held instance was removed), the
	// reaper resumes its normal decision on the very next tick.
	stub.setHold(false)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cs.ownership.IsShuttingDown() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cs.ownership.IsShuttingDown() {
		t.Fatal("ownership.IsShuttingDown() = false after the reaper gate released, want true")
	}
}

// TestUnownedPeriodFloorAdvancesPastEnforcementEnd pins the system design's
// "the unowned period also counts from the later of the last renewal and the
// latest enforcement end": even though ownership was last renewed long ago,
// a recent enforcement end keeps the reaper from firing until a full period
// has elapsed after that enforcement, not after the stale renewal.
func TestUnownedPeriodFloorAdvancesPastEnforcementEnd(t *testing.T) {
	cs := newUnownedReaperTestServer(t)
	backdateOwnership(cs, time.Hour)

	stub := &reaperGateStub{}
	stub.latestEnforcementEnd = time.Now()
	cs.reaperGate = stub

	period := 150 * time.Millisecond
	cs.reaperWG.Add(1)
	go cs.runUnownedReaper(period, 5*time.Millisecond)

	quietDeadline := time.Now().Add(period / 2)
	for time.Now().Before(quietDeadline) {
		if cs.ownership.IsShuttingDown() {
			t.Fatal("ownership.IsShuttingDown() = true before a full period elapsed since enforcement end, want false")
		}
		time.Sleep(5 * time.Millisecond)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cs.ownership.IsShuttingDown() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cs.ownership.IsShuttingDown() {
		t.Fatal("ownership.IsShuttingDown() = false after a full period elapsed since enforcement end, want true")
	}
}
