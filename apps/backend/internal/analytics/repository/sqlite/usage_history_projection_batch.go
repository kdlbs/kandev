package sqlite

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

const usageProjectionBatchArgumentLimit = 999

type usageProjectionSourceIndex struct {
	ID          string `db:"id"`
	ItemID      string `db:"item_id"`
	WorkKey     string `db:"selection_partition"`
	CoverageKey string `db:"selection_coverage"`
	WorkStatus  string `db:"work_status"`
	Metadata    string `db:"observation_metadata"`
	Retired     int    `db:"retired"`
	EventID     string `db:"work_event_id"`
	Bundle      string `db:"selection_bundle"`
	Segment     string `db:"selection_segment"`
	GroupKey    string `db:"selection_group"`
	Layout      int    `db:"selection_layout"`
	TokenBundle string `db:"selection_token_bundle"`
	CostBundle  string `db:"selection_cost_bundle"`
	ScanKey     string `db:"selection_scan_key"`
	Sequence    int64  `db:"commit_sequence"`
}

type usageProjectionPendingWork struct {
	WorkKey  string
	Subject  string
	Sequence int64
}

type usageProjectionDirtyPartition struct {
	Scope     string
	Subject   string
	Partition string
	UpdatedAt time.Time
}

type usageProjectionIndexBatch struct {
	Scope          string
	Sequence       int64
	CreatedAt      time.Time
	Logs           [][]any
	Partitions     map[string][]any
	SourceVersions [][]any
	Dirty          map[string]usageProjectionDirtyPartition
	Sequences      map[string]int64
	HasNative      bool
}

func (r *Repository) stageUsageProjectionFactTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, fact models.UsageCanonicalFact) error {
	return r.stageUsageProjectionFactsTx(ctx, tx, work, []models.UsageCanonicalFact{fact})
}

func (r *Repository) copyUsageProjectionSourcePageTx(ctx context.Context, tx *sqlx.Tx, scope, generation, table string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.loadUsageProjectionSourcePageTx(ctx, tx, scope, table, ids)
	if err != nil {
		return err
	}
	indexed, legacyIDs := splitUsageProjectionSourceRows(rows)
	missingVersions := usageProjectionRowsMissingSequence(indexed)
	sequences, err := r.appendUsageProjectionIndexBatchTx(ctx, tx, missingVersions)
	if err != nil {
		return err
	}
	pending, err := r.stageUsageProjectionSourceItemsTx(ctx, tx, scope, generation, table, indexed, sequences)
	if err != nil {
		return err
	}
	if err := r.enqueueUsageProjectionWorkBatchTx(ctx, tx, scope, generation, pending); err != nil {
		return err
	}
	return r.copyLegacyUsageProjectionSourceIDsTx(ctx, tx, scope, generation, table, legacyIDs)
}

func splitUsageProjectionSourceRows(rows []indexedObservation) ([]indexedObservation, []string) {
	indexed := make([]indexedObservation, 0, len(rows))
	legacyIDs := make([]string, 0)
	for _, row := range rows {
		if row.WorkKey == "" {
			legacyIDs = append(legacyIDs, row.Measurement.ID)
			continue
		}
		indexed = append(indexed, row)
	}
	return indexed, legacyIDs
}

func usageProjectionRowsMissingSequence(rows []indexedObservation) []indexedObservation {
	missing := make([]indexedObservation, 0, len(rows))
	for _, row := range rows {
		if row.CommitSequence == 0 {
			missing = append(missing, row)
		}
	}
	return missing
}

