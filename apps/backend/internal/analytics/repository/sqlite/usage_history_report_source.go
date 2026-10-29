package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

type usageReportQuery struct {
	sql  string
	args []any
}

const usageReportBeforeEnd = ` AND c.coverage_start<?`

// The grid is transaction-local. Bounded inserts avoid a driver parameter limit
// without creating durable copies for arbitrary browser calendars.
func (r *Repository) usageReportGrid(ctx context.Context, tx *sqlx.Tx, filter models.SessionUsageFilter, metadata usageProjectionSnapshot) error {
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE usage_report_calendar (period TEXT PRIMARY KEY, start_at TIMESTAMP NOT NULL UNIQUE, end_at TIMESTAMP NOT NULL, full_day INTEGER NOT NULL, month_partial INTEGER NOT NULL)`); err != nil {
		return err
	}
	location, err := timeLocation(filter.Timezone)
	if err != nil {
		return err
	}
	start, end := metadata.Earliest, metadata.Latest
	if filter.Start != nil {
		start = filter.Start
	}
	if filter.End != nil {
		end = filter.End
	}
	if start == nil || end == nil {
		return nil
	}
	local := start.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for day.Before(*end) {
		var values []string
		var args []any
		for len(values) < 64 && day.Before(*end) {
			next := day.AddDate(0, 0, 1)
			from, until := day.UTC(), next.UTC()
			if from.Before(*start) {
				from = *start
			}
			if until.After(*end) {
				until = *end
			}
			values = append(values, `(?, ?, ?, ?, ?)`)
			args = append(args, day.Format(time.DateOnly), from, until, boolInt(from.Equal(day) && until.Equal(next)), boolInt(usageReportClippedMonth(day, filter)))
			day = next
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_report_calendar VALUES `+strings.Join(values, ",")), args...); err != nil {
			return err
		}
	}
	return nil
}

func usageReportDimensions(alias string) string {
	columns := []string{"metric_kind", "actor_id", "actor_status", "actor_kind", "source", usageGroupModel, reportProviderColumn, "agent_client", "semantics", "currency"}
	for i := range columns {
		columns[i] = alias + "." + columns[i]
	}
	return strings.Join(columns, ",")
}

func usageReportAmountsSQL(alias string, summary bool, partial string) string {
	var columns []string
	for _, name := range usageProjectionAmountColumns {
		columns = append(columns, alias+"."+name)
	}
	if summary {
		for _, name := range usageProjectionMissingColumns {
			columns = append(columns, alias+"."+name)
		}
		for _, name := range []string{"token_count", "cost_count", "partial_count", "estimated_count", "stale_count", "cost_basis"} {
			if name == "partial_count" {
				columns = append(columns, `c.partial_count+CASE WHEN g.month_partial=1 THEN c.token_count+c.cost_count ELSE 0 END AS partial_count`)
			} else {
				columns = append(columns, alias+"."+name)
			}
		}
	} else {
		for i, name := range usageProjectionAmountColumns {
			metric := projectionMetricTokens
			if i >= 8 {
				metric = projectionMetricCost
			}
			columns = append(columns, `CASE WHEN c.metric_kind = '`+metric+`' AND c.`+name+` IS NULL THEN 1 ELSE 0 END AS `+usageProjectionMissingColumns[i])
		}
		columns = append(columns, `CASE WHEN c.metric_kind = 'tokens' THEN 1 ELSE 0 END AS token_count`, `CASE WHEN c.metric_kind = 'cost' THEN 1 ELSE 0 END AS cost_count`, `CASE WHEN c.partial <> 0 OR (`+partial+`) THEN 1 ELSE 0 END AS partial_count`, `c.estimated AS estimated_count`, `c.stale AS stale_count`, `c.cost_basis`)
	}
	return strings.Join(columns, ",")
}

func reportFactJoin() string {
	return ` FROM usage_projection_contributions c JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id
 JOIN usage_canonical_facts f ON f.scope_id=c.scope_id AND f.generation=c.generation AND f.build_id=c.build_id AND f.fact_key=c.fact_key
 LEFT JOIN usage_subjects u ON u.scope_id=f.scope_id AND u.subject_id=f.subject_id `
}

