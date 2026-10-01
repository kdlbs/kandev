package replay

import "testing"

func ok(i int, keys ...string) Attempt { return Attempt{Index: i, OK: true, Keys: keys} }
func bad(i int) Attempt                { return Attempt{Index: i} }

func TestCaseScoreRoundsHalfUp(t *testing.T) {
	exp := expectation{
		reproduced: []expected{{key: "a"}, {key: "b"}, {key: "c"}},
		avoided:    []expected{},
	}
	// 1 of 3 met = 333.33 -> 333; 2 of 3 = 666.67 -> 667.
	if got := caseScore(exp, ok(1, "a")); got != 333 {
		t.Fatalf("1/3 = %d", got)
	}
	if got := caseScore(exp, ok(1, "a", "b")); got != 667 {
		t.Fatalf("2/3 = %d", got)
	}
	// 1 of 8 = 125 exactly; 1 of 16 = 62.5 -> 63.
	exp16 := expectation{}
	for i := 0; i < 16; i++ {
		exp16.avoided = append(exp16.avoided, expected{key: string(rune('a' + i))})
	}
	// attempt proposes 15 of the avoided keys: 1 avoided met out of 16.
	var keys []string
	for i := 1; i < 16; i++ {
		keys = append(keys, string(rune('a'+i)))
	}
	if got := caseScore(exp16, ok(1, keys...)); got != 63 {
		t.Fatalf("1/16 = %d", got)
	}
}

func TestCaseScoreAvoidedMetWhenKeyAbsent(t *testing.T) {
	exp := expectation{reproduced: []expected{{key: "r"}}, avoided: []expected{{key: "x"}}}
	if got := caseScore(exp, ok(1, "r")); got != 1000 {
		t.Fatalf("reproduce and avoid = %d", got)
	}
	if got := caseScore(exp, ok(1, "x")); got != 0 {
		t.Fatalf("repeat the rejected one = %d", got)
	}
	if got := caseScore(exp, ok(1)); got != 500 {
		t.Fatalf("propose nothing = %d", got)
	}
}

func TestMedianOfMeans(t *testing.T) {
	cases := []struct {
		in   []int64
		want int64
		some bool
	}{
		{nil, 0, false},
		{[]int64{700}, 700, true},
		{[]int64{900, 100}, 100, true},
		{[]int64{300, 100, 200}, 200, true},
	}
	for _, c := range cases {
		got, some := median(c.in)
		if some != c.some || got != c.want {
			t.Fatalf("median(%v) = %d,%v want %d,%v", c.in, got, some, c.want, c.some)
		}
	}
}

func TestSideScoreUsesOnlySucceededAttemptIndexes(t *testing.T) {
	exp := expectation{reproduced: []expected{{key: "a"}}}
	cs := []*caseState{
		{exp: exp, candidate: []Attempt{ok(1, "a"), bad(2), ok(3)}},
		{exp: exp, candidate: []Attempt{ok(1), ok(2, "a"), ok(3, "a")}},
	}
	// attempt 1 mean: (1000+0)/2 = 500; attempt 2: only case 2 = 1000;
	// attempt 3: (0+1000)/2 = 500. median of [500,1000,500] = 500.
	got, some := sideScore(cs, candidateSide)
	if !some || got != 500 {
		t.Fatalf("sideScore = %d,%v", got, some)
	}
}

func TestSideScoreNoSuccessfulAttempt(t *testing.T) {
	cs := []*caseState{{exp: expectation{reproduced: []expected{{key: "a"}}}, candidate: []Attempt{bad(1), bad(2), bad(3)}}}
	if _, some := sideScore(cs, candidateSide); some {
		t.Fatal("no successful attempt must give no score")
	}
}

func TestMeanRoundsHalfUp(t *testing.T) {
	if got := meanThousandths(1, 2); got != 1 { // 0.5 -> 1
		t.Fatalf("1/2 = %d", got)
	}
	if got := meanThousandths(5, 3); got != 2 { // 1.67 -> 2
		t.Fatalf("5/3 = %d", got)
	}
	if got := meanThousandths(4, 3); got != 1 { // 1.33 -> 1
		t.Fatalf("4/3 = %d", got)
	}
}
