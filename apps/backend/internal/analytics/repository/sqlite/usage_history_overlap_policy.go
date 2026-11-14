package sqlite

import "github.com/kandev/kandev/internal/analytics/models"

func usageNativeOverlapPair(winner, other indexedObservation) (indexedObservation, bool) {
	native, coarse := winner, other
	if native.Measurement.Source != usageSourceNative {
		native, coarse = other, winner
	}
	return coarse, native.Measurement.Source == usageSourceNative && coarse.Measurement.Source != usageSourceNative && coarse.WorkStatus == usageIdentityProven
}

func usageNativeCoverageDecision(winner, coarse indexedObservation, aggregate models.SessionUsageMeasurement, complete bool) (bool, bool) {
	// Turn evidence is independent of token coverage and is absent from v1 collector reports.
	left, right := aggregate, coarse.Measurement
	left.Turns, right.Turns = nil, nil
	equivalent := complete && tokenValuesEqual(left, right)
	preferNative := equivalent
	if !preferNative && aggregate.TotalTokens != nil && coarse.Measurement.TotalTokens != nil {
		preferNative = *aggregate.TotalTokens >= *coarse.Measurement.TotalTokens
	}
	if coarse.Measurement.TotalTokens == nil {
		preferNative = true
	}
	return preferNative == (winner.Measurement.Source == usageSourceNative), !equivalent
}

func accumulateUsageNativeCoverage(total *models.SessionUsageMeasurement, found *bool, row, coarse indexedObservation) {
	m := row.Measurement
	if m.Source != usageSourceNative || m.Model != coarse.Measurement.Model || (coarse.Measurement.Provider != "" && m.Provider != coarse.Measurement.Provider) || m.Coverage != usageCoverageComplete {
		return
	}
	if !*found {
		*total, *found = cloneUsageMeasurement(m), true
		return
	}
	mergeMeasurementValues(total, m)
}

func usageCostDimensionsMatch(candidate, tokens indexedObservation) bool {
	return candidate.Measurement.Model == tokens.Measurement.Model && candidate.Measurement.Provider == tokens.Measurement.Provider
}

func equivalentUsageCandidate(representative, row indexedObservation) bool {
	return row.CoverageKey == representative.CoverageKey || sameUsagePoint(representative, row)
}

func markUsageOverlapAmbiguous(fact *models.UsageCanonicalFact) {
	fact.Partial, fact.Reason = true, "ambiguous_overlap"
	fact.Measurement.Coverage = usageCoveragePartial
	fact.Measurement.CostCoverage = mergeCoverage(fact.Measurement.CostCoverage, usageCoveragePartial)
}