func reportSubjectColumns() string {
	return `COALESCE(u.subject_id,'') AS subject_id,COALESCE(u.task_id,'') AS task_id,COALESCE(u.task_incarnation_id,'') AS task_incarnation_id,COALESCE(u.session_id,'') AS session_id,COALESCE(u.session_incarnation_id,'') AS session_incarnation_id,f.work_key,f.segment_key,f.bundle_key,f.coverage_kind`
}

func reportScopePredicate(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	q := usageReportQuery{sql: ` c.scope_id=? AND c.generation=?`, args: []any{scope, generation}}
	for _, dimension := range []struct{ name, value string }{{reportProviderColumn, filter.Provider}, {usageGroupModel, filter.Model}, {"source", filter.Source}} {
		if dimension.value != "" {
			if dimension.name == reportProviderColumn && dimension.value == unknownProviderFilter {
				q.sql += ` AND c.provider=''`
				continue
			}
			q.sql += ` AND c.` + dimension.name + `=?`
			q.args = append(q.args, dimension.value)
		}
	}
	for _, dimension := range []struct {
		name   string
		values []string
	}{{"task_id", filter.TaskIDs}, {"session_id", filter.SessionIDs}} {
		if len(dimension.values) > 0 {
			q.sql += ` AND u.` + dimension.name + ` IN (` + placeholders(len(dimension.values)) + `)`
			q.args = append(q.args, stringArgs(dimension.values)...)
		}
	}
	return q
}

func reportOverlap(filter models.SessionUsageFilter) usageReportQuery {
	q := usageReportQuery{sql: `1=1`}
	if filter.Start != nil {
		q.sql += ` AND c.coverage_end>?`
		q.args = append(q.args, *filter.Start)
	}
	if filter.End != nil {
		q.sql += usageReportBeforeEnd
		q.args = append(q.args, *filter.End)
	}
	return q
}

func reportFactSource(scope, generation string, filter models.SessionUsageFilter, lifetime bool) usageReportQuery {
	pred := reportScopePredicate(scope, generation, filter)
	selectSQL := `SELECT ` + reportSubjectColumns() + `,g.period,? AS calendar,'requested' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", false, "g.month_partial=1") + reportFactJoin()
	// Grid intervals are disjoint. The indexed predecessor is the only possible
	// match; the end predicate preserves exclusive bounds and empty ranges.
	point := usageReportQuery{sql: selectSQL + ` JOIN usage_report_calendar g ON g.start_at=(SELECT start_at FROM usage_report_calendar WHERE start_at<=c.coverage_start ORDER BY start_at DESC LIMIT 1) AND c.coverage_start<g.end_at WHERE ` + pred.sql + ` AND c.coverage_kind='point' AND c.calendar='UTC'
 AND c.coverage_start>=(SELECT MIN(start_at) FROM usage_report_calendar) AND c.coverage_start<(SELECT MAX(end_at) FROM usage_report_calendar)`, args: append([]any{filter.Timezone}, pred.args...)}
	coarse := reportCoarseSource(scope, generation, filter)
	q := usageReportQuery{sql: point.sql + ` UNION ALL ` + coarse.sql, args: append(point.args, coarse.args...)}
	if lifetime {
		return reportLifetimeAlternatives(q, pred)
	}
	return q
}

func reportCoarseSource(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	pred, overlap := reportScopePredicate(scope, generation, filter), reportOverlap(filter)
	partial := `c.calendar<>?`
	if filter.GroupBy == usageGroupMonth || filter.GroupBy == usageGroupMonthly {
		partial += ` OR EXISTS(SELECT 1 FROM usage_report_calendar g WHERE g.period=c.usage_date AND g.month_partial=1)`
	}
	args := []any{filter.Timezone, filter.Timezone}
	if filter.Start != nil {
		partial += ` OR c.coverage_start<?`
		args = append(args, *filter.Start)
	}
	if filter.End != nil {
		partial += ` OR c.coverage_end>?`
		args = append(args, *filter.End)
	}
	// Both accepted coarse kinds use the kind/end index. A negative kind
	// predicate makes SQLite walk the dated point history before rejecting it.
	query := `SELECT ` + reportSubjectColumns() + `,c.usage_date AS period,c.calendar,CASE WHEN c.calendar=? THEN 'requested' ELSE 'source' END AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", false, partial) + reportFactJoin() + ` WHERE ` + pred.sql + ` AND c.coverage_kind IN ('calendar','interval') AND ` + overlap.sql
	args = append(args, pred.args...)
	args = append(args, overlap.args...)
	return usageReportQuery{sql: query, args: args}
}

