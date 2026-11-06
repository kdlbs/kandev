package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

func (r *Repository) usageSegmentCostAlternativesTx(ctx context.Context, tx *sqlx.Tx, row indexedObservation, state *usageSegmentReduction) error {
	predicate := `coverage_start IS NULL AND cost_subcents IS NULL AND selection_cost_bundle<>''`
	lifetime := row.Measurement.CoverageStart == nil
	if lifetime {
		predicate = `coverage_start IS NOT NULL AND cost_subcents IS NOT NULL`
	}
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var id string
		query := `SELECT id FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND selection_segment=? AND selection_bundle=? AND ` + predicate + ` LIMIT 1`
		err := tx.GetContext(ctx, &id, r.db.Rebind(query), row.Measurement.ScopeID, row.WorkKey, row.Segment, row.Bundle)
		if err == nil {
			state.SuppressUnknownCost = lifetime
			state.Choice.Ambiguous = state.Choice.Ambiguous || !lifetime
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// Unknown cost is retained evidence, but cannot suppress a known dated price.
// IS NOT NULL deliberately includes a measured monetary zero.
func usageLifetimeCostAuthority(alias string) string {
	return alias + `.cost_source<>'' AND EXISTS(SELECT 1 FROM usage_projection_contributions lp
 WHERE lp.scope_id=` + alias + `.scope_id AND lp.generation=` + alias + `.generation AND lp.build_id=` + alias + `.build_id
 AND lp.fact_key=` + alias + `.fact_key AND lp.metric_kind='cost' AND lp.cost_subcents IS NOT NULL)`
}
