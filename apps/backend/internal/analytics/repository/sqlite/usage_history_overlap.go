package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func sameUsagePoint(a, b indexedObservation) bool {
	return canonicalCoverageKind(a.Measurement) == usageCoveragePoint && canonicalCoverageKind(b.Measurement) == usageCoveragePoint && a.Measurement.CoverageStart.Equal(*b.Measurement.CoverageStart) && (a.EventID == "" || b.EventID == "" || a.EventID == b.EventID)
}

func (r *Repository) selectUsageOverlapTx(ctx context.Context, tx *sqlx.Tx, group []indexedObservation) (models.UsageCanonicalFact, bool, bool, error) {
	representative := group[0]
	if representative.Measurement.CoverageStart == nil {
		return legacyEquivalentFact(group), true, false, nil
	}
	refs, err := r.selectionReferencesTx(ctx, tx, representative.Measurement.ScopeID, representative.WorkKey, "", &representative)
	if err != nil {
		return models.UsageCanonicalFact{}, false, false, err
	}
	if len(refs) > maxSessionUsageBatch {
		return models.UsageCanonicalFact{}, false, true, nil
	}
	group, others, err := r.mergeUsagePointReferencesTx(ctx, tx, group, refs)
	if err != nil {
		return models.UsageCanonicalFact{}, false, false, err
	}
	fact := legacyEquivalentFact(group)
	winner := selectEquivalentToken(group)
	if winner.CoverageKey != representative.CoverageKey {
		return fact, false, false, nil
	}
	for _, other := range others {
		if winner.EventID != "" && other.EventID != "" && winner.EventID != other.EventID {
			continue
		}
		keep, ambiguous, handled, preparing, err := r.nativeUsageOverlapTx(ctx, tx, winner, other)
		if err != nil {
			return fact, false, false, err
		}
		if preparing {
			return fact, false, true, nil
		}
		if !handled {
			keep, ambiguous = !selectionCandidateBetter(other, winner), true
		}
		if ambiguous {
			markUsageOverlapAmbiguous(&fact)
		}
		if !keep {
			return fact, false, false, nil
		}
	}
	return fact, true, false, nil
}

func (r *Repository) mergeUsagePointReferencesTx(ctx context.Context, tx *sqlx.Tx, group []indexedObservation, refs []selectionReference) ([]indexedObservation, []indexedObservation, error) {
	representative := group[0]
	var others []indexedObservation
	for _, ref := range refs {
		other, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return nil, nil, err
		}
		if sameUsagePoint(representative, other) {
			group = append(group, other)
		} else {
			others = append(others, other)
		}
	}
	return group, others, nil
}

func legacyEquivalentFact(group []indexedObservation) models.UsageCanonicalFact {
	fact := selectEquivalentObservations(group)
	sources := map[string]bool{}
	for _, row := range group {
		if row.Metadata == "" && row.Measurement.Source != usageSourceNative {
			sources[row.Measurement.Source] = true
		}
	}
	// Legacy source rows do not carry a versioned equivalence declaration.
	if len(sources) > 1 {
		fact.Partial = true
		fact.Reason = "legacy_equivalence_unknown"
		fact.Measurement.Coverage = usageCoveragePartial
	}
	return fact
}

func (r *Repository) nativeUsageOverlapTx(ctx context.Context, tx *sqlx.Tx, winner, other indexedObservation) (bool, bool, bool, bool, error) {
	coarse, handled := usageNativeOverlapPair(winner, other)
	if !handled {
		return false, false, false, false, nil
	}
	if coarse.Measurement.Coverage != usageCoverageComplete {
		return winner.Measurement.Source == usageSourceNative, false, true, false, nil
	}
	aggregate, complete, preparing, err := r.nativeCoverageAggregateTx(ctx, tx, coarse)
	if err != nil {
		return false, false, true, false, err
	}
	if preparing {
		return false, false, true, true, nil
	}
	keep, ambiguous := usageNativeCoverageDecision(winner, coarse, aggregate, complete)
	return keep, ambiguous, true, false, nil
}

func (r *Repository) nativeCoverageAggregateTx(ctx context.Context, tx *sqlx.Tx, coarse indexedObservation) (models.SessionUsageMeasurement, bool, bool, error) {
	refs, err := r.selectionReferencesTx(ctx, tx, coarse.Measurement.ScopeID, coarse.WorkKey, "", &coarse)
	if err != nil {
		return models.SessionUsageMeasurement{}, false, false, err
	}
	if len(refs) > maxSessionUsageBatch {
		return models.SessionUsageMeasurement{}, false, true, nil
	}
	var total models.SessionUsageMeasurement
	found := false
	for _, ref := range refs {
		row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return total, false, false, err
		}
		accumulateUsageNativeCoverage(&total, &found, row, coarse)

	}
	return total, found, false, nil
}