func (r *Repository) stageUsageProjectionSourceItemsTx(ctx context.Context, tx *sqlx.Tx, scope, generation, table string, indexed []indexedObservation, sequences map[string]int64) (map[string]usageProjectionPendingWork, error) {
	items := make([][]any, 0, len(indexed))
	pending := make(map[string]usageProjectionPendingWork)
	for _, row := range indexed {
		sequence := row.CommitSequence
		if sequence == 0 {
			sequence = sequences[row.Measurement.ID]
		}
		if sequence == 0 {
			return nil, fmt.Errorf("missing usage source version for %s", row.Measurement.ID)
		}
		items = append(items, []any{scope, generation, table, row.Measurement.ID, sequence, row.Retired})
		work := pending[row.WorkKey]
		if sequence > work.Sequence {
			pending[row.WorkKey] = usageProjectionPendingWork{WorkKey: row.WorkKey, Subject: row.Measurement.SubjectID, Sequence: sequence}
		}
	}
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_items",
		"scope_id,generation,source_table,source_id,commit_sequence,tombstone", items,
		`ON CONFLICT(scope_id,generation,source_table,source_id) DO UPDATE SET
		commit_sequence=excluded.commit_sequence,tombstone=excluded.tombstone
		WHERE usage_projection_items.commit_sequence<excluded.commit_sequence
		OR (usage_projection_items.commit_sequence=excluded.commit_sequence AND excluded.tombstone=0)`); err != nil {
		return nil, err
	}
	return pending, nil
}

