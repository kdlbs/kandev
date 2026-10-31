package sqlite

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/analytics/models"
)

const reportPeriodColumn = "period"
const reportProviderColumn = "provider"
const reportCostColumn = "cost_subcents"
const reportTotalTokensColumn = "total_tokens"
const reportRateSQL = `(CASE WHEN rate_tokens>0 AND rate_missing=0 AND cost_missing=0 AND partial_count=0 AND cost_basis NOT IN ('unknown','mixed') AND currency NOT IN ('','mixed') THEN CAST(cost_subcents AS DOUBLE PRECISION)*100/rate_tokens END)`

type usageReportAggregate struct {
	models.UsageDailySummary
	DatedCount         int64  `db:"dated_count"`
	UndatedCount       int64  `db:"undated_count"`
	Entity             string `db:"entity"`
	Period             string `db:"period"`
	SubjectID          string `db:"subject_id"`
	TaskID             string `db:"task_id"`
	TaskIncarnation    string `db:"task_incarnation_id"`
	SessionID          string `db:"session_id"`
	SessionIncarnation string `db:"session_incarnation_id"`
	CalendarBasis      string `db:"calendar_basis"`
	TokenPartial       int64  `db:"token_partial"`
	CostPartial        int64  `db:"cost_partial"`
	TokenActorID       string `db:"token_actor_id"`
	TokenActorStatus   string `db:"token_actor_status"`
	TokenActorKind     string `db:"token_actor_kind"`
	TokenSource        string `db:"token_source"`
	CostActorID        string `db:"cost_actor_id"`
	CostActorStatus    string `db:"cost_actor_status"`
	CostActorKind      string `db:"cost_actor_kind"`
	CostSource         string `db:"cost_source"`
}

func usageReportEntity(group string) string {
	switch group {
	case usageWorkFallback:
		return `task_id||':'||task_incarnation_id`
	case usageSubjectSession:
		return `subject_id`
	case usageGroupDay, usageGroupDaily:
		return reportPeriodColumn
	case usageGroupMonth, usageGroupMonthly:
		return `substr(period,1,7)`
	default:
		return usageGroupModel
	}
}

func usageReportActorSQL(metric string) []string {
	condition := `metric_kind='` + metric + `'`
	actor := `CASE WHEN ` + condition + ` THEN actor_id||':'||actor_status||':'||actor_kind END`
	status := `CASE WHEN COUNT(DISTINCT ` + actor + `)>1 THEN 'mixed' ELSE COALESCE(MIN(CASE WHEN ` + condition + ` THEN actor_status END),'unknown') END AS ` + metric + `_actor_status`
	id := `CASE WHEN COUNT(DISTINCT ` + actor + `)=1 THEN COALESCE(MIN(CASE WHEN ` + condition + ` THEN actor_id END),'') ELSE '' END AS ` + metric + `_actor_id`
	kind := `CASE WHEN COUNT(DISTINCT ` + actor + `)=1 THEN COALESCE(MIN(CASE WHEN ` + condition + ` THEN actor_kind END),'unknown') ELSE 'unknown' END AS ` + metric + `_actor_kind`
	source := `CASE WHEN COUNT(DISTINCT CASE WHEN ` + condition + ` THEN source END)>1 THEN 'mixed' ELSE COALESCE(MIN(CASE WHEN ` + condition + ` THEN source END),'') END AS ` + metric + `_source`
	return []string{id, status, kind, source}
}

func usageReportAggregateSQL(publicationPending bool) string {
	var columns []string
	for i, name := range usageProjectionAmountColumns {
		metric := "token_count"
		if i >= 8 {
			metric = "cost_count"
		}
		expr := `CASE WHEN SUM(` + usageProjectionMissingColumns[i] + `)=0 AND SUM(` + metric + `)>0 THEN CAST(SUM(` + name + `) AS BIGINT) END`
		if name == reportCostColumn {
			expr = `CASE WHEN COUNT(DISTINCT CASE WHEN metric_kind='cost' AND currency<>'' THEN currency END)<=1 THEN ` + expr + ` END`
		}
		switch name {
		case "turns":
			expr = nativeUsageTurnsAggregateSQL()
		case "reasoning_tokens":
			expr = nativeUsageReasoningAggregateSQL(publicationPending)
		}
		columns = append(columns, expr+` AS `+name)
	}
	for _, name := range usageProjectionMissingColumns {
		expr := `SUM(` + name + `)`
		switch name {
		case "turns_missing":
			expr = nativeUsageTurnsMissingAggregateSQL()
		case "reasoning_missing":
			expr = nativeUsageReasoningMissingAggregateSQL(publicationPending)
		}
		columns = append(columns, expr+` AS `+name)
	}
	for _, name := range []string{"token_count", "cost_count", "partial_count", "estimated_count", "stale_count"} {
		columns = append(columns, `SUM(`+name+`) AS `+name)
	}
	for _, name := range []string{usageGroupModel, reportProviderColumn, "agent_client", "calendar", "calendar_basis"} {
		columns = append(columns, `CASE WHEN MIN(`+name+`)=MAX(`+name+`) THEN MIN(`+name+`) ELSE 'mixed' END AS `+name)
	}
	columns = append(columns, `CASE WHEN COUNT(DISTINCT CASE WHEN metric_kind='cost' AND currency<>'' THEN currency END)>1 THEN 'mixed' ELSE COALESCE(MIN(NULLIF(currency,'')),'') END AS currency`,
		`CASE WHEN MIN(CASE WHEN metric_kind='cost' THEN cost_basis END)=MAX(CASE WHEN metric_kind='cost' THEN cost_basis END) THEN MIN(CASE WHEN metric_kind='cost' THEN cost_basis END) WHEN COUNT(DISTINCT CASE WHEN metric_kind='cost' THEN cost_basis END)>1 THEN 'mixed' ELSE 'unknown' END AS cost_basis`,
		`SUM(CASE WHEN metric_kind='tokens' THEN partial_count ELSE 0 END) AS token_partial`,
		`SUM(CASE WHEN metric_kind='cost' THEN partial_count ELSE 0 END) AS cost_partial`)
	columns = append(columns, `SUM(CASE WHEN coverage_kind='lifetime' THEN 1 ELSE 0 END) AS undated_count`, `SUM(CASE WHEN coverage_kind<>'lifetime' THEN 1 ELSE 0 END) AS dated_count`)
	columns = append(columns, usageReportActorSQL("token")...)
	columns = append(columns, usageReportActorSQL(projectionMetricCost)...)
	// metric kinds are plural in storage and singular in the HTTP source names.
	return strings.ReplaceAll(strings.Join(columns, ","), `metric_kind='token'`, `metric_kind='tokens'`)
}

