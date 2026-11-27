package replay

import (
	"reflect"
	"testing"
)

func guardCase(id string, cand, base []Attempt, reproduced ...expected) *caseState {
	return &caseState{id: id, exp: expectation{reproduced: reproduced}, candidate: cand, baseline: base}
}

func TestGuardFlipsAtOneTwoThreeOfThree(t *testing.T) {
	r := expected{key: "k", proposalID: "p1"}
	base := []Attempt{ok(1, "k"), ok(2, "k"), ok(3, "k")}
	cases := []struct {
		name string
		cand []Attempt
		flip bool
	}{
		{"3 of 3", []Attempt{ok(1, "k"), ok(2, "k"), ok(3, "k")}, false},
		{"2 of 3", []Attempt{ok(1, "k"), ok(2, "k"), ok(3)}, false},
		{"1 of 3", []Attempt{ok(1, "k"), ok(2), ok(3)}, true},
		{"0 of 3", []Attempt{ok(1), ok(2), ok(3)}, true},
		{"hit miss failed", []Attempt{ok(1, "k"), ok(2), bad(3)}, true},
		{"hit miss hit", []Attempt{ok(1, "k"), ok(2), ok(3, "k")}, false},
		{"two attempts both hit", []Attempt{ok(1, "k"), ok(2, "k"), bad(3)}, false},
		{"two attempts one hit", []Attempt{ok(1, "k"), bad(2), ok(3)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flips := guardFlips([]*caseState{guardCase("t1", c.cand, base, r)})
			if (len(flips) == 1) != c.flip {
				t.Fatalf("flips = %v, want flip=%v", flips, c.flip)
			}
			if c.flip && !reflect.DeepEqual(flips, []Flip{{TurnID: "t1", ProposalID: "p1"}}) {
				t.Fatalf("flip = %v", flips)
			}
		})
	}
}

func TestGuardOnlyBlocksWhatBaselineReproduces(t *testing.T) {
	r := expected{key: "k", proposalID: "p1"}
	base := []Attempt{ok(1), ok(2), ok(3)}
	cand := []Attempt{ok(1), ok(2), ok(3)}
	if flips := guardFlips([]*caseState{guardCase("t", cand, base, r)}); len(flips) != 0 {
		t.Fatalf("baseline does not reproduce, got flips %v", flips)
	}
}

func TestGuardFlipCarriesSmallestProposalID(t *testing.T) {
	r := expected{key: "k", proposalID: "p1"}
	flips := guardFlips([]*caseState{guardCase("t", []Attempt{ok(1), ok(2), ok(3)},
		[]Attempt{ok(1, "k"), ok(2, "k"), ok(3, "k")}, r)})
	if len(flips) != 1 || flips[0].ProposalID != "p1" {
		t.Fatalf("flips = %v", flips)
	}
}

func TestGuardIgnoresCasesThatDidNotRunOnBothSides(t *testing.T) {
	r := expected{key: "k", proposalID: "p1"}
	c := guardCase("t", []Attempt{ok(1), bad(2), bad(3)}, []Attempt{ok(1, "k"), ok(2, "k"), ok(3, "k")}, r)
	if flips := guardFlips(comparedOnly([]*caseState{c})); len(flips) != 0 {
		t.Fatalf("a candidate-excluded case must leave the guard, got %v", flips)
	}
}