func (r *Repository) copyLegacyUsageProjectionSourceIDsTx(ctx context.Context, tx *sqlx.Tx, scope, generation, table string, ids []string) error {
	for _, id := range ids {
		if err := r.copyUsageProjectionSourceTx(ctx, tx, scope, generation, table, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) loadUsageProjectionSourcePageTx(ctx context.Context, tx *sqlx.Tx, scope, table string, ids []string) ([]indexedObservation, error) {
	query, args, err := sqlx.In(`SELECT `+usageSelectColumns+` FROM `+table+` WHERE scope_id=? AND id IN (?) ORDER BY id`, scope, ids)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	observations := make(map[string]indexedObservation, len(ids))
	for rows.Next() {
		stored, err := scanUsageRow(rows)
		if err != nil {
			return nil, err
		}
		row := indexedObservation{Measurement: stored.stored().measurement(), Table: table}
		observations[row.Measurement.ID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(observations) != len(ids) {
		return nil, ErrUsageNotVisible
	}
	var indexes []usageProjectionSourceIndex
	query, args, err = sqlx.In(`SELECT r.id,r.item_id,r.selection_partition,r.selection_coverage,r.work_status,r.observation_metadata,r.retired,r.work_event_id,
		r.selection_bundle,r.selection_segment,r.selection_group,r.selection_layout,r.selection_token_bundle,r.selection_cost_bundle,r.selection_scan_key,
		COALESCE(v.commit_sequence,0) AS commit_sequence
		FROM `+table+` r LEFT JOIN usage_projection_source_versions v
		ON v.scope_id=r.scope_id AND v.source_table=? AND v.source_id=r.id
		WHERE r.scope_id=? AND r.id IN (?)`, table, scope, ids)
	if err != nil {
		return nil, err
	}
	if err := tx.SelectContext(ctx, &indexes, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	if len(indexes) != len(ids) {
		return nil, ErrUsageNotVisible
	}
	for _, index := range indexes {
		row, ok := observations[index.ID]
		if !ok {
			return nil, ErrUsageNotVisible
		}
		row.ItemID, row.WorkKey, row.CoverageKey = index.ItemID, index.WorkKey, index.CoverageKey
		row.WorkStatus, row.Metadata, row.Retired, row.EventID = index.WorkStatus, index.Metadata, index.Retired, index.EventID
		row.Bundle, row.Segment, row.GroupKey, row.Layout = index.Bundle, index.Segment, index.GroupKey, index.Layout
		row.TokenBundle, row.CostBundle, row.ScanKey, row.CommitSequence = index.TokenBundle, index.CostBundle, index.ScanKey, index.Sequence
		observations[index.ID] = row
	}
	out := make([]indexedObservation, 0, len(ids))
	for _, id := range ids {
		row, ok := observations[id]
		if !ok {
			return nil, ErrUsageNotVisible
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *Repository) appendUsageProjectionIndexBatchTx(ctx context.Context, tx *sqlx.Tx, rows []indexedObservation) (map[string]int64, error) {
	sequences := make(map[string]int64, len(rows))
	if len(rows) == 0 {
		return sequences, nil
	}
	scope := rows[0].Measurement.ScopeID
	var sequence int64
	if err := tx.GetContext(ctx, &sequence, r.db.Rebind(`SELECT commit_sequence FROM usage_projection_scopes WHERE scope_id=?`), scope); err != nil {
		return nil, err
	}
	batch, err := buildUsageProjectionIndexBatch(scope, sequence, time.Now().UTC(), rows)
	if err != nil {
		return nil, err
	}
	if err := r.persistUsageProjectionIndexBatchTx(ctx, tx, batch); err != nil {
		return nil, err
	}
	if err := r.updateUsageProjectionScopeIndexSequenceTx(ctx, tx, batch); err != nil {
		return nil, err
	}
	return batch.Sequences, nil
}

func buildUsageProjectionIndexBatch(scope string, sequence int64, now time.Time, rows []indexedObservation) (usageProjectionIndexBatch, error) {
	batch := usageProjectionIndexBatch{
		Scope: scope, Sequence: sequence, CreatedAt: now,
		Logs: make([][]any, 0, len(rows)), Partitions: make(map[string][]any),
		SourceVersions: make([][]any, 0, len(rows)), Dirty: make(map[string]usageProjectionDirtyPartition),
		Sequences: make(map[string]int64, len(rows)),
	}
	for _, row := range rows {
		if row.Measurement.ScopeID != scope || row.Table != usageMeasurementsTable && row.Table != usageBucketsTable {
			return usageProjectionIndexBatch{}, errors.New("usage projection source page crosses retained scopes")
		}
		if batch.Sequence == math.MaxInt64 {
			return usageProjectionIndexBatch{}, errors.New("usage commit sequence exhausted")
		}
		batch.Sequence++
		change := usageMutationFromRows("index", nil, &row)
		replay := usageProjectionReplay{Version: 1, Mutation: change, AfterTable: row.Table}
		payload, err := encodeUsageProjection(replay)
		if err != nil {
			return usageProjectionIndexBatch{}, fmt.Errorf("encode usage replay: %w", err)
		}
		batch.Logs = append(batch.Logs, []any{scope, batch.Sequence, payload, uuid.NewString(), now})
		partition := row.partition()
		batch.Partitions[partition.WorkKey] = []any{scope, partition.WorkKey, partition.SubjectID, batch.Sequence}
		batch.SourceVersions = append(batch.SourceVersions, []any{scope, row.Table, row.Measurement.ID, batch.Sequence, 0})
		dirtyKey := partition.SubjectID + "\x00" + partition.PartitionID
		batch.Dirty[dirtyKey] = usageProjectionDirtyPartition{Scope: scope, Subject: partition.SubjectID, Partition: partition.PartitionID, UpdatedAt: row.Measurement.CollectedAt}
		batch.Sequences[row.Measurement.ID] = batch.Sequence
		batch.HasNative = batch.HasNative || row.Measurement.Source == usageSourceNative
	}
	return batch, nil
}

func (r *Repository) persistUsageProjectionIndexBatchTx(ctx context.Context, tx *sqlx.Tx, batch usageProjectionIndexBatch) error {
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_log", "scope_id,commit_sequence,payload,publication_group,created_at", batch.Logs,
		`ON CONFLICT(scope_id,commit_sequence) DO UPDATE SET payload=excluded.payload,publication_group=excluded.publication_group,created_at=excluded.created_at`); err != nil {
		return err
	}
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_partitions", "scope_id,work_key,subject_id,commit_sequence", mapRows(batch.Partitions),
		`ON CONFLICT(scope_id,work_key) DO UPDATE SET commit_sequence=excluded.commit_sequence`); err != nil {
		return err
	}
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_source_versions", "scope_id,source_table,source_id,commit_sequence,tombstone", batch.SourceVersions,
		`ON CONFLICT(scope_id,source_table,source_id) DO UPDATE SET commit_sequence=excluded.commit_sequence,tombstone=excluded.tombstone`); err != nil {
		return err
	}
	dirtyRows := make([][]any, 0, len(batch.Dirty))
	for _, item := range batch.Dirty {
		dirtyRows = append(dirtyRows, []any{item.Scope, item.Subject, item.Partition, item.UpdatedAt})
	}
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_dirty_partitions", "scope_id,subject_id,partition_key,updated_at", dirtyRows,
		`ON CONFLICT(scope_id,subject_id,partition_key) DO UPDATE SET updated_at=excluded.updated_at`); err != nil {
		return err
	}
	return nil
}

func (r *Repository) updateUsageProjectionScopeIndexSequenceTx(ctx context.Context, tx *sqlx.Tx, batch usageProjectionIndexBatch) error {
	if batch.HasNative {
		pending, err := r.nativeUsageAccountingRepairPendingTx(ctx, tx, batch.Scope)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET commit_sequence=?,updated_at=?,
			native_accounting_source_version=CASE WHEN ? THEN 0 ELSE native_accounting_source_version END,
			native_accounting_required_sequence=? WHERE scope_id=?`), batch.Sequence, batch.CreatedAt, pending, batch.Sequence, batch.Scope)
		if err != nil {
			return err
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET commit_sequence=?,updated_at=? WHERE scope_id=?`), batch.Sequence, batch.CreatedAt, batch.Scope)
	return err
}

func (r *Repository) enqueueUsageProjectionWorkBatchTx(ctx context.Context, tx *sqlx.Tx, scope, generation string, pending map[string]usageProjectionPendingWork) error {
	if len(pending) == 0 {
		return nil
	}
	keys := make([]string, 0, len(pending))
	for key := range pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	query, args, err := sqlx.In(`SELECT work_key,commit_sequence FROM usage_projection_applied WHERE scope_id=? AND generation=? AND work_key IN (?)`, scope, generation, keys)
	if err != nil {
		return err
	}
	var appliedRows []struct {
		WorkKey  string `db:"work_key"`
		Sequence int64  `db:"commit_sequence"`
	}
	if err := tx.SelectContext(ctx, &appliedRows, r.db.Rebind(query), args...); err != nil {
		return err
	}
	applied := make(map[string]int64, len(appliedRows))
	for _, row := range appliedRows {
		applied[row.WorkKey] = row.Sequence
	}
	workRows := make([][]any, 0, len(keys))
	for _, key := range keys {
		row := pending[key]
		if applied[key] >= row.Sequence {
			continue
		}
		workRows = append(workRows, []any{scope, generation, key, row.Subject, row.Sequence, key})
	}
	return r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_work", "scope_id,generation,work_key,subject_id,requested_sequence,publish_group", workRows,
		`ON CONFLICT(scope_id,generation,work_key) DO UPDATE SET
		requested_sequence=CASE WHEN excluded.requested_sequence>usage_projection_work.requested_sequence THEN excluded.requested_sequence ELSE usage_projection_work.requested_sequence END,
		state=CASE WHEN excluded.requested_sequence>usage_projection_work.selection_sequence THEN 'pending' ELSE usage_projection_work.state END`)
}

func (r *Repository) insertUsageProjectionRowsTx(ctx context.Context, tx *sqlx.Tx, table, columns string, rows [][]any, conflict string) error {
	if len(rows) == 0 {
		return nil
	}
	columnCount := len(rows[0])
	if columnCount == 0 {
		return errors.New("usage projection insert has no columns")
	}
	for _, row := range rows {
		if len(row) != columnCount {
			return errors.New("usage projection insert has inconsistent row width")
		}
	}
	pageSize := max(1, usageProjectionBatchArgumentLimit/columnCount)
	for start := 0; start < len(rows); start += pageSize {
		end := min(start+pageSize, len(rows))
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*columnCount)
		for _, row := range rows[start:end] {
			values = append(values, "("+placeholders(columnCount)+")")
			args = append(args, row...)
		}
		query := `INSERT INTO ` + table + ` (` + columns + `) VALUES ` + strings.Join(values, ",") + ` ` + conflict
		if _, err := tx.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) stageUsageProjectionFactsTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, facts []models.UsageCanonicalFact) error {
	if len(facts) == 0 {
		return nil
	}
	canonicalRows := make([][]any, 0, len(facts))
	contributionRows := make([][]any, 0, len(facts)*2)
	for _, fact := range facts {
		canonicalRow, err := usageProjectionCanonicalFactRow(work, fact)
		if err != nil {
			return err
		}
		canonicalRows = append(canonicalRows, canonicalRow)
		factContributions, err := usageProjectionFactContributionRows(work, fact)
		if err != nil {
			return err
		}
		contributionRows = append(contributionRows, factContributions...)
	}
	if err := r.insertUsageProjectionRowsTx(ctx, tx, "usage_canonical_facts",
		"scope_id,generation,build_id,fact_key,work_key,subject_id,task_id,session_id,coverage_kind,coverage_start,coverage_end,source_date,calendar,model,provider,token_source,cost_source,payload,collected_at,segment_key,bundle_key",
		canonicalRows,
		`ON CONFLICT(scope_id,generation,build_id,fact_key) DO UPDATE SET payload=excluded.payload,collected_at=excluded.collected_at`); err != nil {
		return err
	}
	return r.insertUsageProjectionRowsTx(ctx, tx, "usage_projection_contributions", strings.Join(usageProjectionContributionColumns, ","), contributionRows,
		`ON CONFLICT(scope_id,generation,build_id,fact_key,metric_kind,calendar,usage_date) DO NOTHING`)
}

