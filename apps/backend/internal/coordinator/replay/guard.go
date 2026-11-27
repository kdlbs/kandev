package replay

// reproduces reports whether the key is in more than half of the side's
// successful attempts of the case; a failed attempt shrinks the denominator
// and is never a miss.
func (c *caseState) reproduces(s side, key string) bool {
	ok := c.successes(s)
	hits := 0
	for _, a := range ok {
		if hasKey(a.Keys, key) {
			hits++
		}
	}
	return len(ok) > 0 && hits*2 > len(ok)
}

// guardFlips returns the expected reproduced proposals the baseline reproduces
// and the candidate does not, over the cases handed in (compared cases), in
// case then proposal order.
func guardFlips(cases []*caseState) []Flip {
	var flips []Flip
	for _, c := range cases {
		for _, x := range c.exp.reproduced {
			if c.reproduces(baselineSide, x.key) && !c.reproduces(candidateSide, x.key) {
				flips = append(flips, Flip{TurnID: c.id, ProposalID: x.proposalID})
			}
		}
	}
	return flips
}