func usageReportGrouped(source usageReportQuery, group string, publicationPending bool) usageReportQuery {
	entity := usageReportEntity(group)
	// Tokens carry no currency dimension. Attach them to a sole cost currency;
	// with several currencies retain tokens once and each cost currency separately.
	sql := `WITH base AS (` + source.sql + `), identified AS (SELECT *,` + entity + ` AS entity FROM base), currencies AS (
 SELECT entity,model,provider,period,calendar,COUNT(DISTINCT NULLIF(currency,'')) AS n,MIN(NULLIF(currency,'')) AS only_currency FROM identified GROUP BY entity,model,provider,period,calendar), normalized AS (
 SELECT b.*,CASE WHEN b.metric_kind='tokens' AND d.n=1 THEN d.only_currency ELSE b.currency END AS group_currency FROM identified b JOIN currencies d ON d.entity=b.entity AND d.model=b.model AND d.provider=b.provider AND d.period=b.period AND d.calendar=b.calendar)
 SELECT entity,MIN(subject_id) AS subject_id,MIN(task_id) AS task_id,MIN(task_incarnation_id) AS task_incarnation_id,MIN(session_id) AS session_id,MIN(session_incarnation_id) AS session_incarnation_id,`
	period := `MIN(period)`
	if group == usageGroupMonth || group == usageGroupMonthly {
		period = `substr(MIN(period),1,7)`
	}
	sql += period + ` AS period,` + usageReportAggregateSQL(publicationPending) + ` FROM normalized GROUP BY entity,model,provider,calendar,calendar_basis,group_currency`
	if isDatedUsageGroup(group) {
		sql += `, ` + entity
	}
	return usageReportQuery{sql: sql, args: source.args}
}

func usageReportOrder(filter models.SessionUsageFilter) string {
	name := filter.SortBy
	if name == "" {
		name = projectionMetricCost
		if isDatedUsageGroup(filter.GroupBy) {
			name = reportPeriodColumn
		}
	}
	aliases := map[string]string{projectionMetricCost: reportCostColumn, projectionMetricTokens: reportTotalTokensColumn, reportTotalTokensColumn: reportTotalTokensColumn, "input": "input_tokens", "output": "output_tokens", "cache_read": "cache_read_tokens", "cache_write": "cache_write_tokens", "reasoning": "reasoning_tokens", "turns": "turns", usageGroupModel: usageGroupModel, reportProviderColumn: reportProviderColumn, reportPeriodColumn: reportPeriodColumn, usageWorkFallback: "task_id", usageSubjectSession: "session_id", "effective_rate": reportRateSQL, "rate": reportRateSQL, "effective_cost_per_million": reportRateSQL}
	column, ok := aliases[name]
	if !ok {
		column = reportCostColumn
	}
	direction := "DESC"
	if strings.EqualFold(filter.SortDirection, "asc") || filter.SortDirection == "" && name == reportPeriodColumn {
		direction = "ASC"
	}
	return fmt.Sprintf(` ORDER BY %s IS NULL ASC,%s %s,entity,model,provider,calendar,currency,period`, column, column, direction)
}

// A total across several currencies has no cost sum. Limit the candidate
// cost aggregate to one currency so SQLite never overflows a sum that the
// report cannot use, while retaining every currency in its own result row.
func usageReportTotalsSQL(publicationPending bool) string {
	return strings.ReplaceAll(usageReportAggregateSQL(publicationPending), `SUM(cost_subcents)`, `SUM(CASE WHEN currency=(SELECT MIN(currency) FROM base WHERE metric_kind='cost' AND currency<>'') THEN cost_subcents END)`)
}
