package replay

import "testing"

func TestJudge(t *testing.T) {
	cases := []struct {
		name        string
		guard       string
		heldOut     int
		cand, base  int64
		wantVerdict string
		wantReason  string
	}{
		{"improvement at 50 and 20", GuardPass, 20, 650, 600, VerdictImprovement, ""},
		{"gain 49 is not", GuardPass, 20, 649, 600, VerdictNotAnImprovement, ""},
		{"negative", GuardPass, 25, 500, 600, VerdictNotAnImprovement, ""},
		{"19 held out", GuardPass, 19, 900, 100, VerdictUnmeasured, ReasonTooFew},
		{"blocked with 19", GuardBlocked, 19, 900, 100, VerdictNotAnImprovement, ""},
		{"blocked with 30 and a big gain", GuardBlocked, 30, 900, 100, VerdictNotAnImprovement, ""},
		{"zero held out", GuardPass, 0, 0, 0, VerdictUnmeasured, ReasonTooFew},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, reason := judge(c.guard, c.heldOut, c.cand, c.base)
			if v != c.wantVerdict || reason != c.wantReason {
				t.Fatalf("got %q/%q want %q/%q", v, reason, c.wantVerdict, c.wantReason)
			}
		})
	}
}