func usageProjectionCanonicalFactRow(work usageProjectionWork, fact models.UsageCanonicalFact) ([]any, error) {
	fact.NativeAccountingVersion = normalizeNativeCanonicalAccounting(&fact, fact.NativeAccountingVersion)
	if fact.CoverageKind == usageCoveragePoint && fact.Measurement.CoverageStart != nil {
		end := fact.Measurement.CoverageStart.Add(time.Nanosecond)
		fact.Measurement.CoverageEnd = &end
	}
	payload, err := encodeUsageProjection(fact)
	if err != nil {
		return nil, err
	}
	m := fact.Measurement
	tokenSource, costSource := "", ""
	if fact.TokenSource != nil {
		tokenSource = fact.TokenSource.Source
	}
	if fact.CostSource != nil {
		costSource = fact.CostSource.Source
	}
	start, end := usageProjectionFactBounds(fact)
	return []any{
		work.Scope, work.Generation, work.Build, fact.Key, work.Work, m.SubjectID, m.TaskID, m.SessionID,
		fact.CoverageKind, start, end, m.SourceDate, m.SourceTimezone, m.Model, m.Provider,
		tokenSource, costSource, payload, m.CollectedAt, fact.SegmentKey, fact.BundleKey,
	}, nil
}

func usageProjectionFactContributionRows(work usageProjectionWork, fact models.UsageCanonicalFact) ([][]any, error) {
	periods, err := usageProjectionPeriods(fact)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(periods)*2)
	for _, period := range periods {
		for _, metric := range []string{projectionMetricTokens, projectionMetricCost} {
			if usageProjectionMetricAbsent(fact, metric) {
				continue
			}
			row := usageProjectionContribution(work, fact, metric, period)
			values := make([]any, len(usageProjectionContributionColumns))
			for index, column := range usageProjectionContributionColumns {
				values[index] = row[column]
			}
			rows = append(rows, values)
		}
	}
	return rows, nil
}

func usageProjectionMetricAbsent(fact models.UsageCanonicalFact, metric string) bool {
	return metric == projectionMetricTokens && fact.TokenMetricAbsent || metric == projectionMetricCost && fact.CostMetricAbsent
}

func mapRows(values map[string][]any) [][]any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([][]any, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, values[key])
	}
	return rows
}
