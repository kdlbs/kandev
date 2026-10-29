package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) listRetainedUsageReport(parent context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	filter, err := normalizeUsageReportFilter(filter)
	if err != nil {
		return nil, err
	}
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return nil, err
	}
	defer release()
	report, err := r.readRetainedUsageReport(ctx, filter)
	return report, normalizeUsageReportError(normalizeAnalyticsOperationError(parent, ctx, err))
}

func (r *Repository) readRetainedUsageReport(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.authorizedUsageScopeTx(ctx, tx, filter.WorkspaceID)
	if err != nil {
		return nil, err
	}
	metadata, err := r.usageReportingSnapshotTx(ctx, tx, scope)
	if err != nil {
		return nil, err
	}
	if filter.ProjectionGeneration != "" && filter.ProjectionGeneration != metadata.Generation {
		return nil, analytics.ErrUsageProjectionChanged
	}
	report := &models.TokenUsageReport{RangeStart: filter.Start, RangeEnd: filter.End, Rows: []models.TokenUsageRow{}, Providers: []string{}, ProjectionStatus: metadata.Status, ProjectionGeneration: metadata.Generation, ProjectionPending: metadata.Pending, HasHistory: metadata.HasHistory, HasNativeHistory: metadata.HasNative, DatedCoverage: metadata.HasDated, UndatedCoverage: metadata.HasUndated, LastUpdated: metadata.LastUpdated}
	if err := r.usageReportGap(ctx, tx, filter.WorkspaceID, &report.HistoryGap); err != nil {
		return nil, err
	}
	if metadata.Generation == "" {
		return report, tx.Commit()
	}
	if err := r.usageReportGrid(ctx, tx, filter, metadata); err != nil {
		return nil, err
	}
	if err := r.readUsageReportRows(ctx, tx, scope, metadata.generation, filter, report); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS usage_report_range`); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE usage_report_calendar`); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

// Reporting and saved public reads require both retention and publication
// watermarks. Internal maintenance readers can inspect an unpublished migration.
func (r *Repository) usageReportingSnapshotTx(ctx context.Context, tx *sqlx.Tx, scope string) (usageProjectionSnapshot, error) {
	metadata, err := r.usageProjectionMetadataTx(ctx, tx, scope)
	if err != nil {
		return metadata, err
	}
	ready, err := r.usageReportRetentionReady(ctx, tx)
	if err != nil {
		return metadata, err
	}
	if !ready {
		metadata.Status, metadata.Generation, metadata.Pending = "preparing", "", true
	} else if metadata.Generation == "" {
		var captured bool
		if err := tx.GetContext(ctx, &captured, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_scopes WHERE scope_id=?)`), scope); err != nil {
			return metadata, err
		}
		if !captured {
			metadata.Status, metadata.Pending = projectionReady, false
		}
	}
	return metadata, nil
}

func (r *Repository) usageReportRetentionReady(ctx context.Context, tx *sqlx.Tx) (bool, error) {
	var ready bool
	err := tx.GetContext(ctx, &ready, `SELECT COUNT(*)=3 FROM usage_history_migration_state WHERE source_key IN ('native','measurements','buckets') AND complete=1`)
	return ready, err
}

func (r *Repository) usageReportGap(ctx context.Context, tx *sqlx.Tx, workspace string, gap *bool) error {
	return tx.GetContext(ctx, gap, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_history_migration_gaps g JOIN workspaces w ON w.id=g.workspace_id AND COALESCE(w.org_id,'')=g.organization_id WHERE g.workspace_id=?)`), workspace)
}

