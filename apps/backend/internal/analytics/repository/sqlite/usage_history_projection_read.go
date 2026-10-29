package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
)

type usageProjectionSnapshot struct {
	models.UsageProjectionMetadata
	generation string
}

func (r *Repository) listProjectedUsagePage(parent context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error) {
	return r.listCanonicalUsagePage(parent, filter, false)
}

// ListRetainedCanonicalUsagePage is the public saved-view cutover. Internal
// projection readers remain available to migration and maintenance workflows.
func (r *Repository) ListRetainedCanonicalUsagePage(parent context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error) {
	return r.listCanonicalUsagePage(parent, filter, true)
}

func (r *Repository) listCanonicalUsagePage(parent context.Context, filter models.SessionUsageFilter, retained bool) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return nil, models.UsageProjectionState{}, err
	}
	defer release()
	rows, metadata, err := r.readProjectedUsagePage(ctx, filter, retained)
	return rows, metadata.UsageProjectionState, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) readProjectedUsagePage(ctx context.Context, filter models.SessionUsageFilter, retained bool) ([]models.SessionUsageMeasurement, models.UsageProjectionMetadata, error) {
	facts, metadata, err := r.readUsageCanonicalFacts(ctx, filter, retained)
	rows := make([]models.SessionUsageMeasurement, 0, len(facts))
	for _, fact := range facts {
		measurement := fact.Measurement
		measurement.TokenMetricAbsent, measurement.CostMetricAbsent = fact.TokenMetricAbsent, fact.CostMetricAbsent
		rows = append(rows, measurement)
	}
	return rows, metadata, err
}

// ListUsageCanonicalFacts retains independent metric evidence for internal
// reporting adapters. The returned publication token applies to the same SQL
// snapshot as every fact on this page.
func (r *Repository) ListUsageCanonicalFacts(parent context.Context, filter models.SessionUsageFilter) ([]models.UsageCanonicalFact, models.UsageProjectionMetadata, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	defer release()
	facts, metadata, err := r.readUsageCanonicalFacts(ctx, filter, false)
	return facts, metadata, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) readUsageCanonicalFacts(ctx context.Context, filter models.SessionUsageFilter, retained bool) ([]models.UsageCanonicalFact, models.UsageProjectionMetadata, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.authorizedUsageScopeTx(ctx, tx, filter.WorkspaceID)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	var metadata usageProjectionSnapshot
	if retained {
		metadata, err = r.usageReportingSnapshotTx(ctx, tx, scope)
	} else {
		metadata, err = r.usageProjectionMetadataTx(ctx, tx, scope)
	}
	if err != nil {
		return nil, metadata.UsageProjectionMetadata, err
	}
	if filter.ProjectionGeneration != "" && filter.ProjectionGeneration != metadata.Generation {
		return nil, metadata.UsageProjectionMetadata, analytics.ErrUsageProjectionChanged
	}
	if metadata.Generation == "" {
		return nil, metadata.UsageProjectionMetadata, tx.Commit()
	}
	limit := projectionLimit(filter.Limit)
	if filter.Limit == 201 {
		limit = 201
	}
	queryFilter := filter
	queryFilter.Limit = min(limit+1, 201)
	query, args := projectedUsageFactQuery(scope, metadata.generation, queryFilter)
	var payloads []usageProjectedFactPayload
	if err := tx.SelectContext(ctx, &payloads, r.db.Rebind(query), args...); err != nil {
		return nil, metadata.UsageProjectionMetadata, err
	}
	if len(payloads) > limit {
		payloads = payloads[:limit]
		metadata.HasMore, metadata.NextOffset = true, max(filter.Offset, 0)+limit
	}
	rows := make([]models.UsageCanonicalFact, 0, len(payloads))
	for _, payload := range payloads {
		var fact models.UsageCanonicalFact
		if err := decodeUsageProjection(payload.Payload, &fact); err != nil {
			return nil, metadata.UsageProjectionMetadata, err
		}
		if !payload.KeepTokens {
			clearUsageFactTokens(&fact)
		}
		if !payload.KeepCost {
			clearUsageFactCost(&fact)
		}
		fact.NativeAccountingVersion = normalizeNativeCanonicalAccounting(&fact, fact.NativeAccountingVersion)
		rows = append(rows, fact)
	}
	return rows, metadata.UsageProjectionMetadata, tx.Commit()
}

