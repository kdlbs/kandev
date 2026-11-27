package coordinator

import "testing"

func TestUnknownStepCannotStartAgentThroughDanglingFeeder(t *testing.T) {
	steps := []StepNode{{ID: "auto", AutoStartOnEnter: true, PullFromStepID: "missing"}, {ID: "manual", AllowManualMove: true}}
	if StartsAgentOnEnter(steps, "missing") {
		t.Fatal("unknown step classified as agent-starting through a dangling feeder")
	}
	if !StartsAgentOnEnter(steps, "auto") {
		t.Fatal("known auto-start step must still start an agent")
	}
	steps = append(steps, StepNode{ID: "missing"})
	if !StartsAgentOnEnter(steps, "missing") {
		t.Fatal("known feeder step must still start the downstream agent")
	}
}
