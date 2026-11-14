package sqlite

// Quality describes a coherent source decomposition, after duplicate item
// snapshots have been reduced. A complete point event does not establish the
// completeness of a session or source-day aggregate.
type usageSegmentMetricQuality struct {
	Bundle                                  string
	Coverage                                string
	Span                                    int64
	Window                                  usageSegmentWindow
	Present, Known, Complete, Coarse, Stale bool
	Basis                                   int
}

type usageSegmentAssessment struct {
	Choice           usageSegmentChoice
	Bundle, AfterKey string
	Group            usageSegmentReduction
	Tokens, Cost     usageSegmentMetricQuality
}

func (state *usageSegmentAssessment) add(row indexedObservation) {
	series := row.CoverageKey + ":" + row.Bundle
	if state.Bundle != series || state.Group.Group != row.GroupKey {
		state.finishGroup()
	}
	if state.Bundle != series {
		state.finishBundle()
		state.Bundle = series
	}
	if state.Group.Group == "" {
		state.Group = usageSegmentReduction{Choice: usageSegmentChoice{Tokens: row.Bundle, Cost: row.Bundle}, Group: row.GroupKey}
	}
	state.Group.add(row)
	state.AfterKey = row.ScanKey
}

func (state *usageSegmentAssessment) finishGroup() {
	if row := state.Group.Tokens; row != nil && row.TokenBundle != "" {
		state.Tokens.add(*row, false)
	}
	if row := state.Group.Cost; row != nil && row.CostBundle != "" {
		state.Cost.add(*row, true)
	}
	state.Group = usageSegmentReduction{}
}

func (state *usageSegmentAssessment) finishBundle() {
	state.Choice.selectMetric(state.Tokens, false)
	state.Choice.selectMetric(state.Cost, true)
	state.Tokens, state.Cost = usageSegmentMetricQuality{}, usageSegmentMetricQuality{}
}

func (state *usageSegmentAssessment) finish() usageSegmentChoice {
	state.finishGroup()
	state.finishBundle()
	state.Choice.Ready = true
	return state.Choice
}

func (q *usageSegmentMetricQuality) add(row indexedObservation, cost bool) {
	m := row.Measurement
	known, coverage, basis := usageMeasurementHasTokens(m), m.Coverage, 0
	if cost {
		known, coverage, basis = m.CostSubcents != nil, m.CostCoverage, usageCostBasisRank(m.CostBasis)
	}
	complete := known && coverage == usageCoverageComplete && row.WorkStatus == usageIdentityProven
	if !q.Present {
		q.Bundle, q.Complete, q.Basis = row.Bundle, complete, basis
		q.Coverage, q.Span = row.CoverageKey, usageSegmentSpanSeconds(row)
		q.Window = usageSegmentRowWindow(row)
	} else {
		q.Complete = q.Complete && complete
		q.Basis = min(q.Basis, basis)
	}
	q.Present, q.Known = true, q.Known || known
	q.Coarse = q.Coarse || canonicalCoverageKind(m) != usageCoveragePoint
	q.Stale = q.Stale || m.Stale
}

func (choice *usageSegmentChoice) selectMetric(candidate usageSegmentMetricQuality, cost bool) {
	if !candidate.Present {
		return
	}
	winner, bundle := &choice.TokenQuality, &choice.Tokens
	if cost {
		winner, bundle = &choice.CostQuality, &choice.Cost
	}
	choice.Ambiguous = choice.Ambiguous || winner.Present && winner.Bundle != candidate.Bundle
	if !winner.Present || usageSegmentQualityBetter(candidate, *winner) {
		*winner, *bundle = candidate, candidate.Bundle
	}
}

func usageSegmentQualityBetter(a, b usageSegmentMetricQuality) bool {
	// Overlapping non-equivalent windows cannot be split into invented
	// remainders. Prefer the encompassing representation and disclose overlap.
	if a.Coverage != b.Coverage {
		if a.Coarse != b.Coarse {
			return a.Coarse
		}
		if a.Span != b.Span {
			return a.Span > b.Span
		}
		return a.Coverage < b.Coverage
	}
	for _, pair := range [][2]bool{{a.Known, b.Known}, {a.Complete, b.Complete}} {
		if pair[0] != pair[1] {
			return pair[0]
		}
	}
	if a.Basis != b.Basis {
		return a.Basis > b.Basis
	}
	if a.Stale != b.Stale {
		return !a.Stale
	}
	return a.Bundle < b.Bundle
}
