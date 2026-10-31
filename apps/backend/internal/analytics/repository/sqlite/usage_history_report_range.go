package sqlite

import (
	"context"
	"slices"
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
	if usageReportUsesUTCMonthSummaries(filter) {
		dimensionFilter := filter
		dimensionFilter.Provider = ""
		if err := r.prepareUsageReportUTCMonthDimensions(ctx, tx, scope, generation, dimensionFilter); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS usage_report_range`); err != nil {
		return false, err
	}
	identityGroup := usageReportRangeIdentityForFilter(filter)
	if usageReportRangeShouldBypass(filter, identityGroup) {
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
	source := usageReportSourceForEngine(scope, generation, filter, false, r.db.DriverName() == usageDriverPostgres)
	if usageAllTimeSummaryCandidate(filter) {
		safe, err := r.usageReportAllTimeSummarySafe(ctx, tx, scope, generation, filter)
		if err != nil {
			return false, err
		}
		if safe {
			source = reportAllTimeSummarySource(scope, generation, filter)
		}
	}
	var values []string
	for _, name := range usageReportRangeIdentity {
		if slices.Contains(identityGroup, name) {
			values = append(values, name)
		} else {
			values = append(values, `''`)
		}
	}
	values = append(values, dimensions...)
	for _, name := range numbers {
		values = append(values, `CAST(SUM(`+name+`) AS BIGINT)`)
	}
	columns := append(append([]string{}, text...), numbers...)
	groupBy := append(append([]string{}, identityGroup...), dimensions...)
	query := `WITH report_range_source AS (` + source.sql + `) INSERT INTO usage_report_range (` + strings.Join(columns, ",") + `)
	 SELECT ` + strings.Join(values, ",") + ` FROM report_range_source GROUP BY ` + strings.Join(groupBy, ",")
	_, err := tx.ExecContext(ctx, r.db.Rebind(query), source.args...)
	return err == nil, err
}

func usageReportRangeShouldBypass(filter models.SessionUsageFilter, identityGroup []string) bool {
	needsSubjectRows := filter.GroupBy == usageWorkFallback || filter.GroupBy == usageSubjectSession ||
		filter.Start == nil && filter.End == nil && isDatedUsageGroup(filter.GroupBy)
	if !needsSubjectRows {
		return false
	}
	return len(identityGroup) == 0 || len(filter.TaskIDs) == 0 && len(filter.SessionIDs) == 0
}

func usageReportRangeIdentityForFilter(filter models.SessionUsageFilter) []string {
	if filter.GroupBy != usageWorkFallback && filter.GroupBy != usageSubjectSession ||
		len(filter.TaskIDs) == 0 && len(filter.SessionIDs) == 0 {
		return nil
	}
	return usageReportRangeIdentity[:5]
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

func usageReportReadSourceForEngine(scope, generation string, filter models.SessionUsageFilter, totals, cached, postgres bool) usageReportQuery {
	if cached {
		return usageReportQuery{sql: `SELECT * FROM usage_report_range`}
	}
	return usageReportSourceForEngine(scope, generation, filter, totals, postgres)
}

func usageReportProviderOptionsSourceForEngine(scope, generation string, filter models.SessionUsageFilter, cached, postgres bool) usageReportQuery {
	if cached {
		return usageReportProviderOptionsProjection(usageReportReadSourceForEngine(scope, generation, filter, false, true, postgres))
	}
	if filter.Start != nil || filter.End != nil || len(filter.TaskIDs) > 0 || len(filter.SessionIDs) > 0 {
		return usageReportProviderOptionsProjection(usageReportReadSourceForEngine(scope, generation, filter, false, false, postgres))
	}
	predicate := reportScopePredicate(scope, generation, filter)
	if !isDatedUsageGroup(filter.GroupBy) {
		// All-time options use a compact per-work dimension that already applies
		// the same lifetime-alternative mask as the report source.
		query := `SELECT c.provider FROM usage_projection_provider_options c WHERE ` + predicate.sql
		return usageReportProviderOptionsProjection(usageReportQuery{sql: query, args: predicate.args})
	}
	// Dated options come from dated summaries. Undated intervals have no
	// calendar summary row, so include their indexed published contribution.
	query := `SELECT provider FROM usage_daily_summaries c WHERE ` + predicate.sql
	args := append([]any(nil), predicate.args...)
	intervalPredicate := reportScopePredicate(scope, generation, filter)
	query += ` UNION ALL SELECT c.provider FROM usage_projection_contributions c
 JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
 WHERE ` + intervalPredicate.sql + ` AND c.coverage_kind='interval' AND c.usage_date=''`
	args = append(args, intervalPredicate.args...)
	return usageReportProviderOptionsProjection(usageReportQuery{sql: query, args: args})
}

func usageReportProviderOptionsProjection(source usageReportQuery) usageReportQuery {
	source.sql = `SELECT DISTINCT CASE WHEN provider='' THEN '__unknown_provider__' ELSE provider END FROM (` + source.sql + `) report_providers ORDER BY 1`
	return source
}
