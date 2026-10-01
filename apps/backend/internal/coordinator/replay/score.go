package replay

import "sort"

// expected is one proposal a manager decided, identified by its decision key.
// A key held by several proposals is represented by the smallest proposal id.
type expected struct {
	key        string
	proposalID string
	kind       string
	target     string
}

// expectation is a case's expected reproduced and expected avoided proposals.
type expectation struct {
	reproduced []expected
	avoided    []expected
}

func (e expectation) empty() bool { return len(e.reproduced)+len(e.avoided) == 0 }

func (e expectation) hasKey(key string) bool {
	for _, x := range e.reproduced {
		if x.key == key {
			return true
		}
	}
	for _, x := range e.avoided {
		if x.key == key {
			return true
		}
	}
	return false
}

// caseState is one selected case that is not skipped.
type caseState struct {
	id        string
	exp       expectation
	candidate []Attempt
	baseline  []Attempt
}

type side int

const (
	candidateSide side = iota
	baselineSide
)

func (c *caseState) attempts(s side) []Attempt {
	if s == candidateSide {
		return c.candidate
	}
	return c.baseline
}

func (c *caseState) successes(s side) []Attempt {
	var out []Attempt
	for _, a := range c.attempts(s) {
		if a.OK {
			out = append(out, a)
		}
	}
	return out
}

// ran reports whether at least MinAttemptsRan attempts of the side succeeded.
func (c *caseState) ran(s side) bool { return len(c.successes(s)) >= MinAttemptsRan }

func (c *caseState) compared() bool { return c.ran(candidateSide) && c.ran(baselineSide) }

func hasKey(keys []string, key string) bool {
	i := sort.SearchStrings(keys, key)
	return i < len(keys) && keys[i] == key
}

// caseScore is the share of the case's expectations the attempt met, in
// integer thousandths rounded half up: an expected reproduced proposal is met
// when its key is in the attempt, an expected avoided one when it is not.
func caseScore(e expectation, a Attempt) int64 {
	d := int64(len(e.reproduced) + len(e.avoided))
	var n int64
	for _, x := range e.reproduced {
		if hasKey(a.Keys, x.key) {
			n++
		}
	}
	for _, x := range e.avoided {
		if !hasKey(a.Keys, x.key) {
			n++
		}
	}
	return (2000*n + d) / (2 * d)
}

// meanThousandths is sum/count rounded half up.
func meanThousandths(sum, count int64) int64 { return (2*sum + count) / (2 * count) }

// median is the middle value, the lower of two, and reports false for none.
func median(v []int64) (int64, bool) {
	if len(v) == 0 {
		return 0, false
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[(len(s)-1)/2], true
}

// sideScore is the median of the attempt means: attempt k's mean is the mean
// case score of attempt k over the cases whose attempt k succeeded, and only
// attempt indexes with at least one such case count.
func sideScore(cases []*caseState, s side) (int64, bool) {
	var means []int64
	for k := 1; k <= Attempts; k++ {
		var sum, count int64
		for _, c := range cases {
			for _, a := range c.attempts(s) {
				if a.Index == k && a.OK {
					sum += caseScore(c.exp, a)
					count++
				}
			}
		}
		if count > 0 {
			means = append(means, meanThousandths(sum, count))
		}
	}
	return median(means)
}

// unmatched sums, over the successful attempts of the cases, the keys the
// attempt proposed that match no expectation.
func unmatched(cases []*caseState, s side) int {
	total := 0
	for _, c := range cases {
		for _, a := range c.successes(s) {
			for _, k := range a.Keys {
				if !c.exp.hasKey(k) {
					total++
				}
			}
		}
	}
	return total
}

func comparedOnly(cases []*caseState) []*caseState {
	var out []*caseState
	for _, c := range cases {
		if c.compared() {
			out = append(out, c)
		}
	}
	return out
}
