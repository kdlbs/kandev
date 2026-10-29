package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

func (r *Repository) usageSegmentGroupStateTx(ctx context.Context, tx *sqlx.Tx, choice usageSegmentChoice, row indexedObservation) (usageSegmentReduction, error) {
	state := usageSegmentReduction{Choice: choice, Group: row.GroupKey}
	if err := r.usageSegmentTokenAlternativesTx(ctx, tx, row, &state); err != nil {
		return state, err
	}
	if err := r.usageSegmentCostAlternativesTx(ctx, tx, row, &state); err != nil {
		return state, err
	}
	if row.Measurement.CoverageStart == nil {
		ambiguous, err := r.usageSegmentLifetimeAlternativesTx(ctx, tx, row, choice)
		if err != nil {
			return state, err
		}
		state.Choice.Ambiguous = state.Choice.Ambiguous || ambiguous
	}
	if row.Measurement.Source != usageSourceNative {
		return state, nil
	}
	for _, cost := range []bool{false, true} {
		bundle := &state.Choice.Tokens
		if cost {
			bundle = &state.Choice.Cost
		}
		if *bundle == "" || *bundle == row.Bundle {
			continue
		}
		overlaps, err := r.usageSegmentNativeOverlapTx(ctx, tx, row, *bundle, cost)
		if err != nil {
			return state, err
		}
		if !overlaps {
			*bundle = row.Bundle
		}
	}
	return state, nil
}

func (r *Repository) usageSegmentLifetimeAlternativesTx(ctx context.Context, tx *sqlx.Tx, row indexedObservation, choice usageSegmentChoice) (bool, error) {
	for column, bundle := range map[string]string{"selection_token_bundle": choice.Tokens, "selection_cost_bundle": choice.Cost} {
		if bundle == "" {
			continue
		}
		for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
			for _, comparison := range []string{`>?`, `>'' AND ` + column + `<?`} {
				var other string
				query := `SELECT ` + column + ` FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND ` + column + comparison + ` ORDER BY ` + column + ` LIMIT 1`
				err := tx.GetContext(ctx, &other, r.db.Rebind(query), row.Measurement.ScopeID, row.WorkKey, bundle)
				if err == nil {
					return true, nil
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return false, err
				}
			}
		}
	}
	return false, nil
}

// A selected coarse decomposition can suppress overlapping points, but cannot
// claim points outside its measured coverage or a different known model.
func (r *Repository) usageSegmentNativeOverlapTx(ctx context.Context, tx *sqlx.Tx, row indexedObservation, bundle string, cost bool) (bool, error) {
	column := "selection_token_bundle"
	if cost {
		column = "selection_cost_bundle"
	}
	m := row.Measurement
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		query := `SELECT id FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND ` + column + `=? AND model=?
		 AND (coverage_end IS NULL OR coverage_end>?) AND (coverage_start IS NULL OR coverage_start<=?)
		 AND (provider='' OR provider=?) LIMIT 1`
		var id string
		err := tx.GetContext(ctx, &id, r.db.Rebind(query), m.ScopeID, row.WorkKey, bundle, m.Model, m.CoverageStart, m.CoverageStart, m.Provider)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}