func (r *Repository) readUsageReportRows(ctx context.Context, tx *sqlx.Tx, scope, generation string, filter models.SessionUsageFilter, report *models.TokenUsageReport) error {
	cached, err := r.prepareUsageReportRange(ctx, tx, scope, generation, filter)
	if err != nil {
		return err
	}
	source := usageReportReadSource(scope, generation, filter, false, cached)
	grouped := usageReportGrouped(source, filter.GroupBy, report.ProjectionPending)
	if err := tx.GetContext(ctx, &report.TotalRows, r.db.Rebind(`SELECT COUNT(*) FROM (`+grouped.sql+`) report_groups`), grouped.args...); err != nil {
		return err
	}
	var rows []usageReportAggregate
	args := append(append([]any{}, grouped.args...), filter.Limit, filter.Offset)
	if err := tx.SelectContext(ctx, &rows, r.db.Rebind(`SELECT * FROM (`+grouped.sql+`) usage_report_page`+usageReportOrder(filter)+` LIMIT ? OFFSET ?`), args...); err != nil {
		return err
	}
	for _, row := range rows {
		out := usageReportRow(row)
		markUsageReportMonth(&out, filter)
		if filter.GroupBy == usageWorkFallback || filter.GroupBy == usageSubjectSession {
			if err := r.usageReportAvailability(ctx, tx, filter.WorkspaceID, row, &out); err != nil {
				return err
			}
		} else {
			out.SubjectID, out.TaskID, out.SessionID = "", "", ""
		}
		report.Rows = append(report.Rows, out)
	}
	totalsSource := usageReportReadSource(scope, generation, filter, true, cached)
	var total usageReportAggregate
	if err := tx.GetContext(ctx, &total, r.db.Rebind(`WITH base AS (`+totalsSource.sql+`) SELECT `+usageReportTotalsSQL(report.ProjectionPending)+` FROM base HAVING COUNT(*)>0`), totalsSource.args...); err != nil && err != sql.ErrNoRows {
		return err
	}
	report.Totals = usageReportTotals(total)
	markUsageReportTotalsMonth(report, filter)
	report.Summary = report.Totals
	report.HasMore = filter.Offset+len(rows) < report.TotalRows
	report.HasRangeData = report.TotalRows > 0 && (isDatedUsageGroup(filter.GroupBy) || total.DatedCount > 0)
	providerFilter := filter
	providerFilter.Provider = ""
	providers := usageReportReadSource(scope, generation, providerFilter, false, cached && filter.Provider == "")
	return tx.SelectContext(ctx, &report.Providers, r.db.Rebind(`SELECT DISTINCT CASE WHEN provider='' THEN '__unknown_provider__' ELSE provider END FROM (`+providers.sql+`) report_providers ORDER BY 1`), providers.args...)
}

func markUsageReportTotalsMonth(report *models.TokenUsageReport, filter models.SessionUsageFilter) {
	if (filter.GroupBy == usageGroupMonth || filter.GroupBy == usageGroupMonthly) && report.TotalRows > 0 {
		for _, boundary := range []*time.Time{filter.Start, filter.End} {
			if boundary != nil && !usageReportMonthBoundary(*boundary, filter.Timezone) {
				report.Totals.Partial = true
				report.Totals.EffectiveCostPerMillion = nil
			}
		}
	}
}

func usageReportMonthBoundary(value time.Time, calendar string) bool {
	location, err := timeLocation(calendar)
	if err != nil {
		return false
	}
	local := value.In(location)
	return local.Day() == 1 && local.Hour() == 0 && local.Minute() == 0 && local.Second() == 0 && local.Nanosecond() == 0
}

func markUsageReportMonth(row *models.TokenUsageRow, filter models.SessionUsageFilter) {
	if filter.GroupBy != usageGroupMonth && filter.GroupBy != usageGroupMonthly {
		return
	}
	location, err := timeLocation(row.Calendar)
	if err != nil {
		return
	}
	start, err := time.ParseInLocation("2006-01", row.Period, location)
	if err != nil {
		return
	}
	if filter.Start != nil && filter.Start.After(start) || filter.End != nil && filter.End.Before(start.AddDate(0, 1, 0)) {
		row.Coverage = models.UsageCoveragePartial
		if row.CostCoverage != models.UsageCoverageMissing {
			row.CostCoverage = models.UsageCoveragePartial
		}
		row.EffectiveCostPerMillion = nil
	}
}

