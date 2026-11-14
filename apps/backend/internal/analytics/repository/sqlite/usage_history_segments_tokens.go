package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

func usageKnownTokenPredicate(alias string) string {
	var terms []string
	for _, column := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "reasoning_tokens", "total_tokens"} {
		terms = append(terms, alias+column+` IS NOT NULL`)
	}
	return `(` + strings.Join(terms, ` OR `) + `)`
}

func (r *Repository) usageSegmentTokenAlternativesTx(ctx context.Context, tx *sqlx.Tx, row indexedObservation, state *usageSegmentReduction) error {
	knownDated := row.Measurement.CoverageStart == nil
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		predicate := `coverage_start IS NULL AND NOT ` + usageKnownTokenPredicate("") + ` AND selection_token_bundle<>''`
		if knownDated {
			predicate = `coverage_start IS NOT NULL AND ` + usageKnownTokenPredicate("")
		}
		found, err := r.usageSegmentTokenAlternativeTx(ctx, tx, table, predicate, row)
		if err != nil {
			return err
		}
		if found {
			state.SuppressUnknownTokens = knownDated
			state.Choice.Ambiguous = state.Choice.Ambiguous || !knownDated
			return nil
		}
	}
	return nil
}

// Partial indexes contain only the relevant dated-known or lifetime-unknown
// rows. Each equality/range probe returns at most one indexed candidate.
func (r *Repository) usageSegmentTokenAlternativeTx(ctx context.Context, tx *sqlx.Tx, table, predicate string, row indexedObservation) (bool, error) {
	base := `SELECT id FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND ` + predicate + ` AND `
	queries := []struct {
		condition string
		args      []any
	}{
		{`selection_bundle=? AND selection_segment=?`, []any{row.Bundle, row.Segment}},
		{`selection_bundle>?`, []any{row.Bundle}},
		{`selection_bundle<?`, []any{row.Bundle}},
	}
	for _, query := range queries {
		var id string
		args := append([]any{row.Measurement.ScopeID, row.WorkKey}, query.args...)
		err := tx.GetContext(ctx, &id, r.db.Rebind(base+query.condition+` LIMIT 1`), args...)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}

func usageLifetimeTokenAuthority(alias string) string {
	return alias + `.token_source<>'' AND EXISTS(SELECT 1 FROM usage_projection_contributions lt
 WHERE lt.scope_id=` + alias + `.scope_id AND lt.generation=` + alias + `.generation AND lt.build_id=` + alias + `.build_id
 AND lt.fact_key=` + alias + `.fact_key AND lt.metric_kind='tokens' AND ` + usageKnownTokenPredicate("lt.") + `)`
}