func (r *Repository) usageProjectionMetadataTx(ctx context.Context, tx *sqlx.Tx, scope string) (usageProjectionSnapshot, error) {
	metadata := usageProjectionSnapshot{UsageProjectionMetadata: models.UsageProjectionMetadata{UsageProjectionState: models.UsageProjectionState{Status: "preparing", Pending: true}}}
	var head, replay, facts, dated, lifetime, intervals, native int64
	var indexedVersion, requiredVersion, segmentVersion int64
	var accountingVersion, accountingSequence int64
	query := `SELECT s.active_generation, s.commit_sequence, s.revision, COALESCE(g.replay_sequence, 0),
		COALESCE(g.fact_count, 0), COALESCE(g.dated_count, 0), COALESCE(g.lifetime_count, 0), COALESCE(g.interval_count, 0), COALESCE(g.native_count, 0),
		g.earliest, g.latest, g.collected_at, COALESCE(g.work_version,0), u.work_version, u.segment_version,
		s.native_accounting_source_version,s.native_accounting_required_sequence FROM usage_projection_scopes s JOIN usage_scopes u ON u.scope_id=s.scope_id LEFT JOIN usage_projection_generations g
		ON g.scope_id = s.scope_id AND g.generation = s.active_generation WHERE s.scope_id = ?`
	if r.db.DriverName() == usageDriverPostgres {
		query += ` FOR SHARE OF s`
	}
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), scope).
		Scan(&metadata.generation, &head, &metadata.Revision, &replay, &facts, &dated, &lifetime, &intervals, &native, &metadata.Earliest, &metadata.Latest, &metadata.LastUpdated, &indexedVersion, &requiredVersion, &segmentVersion, &accountingVersion, &accountingSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return metadata, nil
	}
	if err != nil || metadata.generation == "" {
		return metadata, err
	}
	if indexedVersion != requiredVersion {
		return metadata, nil
	}
	var pending bool
	err = tx.GetContext(ctx, &pending, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_work WHERE scope_id = ? AND generation = ?)
		OR EXISTS(SELECT 1 FROM usage_projection_generations WHERE scope_id = ? AND state IN ('scan', 'replay'))
		OR EXISTS(SELECT 1 FROM usage_subjects WHERE scope_id=? AND work_repair_pending=1)`), scope, metadata.generation, scope, scope)
	metadata.Status, metadata.Pending = projectionReady, pending || replay < head || segmentVersion == 0 || accountingVersion < nativeUsageAccountingRepairVersion || replay < accountingSequence
	metadata.Generation = metadata.generation + "." + strconv.FormatInt(metadata.Revision, 36)
	metadata.HasHistory, metadata.HasDated, metadata.HasUndated = facts > 0, dated > 0, lifetime > 0
	metadata.HasIntervals, metadata.HasNative = intervals > 0, native > 0
	return metadata, err
}

func projectedUsageFactQuery(scope, generation string, filter models.SessionUsageFilter) (string, []any) {
	query := `SELECT f.payload,(` + usageProjectedMetricMask(filter, projectionMetricTokens) + `) AS keep_tokens,(` + usageProjectedMetricMask(filter, projectionMetricCost) + `) AS keep_cost FROM usage_canonical_facts f JOIN usage_projection_applied a
		ON a.scope_id = f.scope_id AND a.generation = f.generation AND a.work_key = f.work_key AND a.build_id = f.build_id
		WHERE f.scope_id = ? AND f.generation = ?`
	args := []any{scope, generation}
	for _, group := range []struct {
		column string
		values []string
	}{{"task_id", filter.TaskIDs}, {"session_id", filter.SessionIDs}} {
		if len(group.values) > 0 {
			query += ` AND f.` + group.column + ` IN (` + placeholders(len(group.values)) + `)`
			args = append(args, stringArgs(group.values)...)
		}
	}
	for _, dimension := range []struct{ column, value string }{{"model", filter.Model}, {"provider", filter.Provider}} {
		if dimension.value != "" {
			query += ` AND f.` + dimension.column + ` = ?`
			args = append(args, dimension.value)
		}
	}
	if filter.Source != "" {
		query += ` AND (f.token_source = ? OR f.cost_source = ?)`
		args = append(args, filter.Source, filter.Source)
	}
	query, args = projectedUsageTimeQuery(query, args, filter)
	limit := projectionLimit(filter.Limit)
	if filter.Limit == 201 {
		limit = 201
	}
	offset := max(filter.Offset, 0)
	query += ` ORDER BY f.coverage_start, f.fact_key LIMIT ? OFFSET ?`
	return query, append(args, limit, offset)
}

func projectedUsageTimeQuery(query string, args []any, filter models.SessionUsageFilter) (string, []any) {
	if filter.Start == nil && filter.End == nil && !isDatedUsageGroup(filter.GroupBy) {
		return query + ` AND ((f.token_source<>'' AND ` + usageProjectedMetricMask(filter, projectionMetricTokens) + `)
		 OR (f.cost_source<>'' AND ` + usageProjectedMetricMask(filter, projectionMetricCost) + `))`, args
	}
	point, interval := `f.coverage_kind = 'point'`, `f.coverage_kind <> 'point' AND f.coverage_start IS NOT NULL`
	var pointArgs, intervalArgs []any
	if filter.Start != nil {
		point += ` AND f.coverage_start >= ?`
		interval += ` AND f.coverage_end > ?`
		pointArgs, intervalArgs = append(pointArgs, *filter.Start), append(intervalArgs, *filter.Start)
	}
	if filter.End != nil {
		point += ` AND f.coverage_start < ?`
		interval += ` AND f.coverage_start < ?`
		pointArgs, intervalArgs = append(pointArgs, *filter.End), append(intervalArgs, *filter.End)
	}
	return query + ` AND ((` + point + `) OR (` + interval + `))`, append(append(args, pointArgs...), intervalArgs...)
}

// ListUsageDailySummaries reads compact summaries only for the requested
// calendar. Interval and foreign-calendar facts remain in the timed fact index.
func (r *Repository) ListUsageDailySummaries(parent context.Context, filter models.SessionUsageFilter) ([]models.UsageDailySummary, models.UsageProjectionMetadata, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	defer release()
	rows, metadata, err := r.readUsageDailySummaries(ctx, filter)
	return rows, metadata, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) readUsageDailySummaries(ctx context.Context, filter models.SessionUsageFilter) ([]models.UsageDailySummary, models.UsageProjectionMetadata, error) {
	if len(filter.TaskIDs) > 0 || len(filter.SessionIDs) > 0 {
		return nil, models.UsageProjectionMetadata{}, fmt.Errorf("subject filters require canonical facts")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.authorizedUsageScopeTx(ctx, tx, filter.WorkspaceID)
	if err != nil {
		return nil, models.UsageProjectionMetadata{}, err
	}
	metadata, err := r.usageProjectionMetadataTx(ctx, tx, scope)
	if err != nil || metadata.Generation == "" {
		return nil, metadata.UsageProjectionMetadata, err
	}
	query, args, limit, err := usageDailySummaryQuery(scope, metadata.generation, filter)
	if err != nil {
		return nil, metadata.UsageProjectionMetadata, err
	}
	var summaries []models.UsageDailySummary
	if err := tx.SelectContext(ctx, &summaries, r.db.Rebind(query), args...); err != nil {
		return nil, metadata.UsageProjectionMetadata, err
	}
	if len(summaries) > limit {
		summaries = summaries[:limit]
		metadata.HasMore, metadata.NextOffset = true, max(filter.Offset, 0)+limit
	}
	for i := range summaries {
		suppressNativeSummaryTurns(&summaries[i])
		suppressPendingNativeSummaryReasoning(&summaries[i], metadata.Pending)
	}
	return summaries, metadata.UsageProjectionMetadata, tx.Commit()
}

func usageSummaryCalendar(filter models.SessionUsageFilter) (string, *time.Location, error) {
	calendar := filter.Timezone
	if calendar == "" {
		calendar = projectionCalendarUTC
	}
	location, err := timeLocation(calendar)
	if err != nil {
		return "", nil, err
	}
	for _, boundary := range []*time.Time{filter.Start, filter.End} {
		if boundary != nil {
			local := boundary.In(location)
			if local.Hour() != 0 || local.Minute() != 0 || local.Second() != 0 || local.Nanosecond() != 0 {
				return "", nil, fmt.Errorf("summary boundaries must match calendar days")
			}
		}
	}
	return calendar, location, nil
}

func usageDailySummaryQuery(scope, generation string, filter models.SessionUsageFilter) (string, []any, int, error) {
	calendar, location, err := usageSummaryCalendar(filter)
	if err != nil {
		return "", nil, 0, err
	}
	query := `SELECT * FROM usage_daily_summaries WHERE scope_id = ? AND generation = ? AND calendar = ?`
	args := []any{scope, generation, calendar}
	if filter.Start != nil {
		query += ` AND usage_date >= ?`
		args = append(args, filter.Start.In(location).Format("2006-01-02"))
	}
	if filter.End != nil {
		query += ` AND usage_date < ?`
		args = append(args, filter.End.In(location).Format("2006-01-02"))
	}
	for _, dimension := range []struct{ column, value string }{{"provider", filter.Provider}, {"model", filter.Model}, {"source", filter.Source}} {
		if dimension.value != "" {
			query += ` AND ` + dimension.column + ` = ?`
			args = append(args, dimension.value)
		}
	}
	query += ` ORDER BY usage_date, summary_key LIMIT ? OFFSET ?`
	limit := projectionLimit(filter.Limit)
	args = append(args, limit+1, max(filter.Offset, 0))
	return query, args, limit, nil
}

func timeLocation(calendar string) (*time.Location, error) {
	if strings.TrimSpace(calendar) == "" {
		return nil, fmt.Errorf("usage calendar is required")
	}
	return time.LoadLocation(calendar)
}
