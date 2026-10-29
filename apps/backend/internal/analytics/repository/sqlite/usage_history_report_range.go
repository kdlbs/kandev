package sqlite

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

var usageReportRangeIdentity = []string{"subject_id", "task_id", "task_incarnation_id", "session_id", "session_incarnation_id", "work_key", "segment_key", "bundle_key"}
var usageReportRangeCounts = []string{"token_count", "cost_count", "partial_count", "estimated_count", "stale_count"}

// A report reuses compact dimension aggregates across count, page and totals.
// Range/provider predicates and published build joins run before this grouping.
// It stores no raw observations, source payloads or per-fact history copy.
func (r *Repository) prepareUsageReportRange(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) (bool, error) {
	if filter.GroupBy == usageWorkFallback || filter.GroupBy == usageSubjectSession ||
		filter.Start == nil && filter.End == nil && isDatedUsageGroup(filter.GroupBy) {
		return false, nil
	}
	dimensions := usageReportRangeDimensions()
	text := append(append([]string{}, usageReportRangeIdentity...), dimensions...)
	numbers := usageReportRangeNumbers()
	var definitions []string
	for _, name := range text {
		definitions = append(definitions, name+` TEXT NOT NULL`)
	}
	for _, name := range numbers {
		definitions = append(definitions, name+` BIGINT`)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE usage_report_range (`+strings.Join(definitions, ",")+`)`); err != nil {
		return false, err
	}
	source := usageReportSource(scope, generation, filter, false)
	var values []string
	for range usageReportRangeIdentity {
		values = append(values, `''`)
	}
	values = append(values, dimensions...)
	for _, name := range numbers {
		values = append(values, `CAST(SUM(`+name+`) AS BIGINT)`)
	}
	columns := append(append([]string{}, text...), numbers...)
	query := `WITH report_range_source AS (` + source.sql + `) INSERT INTO usage_report_range (` + strings.Join(columns, ",") + `)
 SELECT ` + strings.Join(values, ",") + ` FROM report_range_source GROUP BY ` + strings.Join(dimensions, ",")
	_, err := tx.ExecContext(ctx, r.db.Rebind(query), source.args...)
	return err == nil, err
}

func usageReportRangeDimensions() []string {
	columns := []string{"coverage_kind", "period", "calendar_basis", "cost_basis"}
	for _, name := range usageProjectionDimensionColumns {
		if name != "usage_date" {
			columns = append(columns, name)
		}
	}
	return columns
}

func usageReportRangeNumbers() []string {
	columns := append([]string{}, usageProjectionAmountColumns...)
	columns = append(columns, usageProjectionMissingColumns...)
	return append(columns, usageReportRangeCounts...)
}

func usageReportReadSource(scope, generation string, filter models.SessionUsageFilter, totals, cached bool) usageReportQuery {
	if cached {
		return usageReportQuery{sql: `SELECT * FROM usage_report_range`}
	}
	return usageReportSource(scope, generation, filter, totals)
}
