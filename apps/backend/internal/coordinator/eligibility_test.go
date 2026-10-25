package coordinator

import (
	"testing"
	"time"
)

// TestEligibleStep_UnknownID_ReturnsFalse covers the unknown-id case named in
// Build decision 2.
func TestEligibleStep_UnknownID_ReturnsFalse(t *testing.T) {
	steps := []StepNode{{ID: "start", IsStart: true}}
	if EligibleStep(steps, "missing") {
		t.Error("EligibleStep(missing) = true, want false")
	}
}

// TestEligibleStep_StartStep_Eligible covers the plain start-step case: no
// auto-start, no feeder relationship, is the start step.
func TestEligibleStep_StartStep_Eligible(t *testing.T) {
	steps := []StepNode{{ID: "start", IsStart: true}}
	if !EligibleStep(steps, "start") {
		t.Error("EligibleStep(start) = false, want true")
	}
}

// TestEligibleStep_ManualMoveAllowed_Eligible covers a non-start step that
// allows manual moves.
func TestEligibleStep_ManualMoveAllowed_Eligible(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true},
		{ID: "middle", AllowManualMove: true},
	}
	if !EligibleStep(steps, "middle") {
		t.Error("EligibleStep(middle) = false, want true")
	}
}

// TestEligibleStep_NeitherStartNorManual_Ineligible covers a step that is
// neither the start step nor manual-move-enabled.
func TestEligibleStep_NeitherStartNorManual_Ineligible(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true},
		{ID: "locked"},
	}
	if EligibleStep(steps, "locked") {
		t.Error("EligibleStep(locked) = true, want false")
	}
}

// TestEligibleStep_AutoStartStep_Ineligible covers a step that itself
// auto-starts an agent on enter.
func TestEligibleStep_AutoStartStep_Ineligible(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true, AutoStartOnEnter: true},
	}
	if EligibleStep(steps, "start") {
		t.Error("EligibleStep(start) = true, want false")
	}
}

// TestEligibleStep_DirectFeeder_Ineligible covers a step that is a direct
// pull_from_step_id feeder of an auto-start step.
func TestEligibleStep_DirectFeeder_Ineligible(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true},
		{ID: "auto", AllowManualMove: true, AutoStartOnEnter: true, PullFromStepID: "start"},
	}
	if EligibleStep(steps, "start") {
		t.Error("EligibleStep(start) = true, want false (direct feeder of an auto-start step)")
	}
}

// TestEligibleStep_TransitiveFeeder_Ineligible covers a multi-hop feeder
// chain: start -> mid -> auto(auto-start), so start feeds an auto-start step
// transitively through mid.
func TestEligibleStep_TransitiveFeeder_Ineligible(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true},
		{ID: "mid", AllowManualMove: true, PullFromStepID: "start"},
		{ID: "auto", AllowManualMove: true, AutoStartOnEnter: true, PullFromStepID: "mid"},
	}
	if EligibleStep(steps, "start") {
		t.Error("EligibleStep(start) = true, want false (transitive feeder of an auto-start step)")
	}
	if EligibleStep(steps, "mid") {
		t.Error("EligibleStep(mid) = true, want false (direct feeder of an auto-start step)")
	}
}

// TestEligibleStep_FeederCycle_Terminates proves the feeder walk uses a
// visited set so a cycle in pull_from_step_id links terminates instead of
// looping forever, per Build decision 2.
func TestEligibleStep_FeederCycle_Terminates(t *testing.T) {
	steps := []StepNode{
		{ID: "a", IsStart: true, AllowManualMove: true, PullFromStepID: "b"},
		{ID: "b", AllowManualMove: true, PullFromStepID: "a"},
	}
	done := make(chan bool, 1)
	go func() { done <- EligibleStep(steps, "a") }()
	select {
	case got := <-done:
		if !got {
			t.Error("EligibleStep(a) = false, want true (cycle with no auto-start step is eligible)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EligibleStep did not terminate on a cyclic feeder graph")
	}
}

// TestEligibleStep_UnrelatedAutoStartStep_DoesNotAffectOthers proves an
// auto-start step elsewhere in the graph does not make an unrelated step
// ineligible.
func TestEligibleStep_UnrelatedAutoStartStep_DoesNotAffectOthers(t *testing.T) {
	steps := []StepNode{
		{ID: "start", IsStart: true},
		{ID: "other", AllowManualMove: true},
		{ID: "auto", AutoStartOnEnter: true, PullFromStepID: "other"},
	}
	if !EligibleStep(steps, "start") {
		t.Error("EligibleStep(start) = false, want true (unrelated auto-start step)")
	}
}
