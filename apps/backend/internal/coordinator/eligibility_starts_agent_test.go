package coordinator

import "testing"

func TestStartsAgentOnEnter(t *testing.T) {
	steps := []StepNode{
		{ID: "plain", AllowManualMove: true},
		{ID: "auto", AutoStartOnEnter: true},
		{ID: "feeder", AllowManualMove: true},
		{ID: "chain", AllowManualMove: true, PullFromStepID: "feeder"},
		{ID: "sink", AutoStartOnEnter: true, PullFromStepID: "chain"},
	}
	cases := map[string]bool{"plain": false, "auto": true, "feeder": true, "chain": true, "sink": true, "missing": false}
	for id, want := range cases {
		if got := StartsAgentOnEnter(steps, id); got != want {
			t.Errorf("StartsAgentOnEnter(%q) = %v, want %v", id, got, want)
		}
	}
}
