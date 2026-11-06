package sqlite

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

// A single coverage aggregate and its cursor bound the resolver's memory and each transaction's source reads.
type usageProjectionNativeCoverage struct {
	Coarse              indexedObservation
	Total               models.SessionUsageMeasurement
	Found, Ready        bool
	AfterTable, AfterID string
}

func (r *Repository) ensureUsageResolutionRepresentativeTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state *usageProjectionResolution) error {
	if state.Representative != nil {
		return nil
	}
	var ref selectionReference
	query := `SELECT id, 'session_usage_measurements' AS source_table, selection_coverage FROM session_usage_measurements WHERE scope_id = ? AND selection_partition = ? AND retired = 0 AND selection_coverage = ?
		UNION ALL SELECT id, 'session_usage_buckets' AS source_table, selection_coverage FROM session_usage_buckets WHERE scope_id = ? AND selection_partition = ? AND retired = 0 AND selection_coverage = ? ORDER BY source_table, id LIMIT 1`
	if err := tx.GetContext(ctx, &ref, r.db.Rebind(query), work.Scope, work.Work, state.Coverage, work.Scope, work.Work, state.Coverage); err != nil {
		return err
	}
	row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
	if err == nil {
		state.Representative = &row
	}
	return err
}

func usageResolutionCandidatePredicate(state usageProjectionResolution) (string, []any) {
	row := *state.Representative
	if state.Phase == projectionMetricTokens || state.Phase == projectionMetricCost {
		if canonicalCoverageKind(row.Measurement) == usageCoveragePoint {
			return ` AND (selection_coverage = ? OR coverage_start = ?)`, []any{state.Coverage, row.Measurement.CoverageStart}
		}
		return ` AND selection_coverage = ?`, []any{state.Coverage}
	}
	if state.Phase == projectionResolutionNative {
		row = state.Native.Coarse
	}
	predicate, args := usageSelectionOverlap(row)
	predicate += ` AND selection_coverage <> ?`
	args = append(args, row.CoverageKey)
	if row.EventID != "" {
		predicate += ` AND (work_event_id = '' OR work_event_id = ?)`
		args = append(args, row.EventID)
	}
	if state.Phase == projectionResolutionNative {
		predicate += ` AND source = ? AND model = ? AND coverage = ?`
		args = append(args, usageSourceNative, row.Measurement.Model, usageCoverageComplete)
		if row.Measurement.Provider != "" {
			predicate += ` AND provider = ?`
			args = append(args, row.Measurement.Provider)
		}
	}
	return predicate, args
}

func advanceUsageResolutionOverlap(state *usageProjectionResolution, row indexedObservation) bool {
	if equivalentUsageCandidate(*state.Representative, row) || (state.Tokens.EventID != "" && row.EventID != "" && state.Tokens.EventID != row.EventID) {
		return false
	}
	coarse, handled := usageNativeOverlapPair(*state.Tokens, row)
	if !handled {
		state.Ambiguous = true
		state.Keep = state.Keep && !selectionCandidateBetter(row, *state.Tokens)
		return false
	}
	if coarse.Measurement.Coverage != usageCoverageComplete {
		state.Keep = state.Keep && state.Tokens.Measurement.Source == usageSourceNative
		return false
	}
	if reusableUsageNativeCoverage(state.Native, coarse) {
		applyUsageNativeResolution(state, row)
		return false
	}
	state.Native = &usageProjectionNativeCoverage{Coarse: coarse}
	state.PendingOther, state.Phase = &row, projectionResolutionNative
	return true
}

func reusableUsageNativeCoverage(cache *usageProjectionNativeCoverage, coarse indexedObservation) bool {
	return cache != nil && cache.Ready && cache.Coarse.CoverageKey == coarse.CoverageKey && cache.Coarse.EventID == coarse.EventID && usageCostDimensionsMatch(cache.Coarse, coarse)
}

func applyUsageNativeResolution(state *usageProjectionResolution, other indexedObservation) {
	coarse, _ := usageNativeOverlapPair(*state.Tokens, other)
	keep, ambiguous := usageNativeCoverageDecision(*state.Tokens, coarse, state.Native.Total, state.Native.Found)
	state.Keep, state.Ambiguous = state.Keep && keep, state.Ambiguous || ambiguous
}

func (r *Repository) nextUsageResolutionCoverageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, after string) (string, bool, error) {
	var coverage sql.NullString
	err := tx.GetContext(ctx, &coverage, r.db.Rebind(`SELECT MIN(selection_coverage) FROM (
        SELECT selection_coverage FROM session_usage_measurements WHERE scope_id = ? AND selection_partition = ? AND retired = 0 AND selection_coverage > ?
        UNION ALL SELECT selection_coverage FROM session_usage_buckets WHERE scope_id = ? AND selection_partition = ? AND retired = 0 AND selection_coverage > ?) candidates`), work.Scope, work.Work, after, work.Scope, work.Work, after)
	return coverage.String, coverage.Valid, err
}

func (r *Repository) carryUsageNativeCoverageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, previous usageProjectionResolution) (bool, error) {
	if previous.Native == nil || !previous.Native.Ready {
		return false, nil
	}
	coverage, found, err := r.nextUsageResolutionCoverageTx(ctx, tx, work, previous.Coverage)
	if err != nil || !found {
		return false, err
	}
	state := usageProjectionResolution{Coverage: coverage, Phase: projectionMetricTokens, Keep: true, Native: previous.Native}
	return true, r.saveUsageProjectionResolutionTx(ctx, tx, work, state)
}