func reportLifetimeAlternatives(dated, pred usageReportQuery) usageReportQuery {
	// Canonical lifetime and dated facts can coexist as alternative coverage.
	// Exclude dated work with a published lifetime alternative for all-time totals.
	costAuthority := usageLifetimeCostMaskAuthority("l", "dated.cost_subcents IS NULL")
	dated.sql = `SELECT dated.* FROM (` + dated.sql + `) dated WHERE NOT EXISTS (SELECT 1 FROM usage_canonical_facts l JOIN usage_projection_applied la ON la.scope_id=l.scope_id AND la.generation=l.generation AND la.work_key=l.work_key AND la.build_id=l.build_id WHERE l.work_key=dated.work_key AND (l.bundle_key<>dated.bundle_key OR l.segment_key=dated.segment_key) AND l.scope_id=? AND l.generation=? AND l.coverage_kind='lifetime' AND ((dated.metric_kind='tokens' AND ` + usageLifetimeTokenAuthority("l") + `) OR (dated.metric_kind='cost' AND ` + costAuthority + `)))`
	dated.args = append(dated.args, pred.args[0], pred.args[1])
	query := `SELECT ` + reportSubjectColumns() + `,'' AS period,'' AS calendar,'unknown' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", false, "0=1") + reportFactJoin() + ` WHERE ` + pred.sql + ` AND c.coverage_kind='lifetime'`
	return usageReportQuery{sql: dated.sql + ` UNION ALL ` + query, args: append(dated.args, pred.args...)}
}

func reportSummarySource(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	pred := reportScopePredicate(scope, generation, filter)
	// Summary dimensions deliberately have no subject IDs. Subject breakdowns
	// always take the fact path instead of pretending to allocate day totals.
	query := `SELECT '' AS subject_id,'' AS task_id,'' AS task_incarnation_id,'' AS session_id,'' AS session_incarnation_id,'' AS work_key,'' AS segment_key,'' AS bundle_key,'summary' AS coverage_kind,g.period,c.calendar,'requested' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", true, "") + ` FROM usage_daily_summaries c JOIN usage_report_calendar g ON g.period=c.usage_date AND g.full_day=1 WHERE ` + pred.sql + ` AND c.calendar=?
 AND c.usage_date>=(SELECT MIN(period) FROM usage_report_calendar) AND c.usage_date<=(SELECT MAX(period) FROM usage_report_calendar)`
	args := append(append([]any{}, pred.args...), filter.Timezone)
	coarse := reportCoarseSource(scope, generation, filter)
	coarse.sql += ` AND (c.calendar<>? OR NOT EXISTS (SELECT 1 FROM usage_report_calendar g WHERE g.period=c.usage_date AND g.full_day=1))`
	coarse.args = append(coarse.args, filter.Timezone)
	if !usageReportNeedsPointFallback(filter) {
		return usageReportQuery{sql: query + ` UNION ALL ` + coarse.sql, args: append(args, coarse.args...)}
	}
	points := reportFactSource(scope, generation, filter, false)
	// Point facts already present in an exact-calendar full-day summary are
	// omitted from the fallback. A source calendar may cover only some points.
	pointEnd := strings.Index(points.sql, ` UNION ALL `)
	pointSQL := points.sql[:pointEnd] + ` AND g.full_day=0`
	pointArgsCount := 1 + len(pred.args)
	pointArgs := append([]any{}, points.args[:pointArgsCount]...)
	if filter.Timezone != projectionCalendarUTC {
		pointSQL = points.sql[:pointEnd] + ` AND (g.full_day=0 OR f.calendar<>?)`
		pointArgs = append(pointArgs, filter.Timezone)
	}
	return usageReportQuery{sql: query + ` UNION ALL ` + pointSQL + ` UNION ALL ` + coarse.sql, args: append(append(args, pointArgs...), coarse.args...)}
}

func usageReportNeedsPointFallback(filter models.SessionUsageFilter) bool {
	if filter.Timezone != projectionCalendarUTC || filter.Start == nil || filter.End == nil {
		return true
	}
	for _, boundary := range []*time.Time{filter.Start, filter.End} {
		utc := boundary.UTC()
		if utc.Hour() != 0 || utc.Minute() != 0 || utc.Second() != 0 || utc.Nanosecond() != 0 {
			return true
		}
	}
	return false
}

func usageReportSource(scope, generation string, filter models.SessionUsageFilter, totals bool) usageReportQuery {
	allTime := filter.Start == nil && filter.End == nil
	var source usageReportQuery
	if len(filter.TaskIDs) > 0 || len(filter.SessionIDs) > 0 || filter.GroupBy == usageWorkFallback || filter.GroupBy == usageSubjectSession || (allTime && (totals || !isDatedUsageGroup(filter.GroupBy))) {
		source = reportFactSource(scope, generation, filter, allTime && (totals || !isDatedUsageGroup(filter.GroupBy)))
	} else {
		source = reportSummarySource(scope, generation, filter)
	}
	if !allTime && filter.IncludeUndated && !isDatedUsageGroup(filter.GroupBy) {
		undated := reportUndatedRangeSource(scope, generation, filter)
		source.sql += ` UNION ALL ` + undated.sql
		source.args = append(source.args, undated.args...)
	}
	return source
}

// An undated source with dated coverage is an alternative, not another amount
// to add to a selected range. Independent lifetime-only work can be disclosed
// explicitly, without assigning its collection time to the chart.
func reportUndatedRangeSource(scope, generation string, filter models.SessionUsageFilter) usageReportQuery {
	pred := reportScopePredicate(scope, generation, filter)
	query := `SELECT ` + reportSubjectColumns() + `,'' AS period,'' AS calendar,'unknown' AS calendar_basis,` + usageReportDimensions("c") + `,` + usageReportAmountsSQL("c", false, "1=1") + reportFactJoin() + ` WHERE ` + pred.sql + ` AND c.coverage_kind='lifetime' AND NOT EXISTS (
 SELECT 1 FROM usage_canonical_facts d JOIN usage_projection_applied da ON da.scope_id=d.scope_id AND da.generation=d.generation AND da.work_key=d.work_key AND da.build_id=d.build_id
 WHERE d.scope_id=f.scope_id AND d.generation=f.generation AND d.work_key=f.work_key AND (d.bundle_key<>f.bundle_key OR d.segment_key=f.segment_key) AND d.coverage_kind<>'lifetime' AND ((c.metric_kind='tokens' AND d.token_source<>'') OR (c.metric_kind='cost' AND d.cost_source<>'')))`
	return usageReportQuery{sql: query, args: pred.args}
}

func normalizeUsageReportFilter(filter models.SessionUsageFilter) (models.SessionUsageFilter, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return filter, fmt.Errorf("workspace id is required")
	}
	if filter.Timezone == "" {
		filter.Timezone = projectionCalendarUTC
	}
	if _, err := timeLocation(filter.Timezone); err != nil {
		return filter, err
	}
	if filter.GroupBy == "" {
		filter.GroupBy = usageGroupModel
	}
	switch filter.GroupBy {
	case usageGroupModel, usageWorkFallback, usageSubjectSession, usageGroupDay, usageGroupDaily, usageGroupMonth, usageGroupMonthly:
	default:
		return filter, fmt.Errorf("invalid usage grouping")
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	filter.Limit = min(filter.Limit, 1000)
	filter.Offset = max(filter.Offset, 0)
	return filter, nil
}

func usageReportClippedMonth(day time.Time, filter models.SessionUsageFilter) bool {
	if filter.GroupBy != usageGroupMonth && filter.GroupBy != usageGroupMonthly {
		return false
	}
	start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	return filter.Start != nil && filter.Start.After(start) || filter.End != nil && filter.End.Before(start.AddDate(0, 1, 0))
}