func usageReportAccounting(row usageReportAggregate) models.UsageReportAccounting {
	return models.UsageReportAccounting{Calendar: row.Calendar, CalendarBasis: row.CalendarBasis,
		TokenSource: &models.UsageReportSource{Source: row.TokenSource, ActorID: row.TokenActorID, ActorStatus: row.TokenActorStatus, ActorKind: row.TokenActorKind},
		CostSource:  &models.UsageReportSource{Source: row.CostSource, ActorID: row.CostActorID, ActorStatus: row.CostActorStatus, ActorKind: row.CostActorKind}}
}

func reportCoverage(count, partial, missing int64) string {
	if count == 0 || missing >= count {
		return models.UsageCoverageMissing
	}
	if partial > 0 || missing > 0 {
		return models.UsageCoveragePartial
	}
	return models.UsageCoverageComplete
}

func usageReportRow(row usageReportAggregate) models.TokenUsageRow {
	source := row.TokenSource
	if row.CostSource != "" && source != "" && row.CostSource != source {
		source = usageMixedValue
	}
	if source == "" {
		source = row.CostSource
	}
	key, _ := json.Marshal([]string{row.Entity, row.Model, row.Provider, row.Calendar, row.CalendarBasis, row.Currency, row.Period})
	return models.TokenUsageRow{UsageReportAccounting: usageReportAccounting(row), Key: uuid.NewSHA1(uuid.NameSpaceURL, key).String(),
		Period: row.Period, Model: row.Model, Provider: row.Provider, SubjectID: row.SubjectID, TaskID: row.TaskID, SessionID: row.SessionID, AgentClient: row.AgentClient, Source: source,
		InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens, ReasoningTokens: row.ReasoningTokens, TotalTokens: row.TotalTokens, SourceTotalTokens: row.SourceTotalTokens, Turns: row.Turns, CostSubcents: row.CostSubcents,
		Currency: row.Currency, CostBasis: row.CostBasis, CostCoverage: reportCoverage(row.CostCount, row.CostPartial, row.CostMissing), Coverage: reportCoverage(row.TokenCount, row.TokenPartial, row.TotalMissing), Estimated: row.EstimatedCount > 0, Stale: row.StaleCount > 0, Undated: row.UndatedCount > 0, EffectiveCostPerMillion: row.EffectiveCostPerMillion()}
}

func usageReportTotals(row usageReportAggregate) models.UsageTotals {
	return models.UsageTotals{UsageReportAccounting: usageReportAccounting(row), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens, ReasoningTokens: row.ReasoningTokens, TotalTokens: row.TotalTokens, SourceTotalTokens: row.SourceTotalTokens, Turns: row.Turns, CostSubcents: row.CostSubcents, Currency: row.Currency, CostBasis: row.CostBasis, CostCoverage: reportCoverage(row.CostCount, row.CostPartial, row.CostMissing), Partial: row.PartialCount > 0, Undated: row.UndatedCount > 0, Estimated: row.EstimatedCount > 0, EffectiveCostPerMillion: row.EffectiveCostPerMillion()}
}

func (r *Repository) usageReportAvailability(ctx context.Context, tx *sqlx.Tx, workspace string, row usageReportAggregate, out *models.TokenUsageRow) error {
	if row.TaskID == "" {
		return nil
	}
	var created sql.NullTime
	var archived sql.NullTime
	var session, incarnation sql.NullString
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.created_at,t.archived_at,s.id,COALESCE(NULLIF(s.queue_incarnation_id,''),s.id) FROM tasks t LEFT JOIN task_sessions s ON s.task_id=t.id AND s.id=? WHERE t.id=? AND t.workspace_id=?`), row.SessionID, row.TaskID, workspace).Scan(&created, &archived, &session, &incarnation)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	out.TaskAvailable = created.Valid && usageTaskIncarnationID(created.Time) == row.TaskIncarnation
	out.TaskArchived = out.TaskAvailable && archived.Valid
	out.SessionAvailable = out.TaskAvailable && session.Valid && incarnation.String == row.SessionIncarnation
	return nil
}
