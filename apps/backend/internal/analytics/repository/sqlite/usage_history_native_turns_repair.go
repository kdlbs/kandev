package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db"
)

const nativeUsageAccountingRepairVersion = 1
const nativeUsageAccountingRepairOperation = "native_accounting_v1"
const nativeUsageAccountingPendingSQL = "source='native' AND (turns IS NOT NULL OR native_accounting_version < 1)"

// ensureNativeUsageAccountingRepairSchema keeps pending-row discovery bounded even
// after most historical rows have been repaired. The row version also schedules
// reasoning re-projection without changing raw reasoning amounts or digests.
func (r *Repository) ensureNativeUsageAccountingRepairSchema(ctx context.Context) error {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		_, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN native_accounting_version INTEGER NOT NULL DEFAULT 0`)
		if err != nil && !db.IsDuplicateColumnError(err) {
			return fmt.Errorf("add native accounting repair version to %s: %w", table, err)
		}
		statement := `CREATE INDEX IF NOT EXISTS idx_` + table + `_native_accounting_pending_v1 ON ` + table +
			`(scope_id, id) WHERE ` + nativeUsageAccountingPendingSQL
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("index pending native turn repair for %s: %w", table, err)
		}
	}
	return r.ensureNativeUsageAccountingStateSchema(ctx)
}

// repairNativeUsageAccountingBatch repairs at most limit rows in one captured scope.
// The accounting version is the durable completion marker: source changes, invalidations,
// and replay sequences commit together, so cancellation needs no local cursor.
// Callers must finish retained-scope backfill first and continue projecting the
// recorded mutations after this returns done. done does not mean published.
func (r *Repository) repairNativeUsageAccountingBatch(ctx context.Context, limit int) (bool, error) {
	if limit < 1 {
		return false, errors.New("native turn repair batch limit must be positive")
	}
	limit = min(limit, usageHistoryBatchLimit)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.nextNativeUsageAccountingRepairScopeTx(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return true, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	// This resolves workspace/org from the retained scope and participates in
	// the same erasure fences and publication lock as ordinary source writes.
	if _, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope); err != nil {
		return false, err
	}
	refs, err := r.nativeUsageAccountingRepairReferencesTx(ctx, tx, scope, limit)
	if err != nil {
		return false, err
	}
	var changes []models.UsageMutation
	for _, ref := range refs {
		change, err := r.repairNativeUsageAccountingRowTx(ctx, tx, scope, ref)
		if err != nil {
			return false, err
		}
		if change != nil {
			changes = append(changes, *change)
		}
	}
	if err := r.groupUsageProjectionMutationsTx(ctx, tx, changes); err != nil {
		return false, err
	}
	if err := r.completeNativeUsageAccountingSourceTx(ctx, tx, scope); err != nil {
		return false, err
	}
	_, err = r.nextNativeUsageAccountingRepairScopeTx(ctx, tx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	done := errors.Is(err, sql.ErrNoRows)
	return done, tx.Commit()
}

func (r *Repository) nextNativeUsageAccountingRepairScopeTx(ctx context.Context, tx *sqlx.Tx) (string, error) {
	var scope string
	query := `SELECT scope_id FROM (
		SELECT scope_id FROM (SELECT scope_id FROM session_usage_measurements
			WHERE ` + nativeUsageAccountingPendingSQL + ` ORDER BY scope_id, id LIMIT 1) measurements
		UNION ALL
		SELECT scope_id FROM (SELECT scope_id FROM session_usage_buckets
			WHERE ` + nativeUsageAccountingPendingSQL + ` ORDER BY scope_id, id LIMIT 1) buckets
		UNION ALL
		SELECT scope_id FROM (SELECT scope_id FROM usage_projection_scopes
			WHERE native_accounting_source_version < 1 ORDER BY native_accounting_source_version,scope_id LIMIT 1) states
		UNION ALL
		SELECT scope_id FROM (SELECT scope_id FROM usage_scopes
			WHERE projection_registered=0 ORDER BY scope_id LIMIT 1) unregistered
	) pending ORDER BY scope_id LIMIT 1`
	err := tx.GetContext(ctx, &scope, query)
	return scope, err
}

func (r *Repository) nativeUsageAccountingRepairReferencesTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) ([]selectionReference, error) {
	query := `SELECT id, source_table FROM (
		SELECT id, 'session_usage_measurements' AS source_table FROM (SELECT id FROM session_usage_measurements
			WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + ` ORDER BY id LIMIT ?) measurements
		UNION ALL
		SELECT id, 'session_usage_buckets' AS source_table FROM (SELECT id FROM session_usage_buckets
			WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + ` ORDER BY id LIMIT ?) buckets
	) pending ORDER BY source_table, id LIMIT ?`
	var refs []selectionReference
	err := tx.SelectContext(ctx, &refs, r.db.Rebind(query), scope, limit, scope, limit, limit)
	return refs, err
}

func (r *Repository) repairNativeUsageAccountingRowTx(ctx context.Context, tx *sqlx.Tx, scope string, ref selectionReference) (*models.UsageMutation, error) {
	before, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
	if err != nil {
		return nil, err
	}
	if before.Measurement.ScopeID != scope {
		return nil, ErrUsageNotVisible
	}
	if before.Measurement.Source != usageSourceNative {
		return nil, nil
	}
	if before.WorkKey == "" {
		if err := r.indexRetainedSourceRowTx(ctx, tx, ref.Table, ref.ID); err != nil {
			return nil, err
		}
		before, err = r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+ref.Table+`
		SET turns=NULL, native_accounting_version=1 WHERE id=? AND scope_id=? AND `+nativeUsageAccountingPendingSQL), ref.ID, scope)
	if err != nil {
		return nil, err
	}
	after, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
	if err != nil {
		return nil, err
	}
	change, err := r.recordUsageMutationTx(ctx, tx, nativeUsageAccountingRepairOperation, &before, &after)
	return &change, err
}

// markNativeUsageAccountingCurrentTx is for newly inserted native rows after
// all canonical/public accounting call sites use the current normalization.
func (r *Repository) markNativeUsageAccountingCurrentTx(ctx context.Context, tx *sqlx.Tx, table, id string) error {
	if table != usageMeasurementsTable && table != usageBucketsTable {
		return errors.New("invalid native accounting repair representation")
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET native_accounting_version=1 WHERE id=? AND source='native'`), id)
	return err
}

