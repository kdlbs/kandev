package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/analytics/models"
)

// Source-owned pages expose accepted observations without canonical selection.
// Their scope is retained ownership, independent of live task/session joins.
func (r *Repository) listSourceUsagePage(parent context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := r.readSourceUsagePage(ctx, filter)
	return rows, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) readSourceUsagePage(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.authorizedUsageScopeTx(ctx, tx, filter.WorkspaceID)
	if err != nil {
		return nil, err
	}
	query, args := sourceUsagePageQuery(scope, filter)
	cursor, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	records := make([]models.SessionUsageMeasurement, 0)
	for cursor.Next() {
		row, scanErr := scanUsageRow(cursor)
		if scanErr != nil {
			_ = cursor.Close()
			return nil, scanErr
		}
		records = append(records, row.stored().measurement())
	}
	err = cursor.Err()
	closeErr := cursor.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return records, tx.Commit()
}

func sourceUsagePageQuery(scope string, filter models.SessionUsageFilter) (string, []any) {
	var arms []string
	var args []any
	columns := strings.ReplaceAll(usageSelectColumns, "COALESCE(session_id, '')", "COALESCE(session_id, '') AS session_id")
	for _, name := range []string{"observed_at", "collected_at", "coverage_start", "coverage_end"} {
		columns = strings.ReplaceAll(columns, "CAST("+name+" AS TEXT)", "CAST("+name+" AS TEXT) AS "+name)
	}
	for i, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		base := sourceUsagePredicate(scope, filter)
		for _, timeFilter := range sourceUsageTimePredicates(filter) {
			arms = append(arms, `SELECT `+columns+fmt.Sprintf(`,%d AS storage_kind FROM `, i)+table+` c WHERE `+base.sql+` AND `+timeFilter.sql)
			args = append(args, base.args...)
			args = append(args, timeFilter.args...)
		}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = maxSessionUsageBatch
	}
	limit = min(limit, maxSessionUsageBatch+1)
	query := `WITH observations AS (` + strings.Join(arms, ` UNION ALL `) + `) SELECT ` + usageSelectColumns + ` FROM observations` + sourceUsageOrder(filter) + ` LIMIT ? OFFSET ?`
	return query, append(args, limit, max(filter.Offset, 0))
}

func sourceUsagePredicate(scope string, filter models.SessionUsageFilter) usageReportQuery {
	q := usageReportQuery{sql: `c.scope_id=? AND c.retired=0 AND c.source<>'native'`, args: []any{scope}}
	for _, dimension := range []struct{ name, value string }{{"source", filter.Source}, {"model", filter.Model}, {reportProviderColumn, filter.Provider}} {
		if dimension.value == "" {
			continue
		}
		if dimension.name == reportProviderColumn && dimension.value == unknownProviderFilter {
			q.sql += ` AND c.provider=''`
			continue
		}
		q.sql += ` AND c.` + dimension.name + `=?`
		q.args = append(q.args, dimension.value)
	}
	for _, dimension := range []struct {
		name   string
		values []string
	}{{"task_id", filter.TaskIDs}, {"session_id", filter.SessionIDs}} {
		if len(dimension.values) > 0 {
			q.sql += ` AND c.` + dimension.name + ` IN (` + placeholders(len(dimension.values)) + `)`
			q.args = append(q.args, stringArgs(dimension.values)...)
		}
	}
	return q
}

func sourceUsageTimePredicates(filter models.SessionUsageFilter) []usageReportQuery {
	if filter.Start == nil && filter.End == nil {
		predicate := `1=1`
		if isDatedUsageGroup(filter.GroupBy) {
			predicate = `c.coverage_start IS NOT NULL`
		}
		return []usageReportQuery{{sql: predicate}}
	}
	point := `(c.coverage_start=c.coverage_end OR c.selection_coverage LIKE 'point:%' OR c.selection_coverage LIKE 'event:%:point:%')`
	points := usageReportQuery{sql: point}
	intervals := usageReportQuery{sql: `c.coverage_start IS NOT NULL AND NOT ` + point}
	if filter.Start != nil {
		points.sql += ` AND c.coverage_start>=?`
		points.args = append(points.args, *filter.Start)
		intervals.sql += ` AND c.coverage_end>?`
		intervals.args = append(intervals.args, *filter.Start)
	}
	if filter.End != nil {
		points.sql += usageReportBeforeEnd
		points.args = append(points.args, *filter.End)
		intervals.sql += usageReportBeforeEnd
		intervals.args = append(intervals.args, *filter.End)
	}
	result := []usageReportQuery{points, intervals}
	if filter.IncludeUndated && !isDatedUsageGroup(filter.GroupBy) {
		result = append(result, usageReportQuery{sql: `c.coverage_start IS NULL`})
	}
	return result
}

func sourceUsageOrder(filter models.SessionUsageFilter) string {
	column := `id`
	switch filter.SortBy {
	case projectionMetricTokens, reportTotalTokensColumn:
		column = reportTotalTokensColumn
	case projectionMetricCost, reportCostColumn:
		column = reportCostColumn
	case "model":
		column = "model"
	case reportProviderColumn:
		column = reportProviderColumn
	case "period", "date":
		column = "source_date"
	}
	direction := "ASC"
	if strings.EqualFold(filter.SortDirection, "desc") {
		direction = "DESC"
	}
	return ` ORDER BY ` + column + ` IS NULL,` + column + ` ` + direction + `,source_record_id,model,provider,storage_kind,id`
}
