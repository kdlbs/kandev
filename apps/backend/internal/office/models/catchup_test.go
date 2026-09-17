package models_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestNormaliseCatchUpPolicy(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want models.RoutineCatchUpPolicy
	}{
		{"canonical", "summarize_missed", models.CatchUpPolicySummarizeMissed},
		{"deprecated alias", "enqueue_missed_with_cap", models.CatchUpPolicySummarizeMissed},
		{"skip_missed unchanged", "skip_missed", models.CatchUpPolicySkipMissed},
		{"empty defaults to summarize", "", models.CatchUpPolicySummarizeMissed},
		{"unknown defaults to summarize", "garbage", models.CatchUpPolicySummarizeMissed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := models.NormaliseCatchUpPolicy(tc.in); got != tc.want {
				t.Errorf("NormaliseCatchUpPolicy(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRoutineCatchUpPolicy_Scan(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want models.RoutineCatchUpPolicy
	}{
		{"string alias", "enqueue_missed_with_cap", models.CatchUpPolicySummarizeMissed},
		{"bytes alias", []byte("enqueue_missed_with_cap"), models.CatchUpPolicySummarizeMissed},
		{"string canonical", "summarize_missed", models.CatchUpPolicySummarizeMissed},
		{"string skip", "skip_missed", models.CatchUpPolicySkipMissed},
		{"nil", nil, models.CatchUpPolicySummarizeMissed},
		{"unknown string", "totally-unknown", models.CatchUpPolicySummarizeMissed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p models.RoutineCatchUpPolicy
			if err := p.Scan(tc.in); err != nil {
				t.Fatalf("Scan(%v): %v", tc.in, err)
			}
			if p != tc.want {
				t.Errorf("Scan(%v) = %q, want %q", tc.in, p, tc.want)
			}
		})
	}
}

func TestRoutineCatchUpPolicy_Scan_UnsupportedType(t *testing.T) {
	var p models.RoutineCatchUpPolicy
	if err := p.Scan(42); err == nil {
		t.Fatal("Scan(42) error = nil, want error for unsupported scan type")
	}
}

func TestRoutineCatchUpPolicy_Valid(t *testing.T) {
	valid := []models.RoutineCatchUpPolicy{
		models.CatchUpPolicySummarizeMissed,
		models.CatchUpPolicyEnqueueMissedWithCap,
		models.CatchUpPolicySkipMissed,
	}
	for _, p := range valid {
		if !p.Valid() {
			t.Errorf("Valid(%q) = false, want true", p)
		}
	}
	if models.RoutineCatchUpPolicy("bogus").Valid() {
		t.Error("Valid(bogus) = true, want false")
	}
}

func TestNormaliseCatchUpMax(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"in range untouched", 5, 5},
		{"zero clamps to default", 0, models.CatchUpMaxDefault},
		{"negative clamps to default", -3, models.CatchUpMaxDefault},
		{"exactly one untouched", 1, 1},
		{"exactly ceiling untouched", 1000, 1000},
		{"above ceiling clamps down", 5000, models.CatchUpMaxCeiling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := models.NormaliseCatchUpMax(tc.in); got != tc.want {
				t.Errorf("NormaliseCatchUpMax(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormaliseCatchUpMax_Fixpoint(t *testing.T) {
	for _, in := range []int{-5, 0, 1, 25, 1000, 5000} {
		once := models.NormaliseCatchUpMax(in)
		twice := models.NormaliseCatchUpMax(once)
		if once != twice {
			t.Errorf("NormaliseCatchUpMax not a fixpoint at %d: once=%d twice=%d", in, once, twice)
		}
	}
}