func (r *Repository) nativeUsageAccountingRepairPendingTx(ctx context.Context, tx *sqlx.Tx, scope string) (bool, error) {
	var pending bool
	query := `SELECT EXISTS(SELECT 1 FROM session_usage_measurements WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL +
		`) OR EXISTS(SELECT 1 FROM session_usage_buckets WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + `)`
	err := tx.GetContext(ctx, &pending, r.db.Rebind(query), scope, scope)
	return pending, err
}

// suppressNativeUsageTurns is for source normalization before token provenance
// is split from cost provenance. An event/response is not a turn aggregate.
func suppressNativeUsageTurns(measurement *models.SessionUsageMeasurement) {
	if measurement.Source == usageSourceNative {
		measurement.Turns = nil
	}
}

// suppressNativeCanonicalTurns also handles native-token/external-cost facts
// whose shared Measurement.Source is mixed. TokenSource is authoritative.
func suppressNativeCanonicalTurns(fact *models.UsageCanonicalFact) {
	if fact.TokenSource != nil {
		if fact.TokenSource.Source == usageSourceNative {
			fact.Measurement.Turns = nil
		}
		return
	}
	suppressNativeUsageTurns(&fact.Measurement)
}

func suppressNativeSummaryTurns(summary *models.UsageDailySummary) {
	if summary.MetricKind == projectionMetricTokens && summary.Source == usageSourceNative {
		summary.Turns = nil
		summary.TurnsMissing = max(summary.TurnsMissing, summary.TokenCount)
	}
}

// nativeUsageTurnsMissingAggregateSQL masks old published summaries as well as
// contribution-backed ranges. Grouping with unknown native turns remains NULL;
// non-native Host amounts retain the normal missing-value contract.
func nativeUsageTurnsMissingAggregateSQL() string {
	return `SUM(CASE WHEN metric_kind='tokens' AND source='native' THEN token_count ELSE turns_missing END)`
}

func nativeUsageTurnsAggregateSQL() string {
	return `CASE WHEN ` + nativeUsageTurnsMissingAggregateSQL() + `=0 AND SUM(token_count)>0 THEN
		CAST(SUM(CASE WHEN metric_kind='tokens' AND source='native' THEN NULL ELSE turns END) AS BIGINT) END`
}
