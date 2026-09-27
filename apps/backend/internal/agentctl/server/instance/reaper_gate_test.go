package instance

import (
	"testing"
	"time"
)

// TestInstanceManagerReaperGateAggregatesLiveInstances pins
// AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5/AC3: the unowned reaper's hold
// decision reads every live instance's own gate state fresh on each call, so
// a newly created instance is covered and a removed one no longer holds the
// gate. latestEnforcementEnd is the maximum enforcement-end time observed
// across every instance, used to push the unowned reaper's floor forward.
func TestInstanceManagerReaperGateAggregatesLiveInstances(t *testing.T) {
	mgr := &Manager{instances: map[string]*Instance{}}

	if hold, end := mgr.ReaperGate(); hold || !end.IsZero() {
		t.Fatalf("ReaperGate() on an empty manager = (%v, %v), want (false, zero)", hold, end)
	}

	holding := &Instance{ID: "holding", manager: &fakeProcessManager{reaperHold: true}}
	mgr.instances[holding.ID] = holding

	if hold, _ := mgr.ReaperGate(); !hold {
		t.Fatal("ReaperGate() hold = false with one held instance live, want true")
	}

	earlier := time.Now().Add(-time.Minute)
	later := time.Now()
	quiet := &Instance{ID: "quiet", manager: &fakeProcessManager{reaperEnforcementEnd: earlier}}
	loudest := &Instance{ID: "loudest", manager: &fakeProcessManager{reaperEnforcementEnd: later}}
	mgr.instances[quiet.ID] = quiet
	mgr.instances[loudest.ID] = loudest

	if _, end := mgr.ReaperGate(); !end.Equal(later) {
		t.Fatalf("ReaperGate() latestEnforcementEnd = %v, want the max across instances (%v)", end, later)
	}

	// Removing the only held instance releases the gate: a newly created
	// instance is covered, and a removed one no longer holds it.
	delete(mgr.instances, holding.ID)

	if hold, _ := mgr.ReaperGate(); hold {
		t.Fatal("ReaperGate() hold = true after the held instance was removed, want false")
	}
}

// TestInstanceManagerReaperGateSkipsInstanceWithoutManager pins the
// nil-manager guard already used by Info(): a defensive test-only Instance
// with no process manager must not panic ReaperGate() and contributes
// nothing to either return value.
func TestInstanceManagerReaperGateSkipsInstanceWithoutManager(t *testing.T) {
	mgr := &Manager{instances: map[string]*Instance{
		"no-manager": {ID: "no-manager"},
	}}

	hold, end := mgr.ReaperGate()
	if hold || !end.IsZero() {
		t.Fatalf("ReaperGate() with a managerless instance = (%v, %v), want (false, zero)", hold, end)
	}
}
