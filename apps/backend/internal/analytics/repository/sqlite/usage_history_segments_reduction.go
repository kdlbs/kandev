package sqlite

import (
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/analytics/models"
)

type usageSegmentReduction struct {
	Choice                usageSegmentChoice
	Group                 string
	AfterTable            string
	AfterID               string
	Tokens, Cost          *indexedObservation
	Subject               string
	Shared, Conflict      bool
	SuppressUnknownCost   bool
	SuppressUnknownTokens bool
}

func (state *usageSegmentReduction) add(row indexedObservation) {
	if row.Bundle != state.Choice.Tokens && row.Bundle != state.Choice.Cost {
		return
	}
	if state.Subject == "" {
		state.Subject = row.Measurement.SubjectID
	}
	state.Shared = state.Shared || state.Subject != row.Measurement.SubjectID || row.Measurement.AttributionStatus == models.UsageAttributionAmbiguous
	if row.Bundle == state.Choice.Tokens {
		state.addTokens(row)
	}
	if row.Bundle == state.Choice.Cost && (state.Cost == nil || usageSegmentCandidateBetter(row, *state.Cost)) {
		copy := row
		state.Cost = &copy
	}
}

func (state *usageSegmentReduction) addTokens(row indexedObservation) {
	if state.Tokens != nil && usageSegmentSameRevision(row, *state.Tokens) &&
		(row.Measurement.Model != state.Tokens.Measurement.Model || row.Measurement.Provider != state.Tokens.Measurement.Provider || !tokenValuesEqual(row.Measurement, state.Tokens.Measurement)) {
		state.Conflict = true
	}
	if state.Tokens == nil || usageSegmentCandidateBetter(row, *state.Tokens) {
		copy := row
		state.Tokens = &copy
	}
}

func usageSegmentSameRevision(a, b indexedObservation) bool {
	return a.ItemID == b.ItemID && a.Measurement.Source == b.Measurement.Source && a.Measurement.Revision == b.Measurement.Revision
}

func usageSegmentCandidateBetter(a, b indexedObservation) bool {
	if a.Measurement.Source == b.Measurement.Source && a.ItemID == b.ItemID && a.Measurement.Revision != b.Measurement.Revision {
		return a.Measurement.Revision > b.Measurement.Revision
	}
	if usageCoverageRank(a.Measurement.Coverage) != usageCoverageRank(b.Measurement.Coverage) {
		return usageCoverageRank(a.Measurement.Coverage) > usageCoverageRank(b.Measurement.Coverage)
	}
	if a.Measurement.Stale != b.Measurement.Stale {
		return !a.Measurement.Stale
	}
	if !a.Measurement.CollectedAt.Equal(b.Measurement.CollectedAt) {
		return a.Measurement.CollectedAt.After(b.Measurement.CollectedAt)
	}
	return selectionCandidateBetter(a, b)
}

func (state usageSegmentReduction) fact() (models.UsageCanonicalFact, bool) {
	base := state.Tokens
	if base == nil {
		base = state.Cost
	}
	if base == nil {
		return models.UsageCanonicalFact{}, false
	}
	fact := selectEquivalentObservations([]indexedObservation{*base})
	fact.SegmentKey = base.Segment
	fact.BundleKey = base.Bundle
	fact.Key = uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-fact:"+base.Measurement.ScopeID+":"+base.WorkKey+":"+base.GroupKey)).String()
	state.applyTokens(&fact)
	clearUsageFactCost(&fact)
	// If no cost representation exists, retain the ordinary missing-cost
	// counter. A different selected cost representation owns its own counter.
	fact.CostMetricAbsent = state.Choice.Cost != ""
	if fact.TokenMetricAbsent && state.SuppressUnknownTokens && state.Cost == nil {
		fact.CostMetricAbsent = true
	}
	state.applyCost(&fact)
	if state.Choice.Ambiguous || state.Conflict {
		markUsageOverlapAmbiguous(&fact)
	}
	if state.Shared && base.Measurement.Source != usageSourceNative && base.EventID == "" {
		fact.Measurement.SubjectID, fact.Measurement.TaskID, fact.Measurement.SessionID, fact.Measurement.UsageBindingID = "", "", "", ""
		fact.Measurement.AttributionStatus = models.UsageAttributionAmbiguous
		fact.Partial = true
	}
	return fact, !fact.TokenMetricAbsent || !fact.CostMetricAbsent
}

func (state usageSegmentReduction) applyTokens(fact *models.UsageCanonicalFact) {
	if state.Tokens == nil || state.Tokens.TokenBundle == "" {
		clearUsageFactTokens(fact)
	}
	if state.Tokens != nil && state.SuppressUnknownTokens && !usageMeasurementHasCanonicalTokens(state.Tokens.Measurement) {
		clearUsageFactTokens(fact)
		markUsageOverlapAmbiguous(fact)
	}
}

func (state usageSegmentReduction) applyCost(fact *models.UsageCanonicalFact) {
	fact.Measurement.Estimated = false
	if state.Cost != nil && state.Cost.CostBundle != "" {
		m := state.Cost.Measurement
		if m.CostSubcents == nil && state.SuppressUnknownCost {
			fact.CostMetricAbsent = true
			markUsageOverlapAmbiguous(fact)
			return
		}
		fact.Measurement.CostSubcents, fact.Measurement.Currency = cloneInt64(m.CostSubcents), m.Currency
		fact.Measurement.CostBasis, fact.Measurement.CostCoverage = m.CostBasis, m.CostCoverage
		fact.CostSource = metricSource(*state.Cost)
		fact.CostMetricAbsent = false
		fact.Measurement.Estimated = m.Estimated || m.CostBasis == string(models.UsageCostBasisEstimated)
		fact.Measurement.Stale = fact.Measurement.Stale || m.Stale
		if m.CollectedAt.After(fact.Measurement.CollectedAt) {
			fact.Measurement.CollectedAt = m.CollectedAt
		}
	}
}
