package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureUsageSegmentSchema(ctx context.Context) error {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		for _, column := range []string{
			`selection_bundle TEXT NOT NULL DEFAULT ''`, `selection_segment TEXT NOT NULL DEFAULT ''`,
			`selection_group TEXT NOT NULL DEFAULT ''`, `selection_layout INTEGER NOT NULL DEFAULT 0`,
			`selection_token_bundle TEXT NOT NULL DEFAULT ''`, `selection_cost_bundle TEXT NOT NULL DEFAULT ''`,
			`selection_scan_key TEXT NOT NULL DEFAULT ''`,
			`selection_span_seconds BIGINT NOT NULL DEFAULT 0`,
		} {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column); err != nil && !db.IsDuplicateColumnError(err) {
				return err
			}
		}
		for _, suffix := range []string{
			`segment_layout ON ` + table + `(scope_id,selection_partition,retired,selection_layout,selection_segment)`,
			`segment_group ON ` + table + `(scope_id,selection_partition,retired,selection_group,id)`,
			`segment_tokens ON ` + table + `(scope_id,selection_partition,retired,selection_token_bundle,selection_group,id)`,
			`segment_cost ON ` + table + `(scope_id,selection_partition,retired,selection_cost_bundle,selection_group,id)`,
			`segment_pending ON ` + table + `(scope_id,id) WHERE selection_bundle='' AND scope_id<>''`,
			`segment_scan ON ` + table + `(scope_id,selection_partition,retired,selection_scan_key,id)`,
			`segment_span ON ` + table + `(scope_id,selection_partition,retired,selection_span_seconds)`,
			`segment_scan_pending ON ` + table + `(scope_id,id) WHERE selection_scan_key='' AND scope_id<>''`,
			`segment_token_range ON ` + table + `(scope_id,selection_partition,retired,selection_token_bundle,model,coverage_end,coverage_start)`,
			`segment_cost_range ON ` + table + `(scope_id,selection_partition,retired,selection_cost_bundle,model,coverage_end,coverage_start)`,
			`segment_priced ON ` + table + `(scope_id,selection_partition,retired,selection_segment,selection_bundle,coverage_start,id) WHERE cost_subcents IS NOT NULL`,
			`segment_unpriced ON ` + table + `(scope_id,selection_partition,retired,selection_segment,selection_bundle,coverage_start,id) WHERE cost_subcents IS NULL AND selection_cost_bundle<>''`,
			`segment_dated_tokens ON ` + table + `(scope_id,selection_partition,retired,selection_bundle,selection_segment,id) WHERE coverage_start IS NOT NULL AND ` + usageKnownTokenPredicate(""),
			`segment_unknown_tokens ON ` + table + `(scope_id,selection_partition,retired,selection_bundle,selection_segment,id) WHERE coverage_start IS NULL AND NOT ` + usageKnownTokenPredicate("") + ` AND selection_token_bundle<>''`,
			`subject_source ON ` + table + `(scope_id,subject_id,id)`,
		} {
			if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_`+table+`_`+suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) ensureUsageProjectionSegmentSchema(ctx context.Context) error {
	for _, column := range []string{"segment_key", "bundle_key"} {
		if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_canonical_facts ADD COLUMN `+column+` TEXT NOT NULL DEFAULT ''`); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_canonical_segment_lifetime ON usage_canonical_facts(scope_id,generation,work_key,segment_key,build_id) WHERE coverage_kind='lifetime'`)
	return err
}
