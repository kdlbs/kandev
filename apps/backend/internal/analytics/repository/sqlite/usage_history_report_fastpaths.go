package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) prepareUsageReportUTCMonthDimensions(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) error {
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE usage_report_utc_month_dimensions (model TEXT NOT NULL, provider TEXT NOT NULL, period TEXT NOT NULL, summarize INTEGER NOT NULL, PRIMARY KEY(model,provider,period))`); err != nil {
		return err
	}
	if err := r.populateUsageReportUTCMonthDimensions(ctx, tx, scope, generation, filter); err != nil {
		return err
	}
	if err := r.excludeUsageReportCurrencyUnsafeMonths(ctx, tx, scope, generation, filter); err != nil {
		return err
	}
	return r.excludeUsageReportCoarseMonths(ctx, tx, scope, generation, filter)
}

func (r *Repository) populateUsageReportUTCMonthDimensions(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) error {
	predicate := reportScopePredicate(scope, generation, filter)
	query := `WITH matched AS (
	 SELECT c.model,c.provider,c.metric_kind,c.currency,c.usage_date,g.period,c.token_count,c.cost_count
 FROM usage_daily_summaries c JOIN usage_report_utc_calendar g ON g.utc_date=c.usage_date
 WHERE ` + predicate.sql + ` AND c.calendar='UTC' AND g.full_month=1
)
INSERT INTO usage_report_utc_month_dimensions(model,provider,period,summarize)
 SELECT m.model,m.provider,m.period,
 CASE WHEN COUNT(DISTINCT CASE WHEN m.metric_kind='cost' THEN NULLIF(m.currency,'') END)<=1 THEN 1 ELSE 0 END
FROM matched m GROUP BY m.model,m.provider,m.period`
	if _, err := tx.ExecContext(ctx, r.db.Rebind(query), predicate.args...); err != nil {
		return fmt.Errorf("populate UTC month summary dimensions: %w", err)
	}
	return nil
}

func (r *Repository) excludeUsageReportCurrencyUnsafeMonths(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) error {
	predicate := reportScopePredicate(scope, generation, filter)
	unsafeDimensions := `WITH matched AS (
	 SELECT c.model,c.provider,c.metric_kind,c.currency,c.usage_date,g.period,c.token_count,c.cost_count,c.rate_missing
 FROM usage_daily_summaries c JOIN usage_report_utc_calendar g ON g.utc_date=c.usage_date
 WHERE ` + predicate.sql + ` AND c.calendar='UTC' AND g.full_month=1
), edge_matched AS (
	 SELECT c.model,c.provider,substr(local.period,1,7)||'-01' AS period,c.metric_kind,c.currency,c.usage_date,c.rate_tokens
	 FROM usage_projection_contributions c
	 JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
	 JOIN usage_report_utc_calendar u ON u.utc_date=c.usage_date AND u.full_month=0
	 JOIN usage_report_calendar local ON c.coverage_start>=local.start_at AND c.coverage_start<local.end_at
	 WHERE c.calendar='UTC' AND c.coverage_kind='point' AND c.coverage_start>=u.start_at AND c.coverage_start<u.end_at AND ` + predicate.sql + `
), currencies AS (
	 SELECT model,provider,period,currency FROM matched WHERE metric_kind='cost'
	 UNION ALL
	 SELECT model,provider,period,currency FROM edge_matched WHERE metric_kind='cost'
), monthly AS (
	 SELECT model,provider,period,COUNT(DISTINCT NULLIF(currency,'')) AS currencies
	 FROM currencies GROUP BY model,provider,period
), daily AS (
 SELECT model,provider,period,usage_date,
 SUM(CASE WHEN metric_kind='tokens' THEN token_count ELSE 0 END) AS tokens,
 SUM(CASE WHEN metric_kind='cost' THEN cost_count ELSE 0 END) AS costs,
 SUM(CASE WHEN metric_kind='cost' AND NULLIF(currency,'') IS NULL THEN cost_count ELSE 0 END) AS unknown_costs,
	 SUM(CASE WHEN metric_kind='cost' THEN rate_missing ELSE 0 END) AS rate_missing,
 COUNT(DISTINCT CASE WHEN metric_kind='cost' THEN NULLIF(currency,'') END) AS currencies
 FROM matched GROUP BY model,provider,period,usage_date
	), edge_daily AS (
	 SELECT model,provider,period,usage_date,
	 SUM(CASE WHEN metric_kind='tokens' THEN 1 ELSE 0 END) AS tokens,
	 SUM(CASE WHEN metric_kind='cost' THEN 1 ELSE 0 END) AS costs,
	 SUM(CASE WHEN metric_kind='cost' AND NULLIF(currency,'') IS NULL THEN 1 ELSE 0 END) AS unknown_costs,
	 SUM(CASE WHEN metric_kind='cost' AND rate_tokens IS NULL THEN 1 ELSE 0 END) AS rate_missing,
	 COUNT(DISTINCT CASE WHEN metric_kind='cost' THEN NULLIF(currency,'') END) AS currencies
	 FROM edge_matched GROUP BY model,provider,period,usage_date
)

UPDATE usage_report_utc_month_dimensions SET summarize=0 WHERE EXISTS (
	 SELECT 1 FROM monthly m WHERE m.model=usage_report_utc_month_dimensions.model
	 AND m.provider=usage_report_utc_month_dimensions.provider AND m.period=usage_report_utc_month_dimensions.period
	 AND m.currencies>1) OR EXISTS (
 SELECT 1 FROM daily d JOIN monthly m
 ON m.model=d.model AND m.provider=d.provider AND m.period=d.period
 WHERE d.model=usage_report_utc_month_dimensions.model AND d.provider=usage_report_utc_month_dimensions.provider
 AND d.period=usage_report_utc_month_dimensions.period AND m.currencies=1 AND d.tokens>0
		 AND (d.currencies=0 OR d.unknown_costs>0 OR d.tokens>d.costs OR d.rate_missing>0)) OR EXISTS (
	 SELECT 1 FROM edge_daily d JOIN monthly m
	 ON m.model=d.model AND m.provider=d.provider AND m.period=d.period
	 WHERE d.model=usage_report_utc_month_dimensions.model AND d.provider=usage_report_utc_month_dimensions.provider
	 AND d.period=usage_report_utc_month_dimensions.period AND m.currencies=1 AND d.tokens>0
		 AND (d.currencies=0 OR d.unknown_costs>0 OR d.tokens>d.costs OR d.rate_missing>0))`
	unsafeArgs := append(append([]any{}, predicate.args...), predicate.args...)
	if _, err := tx.ExecContext(ctx, r.db.Rebind(unsafeDimensions), unsafeArgs...); err != nil {
		return fmt.Errorf("exclude UTC months with currency-unsafe days: %w", err)
	}
	return nil
}

func (r *Repository) excludeUsageReportCoarseMonths(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) error {
	predicate := reportScopePredicate(scope, generation, filter)
	coarseDimensions := `WITH coarse_months AS (
	 SELECT DISTINCT c.model,c.provider,substr(local.period,1,7) AS month
	 FROM usage_projection_contributions c
	 JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
	 JOIN usage_report_calendar local ON c.coverage_start<local.end_at AND c.coverage_end>local.start_at
	 WHERE c.coverage_kind IN ('calendar','interval') AND ` + predicate.sql + `
	)
	UPDATE usage_report_utc_month_dimensions SET summarize=0 WHERE EXISTS (
	 SELECT 1 FROM coarse_months m WHERE m.model=usage_report_utc_month_dimensions.model
	 AND m.provider=usage_report_utc_month_dimensions.provider
	 AND m.month=substr(usage_report_utc_month_dimensions.period,1,7))`
	_, err := tx.ExecContext(ctx, r.db.Rebind(coarseDimensions), predicate.args...)
	if err != nil {
		return fmt.Errorf("exclude months with requested-calendar coarse contributions: %w", err)
	}
	return nil
}

func usageAllTimeSummaryCandidate(filter models.SessionUsageFilter) bool {
	return filter.Start == nil && filter.End == nil && !isDatedUsageGroup(filter.GroupBy) &&
		filter.GroupBy != usageWorkFallback && filter.GroupBy != usageSubjectSession &&
		len(filter.TaskIDs) == 0 && len(filter.SessionIDs) == 0 &&
		filter.Provider != "" && filter.Source == ""
}

func (r *Repository) usageReportAllTimeSummarySafe(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter) (bool, error) {
	predicate := reportScopePredicate(scope, generation, filter)
	var hasMixedLifetime bool
	query := `SELECT EXISTS (
 SELECT 1 FROM usage_projection_history h
 JOIN usage_projection_contributions c ON c.scope_id=h.scope_id AND c.generation=h.generation AND c.work_key=h.work_key
 JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
 WHERE h.scope_id=? AND h.generation=? AND h.lifetime_count>0 AND h.dated_count>0
 AND c.coverage_kind<>'lifetime' AND ` + predicate.sql + `)
 OR EXISTS (
 SELECT 1 FROM usage_projection_contributions c JOIN usage_projection_applied a
 ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
 WHERE ` + predicate.sql + ` AND c.coverage_kind IN ('calendar','interval'))`
	args := append([]any{scope, generation}, predicate.args...)
	args = append(args, predicate.args...)
	if err := tx.GetContext(ctx, &hasMixedLifetime, r.db.Rebind(query), args...); err != nil {
		return false, err
	}
	if hasMixedLifetime {
		return false, nil
	}
	var unsafeSummary bool
	query = `SELECT EXISTS (
 SELECT 1 FROM usage_daily_summaries c WHERE ` + predicate.sql + ` AND c.calendar='UTC' AND c.source<>'native')
 OR EXISTS (
 SELECT 1 FROM usage_daily_summaries c WHERE ` + predicate.sql + ` AND c.calendar='UTC'
 GROUP BY c.model,c.provider HAVING COUNT(DISTINCT CASE WHEN c.metric_kind='cost' THEN c.currency END)>1)
 OR EXISTS (
 SELECT 1 FROM usage_daily_summaries c WHERE ` + predicate.sql + ` AND c.calendar='UTC'
 GROUP BY c.model,c.provider,c.usage_date,c.actor_id,c.actor_status,c.actor_kind,c.source,c.agent_client,c.semantics
 HAVING SUM(CASE WHEN c.metric_kind='tokens' THEN c.token_count ELSE 0 END)<>
        SUM(CASE WHEN c.metric_kind='cost' THEN c.cost_count ELSE 0 END))`
	args = append(append(append([]any{}, predicate.args...), predicate.args...), predicate.args...)
	if err := tx.GetContext(ctx, &unsafeSummary, r.db.Rebind(query), args...); err != nil {
		return false, err
	}
	return !unsafeSummary, nil
}

func reportAllTimeSummarySource(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	predicate := reportScopePredicate(scope, generation, filter)
	periods := `WITH first_period_groups AS MATERIALIZED (
	 SELECT c.scope_id,c.generation,c.model,c.provider FROM usage_daily_summaries c WHERE ` + predicate.sql + ` AND c.calendar='UTC' GROUP BY c.scope_id,c.generation,c.model,c.provider
	), first_periods AS MATERIALIZED (
	 SELECT d.model,d.provider,` + reportAllTimeFirstPointPeriod("d") + ` AS period FROM first_period_groups d
	) SELECT model,provider,period FROM first_periods`
	point := `SELECT '' AS subject_id,'' AS task_id,'' AS task_incarnation_id,'' AS session_id,'' AS session_incarnation_id,'' AS work_key,'' AS segment_key,'' AS bundle_key,'summary' AS coverage_kind,first_period.period,? AS calendar,'requested' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", true, "") + ` FROM usage_daily_summaries c JOIN (` + periods + `) first_period ON first_period.model=c.model AND first_period.provider=c.provider CROSS JOIN (SELECT 0 AS month_partial) g WHERE ` + predicate.sql + ` AND c.calendar='UTC'`
	pointArgs := []any{filter.Timezone}
	pointArgs = append(pointArgs, predicate.args...)
	pointArgs = append(pointArgs, predicate.args...)
	lifetime := `SELECT '' AS subject_id,'' AS task_id,'' AS task_incarnation_id,'' AS session_id,'' AS session_incarnation_id,'' AS work_key,'' AS segment_key,'' AS bundle_key,'lifetime' AS coverage_kind,'' AS period,'' AS calendar,'unknown' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", true, "") + ` FROM usage_lifetime_summaries c CROSS JOIN (SELECT 0 AS month_partial) g WHERE ` + predicate.sql
	return usageReportQuery{sql: point + ` UNION ALL ` + lifetime, args: append(pointArgs, predicate.args...)}
}

func reportAllTimeFirstPointPeriod(alias string) string {
	return `(SELECT local_day.period FROM usage_projection_contributions p
 JOIN usage_projection_applied pa ON pa.scope_id=p.scope_id AND pa.generation=p.generation AND pa.work_key=p.work_key AND pa.build_id=p.build_id
 JOIN usage_report_calendar local_day ON local_day.start_at=(SELECT start_at FROM usage_report_calendar WHERE start_at<=p.coverage_start ORDER BY start_at DESC LIMIT 1) AND p.coverage_start<local_day.end_at
 WHERE p.scope_id=` + alias + `.scope_id AND p.generation=` + alias + `.generation AND p.model=` + alias + `.model AND p.provider=` + alias + `.provider
 AND p.calendar='UTC' AND p.coverage_kind='point' AND p.coverage_start>=(SELECT MIN(start_at) FROM usage_report_calendar) AND p.coverage_start<(SELECT MAX(end_at) FROM usage_report_calendar)
 ORDER BY p.coverage_start LIMIT 1)`
}

func reportUTCMonthSummarySource(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	predicate := reportScopePredicate(scope, generation, filter)
	factPredicate := reportFactScopePredicate(scope, generation, filter)
	summary := `SELECT '' AS subject_id,'' AS task_id,'' AS task_incarnation_id,'' AS session_id,'' AS session_incarnation_id,'' AS work_key,'' AS segment_key,'' AS bundle_key,'summary' AS coverage_kind,g.period,? AS calendar,'requested' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", true, "") + ` FROM usage_daily_summaries c JOIN usage_report_utc_calendar g ON g.utc_date=c.usage_date AND g.full_month=1 JOIN usage_report_utc_month_dimensions d ON d.model=c.model AND d.provider=c.provider AND d.period=g.period AND d.summarize=1 WHERE ` + predicate.sql + ` AND c.calendar='UTC'`
	summaryArgs := append([]any{filter.Timezone}, predicate.args...)
	selectSQL := `SELECT ` + reportSubjectColumns() + `,g.period,? AS calendar,'requested' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", false, "g.month_partial=1")
	joinSuffix := strings.TrimPrefix(reportFactJoin(), ` FROM usage_projection_contributions c`) + ` JOIN usage_report_calendar g ON g.start_at=(SELECT start_at FROM usage_report_calendar WHERE start_at<=c.coverage_start ORDER BY start_at DESC LIMIT 1) AND c.coverage_start<g.end_at`
	edges := reportUTCMonthEdgeSource(selectSQL, joinSuffix, factPredicate, filter)
	fallbackDays := `(WITH fallback_days AS MATERIALIZED (
 SELECT d.period,d.start_at,d.end_at,x.model,x.provider FROM usage_report_utc_calendar d
 JOIN usage_report_utc_month_dimensions x ON x.period=d.period AND x.summarize=0 WHERE d.full_month=1
	 ) SELECT * FROM fallback_days)`
	unsafe := usageReportQuery{sql: selectSQL + ` FROM ` + fallbackDays + ` d CROSS JOIN usage_projection_contributions c` + joinSuffix + ` WHERE c.model=d.model AND c.provider=d.provider AND c.calendar='UTC' AND c.coverage_kind='point' AND c.coverage_start>=d.start_at AND c.coverage_start<d.end_at AND ` + factPredicate.sql,
		args: append([]any{filter.Timezone}, factPredicate.args...)}
	coarse := reportCoarseSource(scope, generation, filter)
	query := summary + ` UNION ALL ` + edges.sql + ` UNION ALL ` + unsafe.sql
	args := append(append([]any{}, summaryArgs...), edges.args...)
	args = append(args, unsafe.args...)
	return usageReportQuery{sql: query + ` UNION ALL ` + coarse.sql, args: append(args, coarse.args...)}
}

func reportUTCMonthEdgeSource(selectSQL, joinSuffix string, predicate usageReportQuery, filter models.SessionUsageFilter) usageReportQuery {
	// Keep one indexed point probe per UTC edge day. A long monthly range has
	// dozens of disjoint UTC edges; expressing them as an OR over coverage_start
	// makes SQLite scan every point in the scope instead of seeking each day.
	edgeDays := `(WITH edge_days AS MATERIALIZED (
 SELECT start_at,end_at FROM usage_report_utc_calendar WHERE full_month=0
) SELECT * FROM edge_days)`
	return usageReportQuery{
		sql:  selectSQL + ` FROM ` + edgeDays + ` d CROSS JOIN usage_projection_contributions c` + joinSuffix + ` WHERE c.calendar='UTC' AND c.coverage_kind='point' AND c.coverage_start>=d.start_at AND c.coverage_start<d.end_at AND ` + predicate.sql,
		args: append([]any{filter.Timezone}, predicate.args...),
	}
}
