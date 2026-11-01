package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

type usageSchemaColumn struct{ name, ddl string }

func (r *Repository) upgradeUsageObservationTables(ctx context.Context) error {
	// The tables were introduced before cost coverage became a separate
	// dimension. Keep existing installations readable during the additive
	// migration; SQLite reports a duplicate-column error when the column is
	// already present.
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		if err := r.addUsageSchemaColumns(ctx, table, usageHistoryAdditiveColumns, "retained usage"); err != nil {
			return err
		}
		if err := r.addUsageSchemaColumns(ctx, table, usageNativeAdditiveColumns, "native usage"); err != nil {
			return err
		}
		if err := r.ensureUsageCostCoverage(ctx, table); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) addUsageSchemaColumns(ctx context.Context, table string, columns []usageSchemaColumn, kind string) error {
	for _, column := range columns {
		_, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column.ddl)
		if err != nil && !db.IsDuplicateColumnError(err) {
			return fmt.Errorf("add %s column %s.%s: %w", kind, table, column.name, err)
		}
	}
	return nil
}

func (r *Repository) ensureUsageCostCoverage(ctx context.Context, table string) error {
	_, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN cost_coverage TEXT NOT NULL DEFAULT 'missing'`)
	if err != nil && !db.IsDuplicateColumnError(err) {
		return fmt.Errorf("add cost coverage to %s: %w", table, err)
	}
	// Rows written before cost coverage was split from token coverage have a
	// default of "missing" after the additive ALTER. Recover the old known
	// cost semantics without overwriting an explicitly partial/missing row
	// from the new writer.
	if _, err := r.db.ExecContext(ctx, `UPDATE `+table+`
			SET cost_coverage = coverage
			WHERE cost_coverage = 'missing'
			  AND cost_subcents IS NOT NULL
			  AND COALESCE(cost_basis, 'unknown') NOT IN ('', 'unknown')`); err != nil {
		return fmt.Errorf("backfill cost coverage for %s: %w", table, err)
	}
	return nil
}

func (r *Repository) addUsageTaskIncarnationColumns(ctx context.Context) error {
	for _, table := range []string{"usage_subjects", "usage_execution_bindings"} {
		_, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN task_incarnation_id TEXT NOT NULL DEFAULT ''`)
		if err != nil && !db.IsDuplicateColumnError(err) {
			return fmt.Errorf("add task incarnation to retained usage table %s: %w", table, err)
		}
	}
	return nil
}
