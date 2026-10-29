package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db/dialect"
)

var _ usagehistory.Migrator = (*Repository)(nil)

var usageHistoryMigrationSources = []string{usageSourceNative, "measurements", "buckets"}

// BackfillUsageHistory commits at most limit source rows in one transaction.
// Its high-water and keyset cursor advance atomically with retained copies so
// startup can safely resume after any process or database interruption.
func (r *Repository) BackfillUsageHistory(ctx context.Context, limit int) (bool, error) {
	if limit < 1 {
		return false, errors.New("usage history migration batch limit must be positive")
	}
	if limit > usageHistoryBatchLimit {
		limit = usageHistoryBatchLimit
	}
	for _, source := range usageHistoryMigrationSources {
		done, err := r.backfillUsageHistorySourcePage(ctx, source, limit)
		if err != nil {
			return false, err
		}
		if !done {
			return false, nil
		}
	}
	done, err := r.BackfillUsageSelection(ctx, limit)
	if err != nil || !done {
		return done, err
	}
	return r.repairNativeUsageAccountingBatch(ctx, limit)
}

func (r *Repository) backfillUsageHistorySourcePage(ctx context.Context, source string, limit int) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	cursor, err := r.ensureUsageMigrationCursorTx(ctx, tx, source)
	if err != nil {
		return false, err
	}
	if cursor.Complete != 0 {
		return true, tx.Commit()
	}

	count, lastKey, err := r.backfillUsageSourcePageTx(ctx, tx, source, cursor, limit)
	if err != nil {
		return false, err
	}
	complete := count < limit
	if source == usageSourceNative && lastKey == "" {
		lastKey = cursor.LastKey
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_history_migration_state
		SET last_key = ?, complete = ?, updated_at = ? WHERE source_key = ?`), lastKey, boolInt(complete), time.Now().UTC(), source)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return complete, nil
}

func (r *Repository) ensureUsageMigrationCursorTx(ctx context.Context, tx *sqlx.Tx, source string) (usagePreservationCursor, error) {
	query := `SELECT source_key, high_water, last_key, complete FROM usage_history_migration_state WHERE source_key = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	var cursor usagePreservationCursor
	err := tx.GetContext(ctx, &cursor, r.db.Rebind(query), source)
	if err == nil {
		return cursor, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return cursor, err
	}
	highWater, err := r.globalUsageSourceHighWaterTx(ctx, tx, source)
	if err != nil {
		return cursor, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_history_migration_state
		(source_key, high_water, last_key, complete, updated_at) VALUES (?, ?, '', 0, ?)
		ON CONFLICT(source_key) DO NOTHING`), source, highWater, time.Now().UTC())
	if err != nil {
		return cursor, err
	}
	if err := tx.GetContext(ctx, &cursor, r.db.Rebind(query), source); err != nil {
		return cursor, err
	}
	return cursor, nil
}

func (r *Repository) globalUsageSourceHighWaterTx(ctx context.Context, tx *sqlx.Tx, source string) (string, error) {
	if source == usageSourceNative {
		var high int64
		if err := tx.QueryRowxContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM task_usage_events`).Scan(&high); err != nil {
			return "", err
		}
		return strconv.FormatInt(high, 10), nil
	}
	table := "session_usage_" + source
	expr := dialect.ByteOrderedText(r.db.DriverName(), "id")
	var high string
	if err := tx.QueryRowxContext(ctx, `SELECT COALESCE(MAX(`+expr+`), '') FROM `+table).Scan(&high); err != nil {
		return "", err
	}
	return high, nil
}

func (r *Repository) backfillUsageSourcePageTx(ctx context.Context, tx *sqlx.Tx, source string, cursor usagePreservationCursor, limit int) (int, string, error) {
	if source == usageSourceNative {
		return r.backfillNativeUsagePageTx(ctx, tx, cursor, limit)
	}
	return r.backfillExternalUsagePageTx(ctx, tx, source, cursor, limit)
}

func (r *Repository) backfillNativeUsagePageTx(ctx context.Context, tx *sqlx.Tx, cursor usagePreservationCursor, limit int) (int, string, error) {
	high, err := strconv.ParseInt(cursor.HighWater, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid native migration high-water %q: %w", cursor.HighWater, err)
	}
	last := int64(0)
	if cursor.LastKey != "" {
		last, err = strconv.ParseInt(cursor.LastKey, 10, 64)
		if err != nil {
			return 0, "", fmt.Errorf("invalid native migration cursor %q: %w", cursor.LastKey, err)
		}
	}
	query := `SELECT e.id, e.usage_event_id, e.task_id, COALESCE(e.session_id, '') AS session_id,
		COALESCE(e.turn_id, '') AS turn_id, COALESCE(e.usage_binding_id, '') AS usage_binding_id,
		e.agent_type, e.model, e.provider, e.provider_thread_id, e.provider_turn_id,
		e.provider_response_id, e.native_scope, e.measurement_source, e.usage_completeness,
		e.usage_schema_version, e.tokens_in, e.tokens_cached_read, e.tokens_cached_write,
		e.tokens_out, e.tokens_thought, e.reasoning_output_tokens, e.reported_cache_write_tokens,
		e.reported_total_tokens, e.tokens_total, e.cost_subcents, e.cost_source, e.estimated,
		e.pricing_catalog_version, e.contract_version, e.occurred_at, e.created_at
		FROM task_usage_events e WHERE e.id > ? AND e.id <= ? ORDER BY e.id ASC LIMIT ?`
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), last, high, limit)
	if err != nil {
		return 0, "", err
	}
	batch := make([]legacyNativeUsageRow, 0, limit)
	lastKey := cursor.LastKey
	for rows.Next() {
		var row legacyNativeUsageRow
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return 0, lastKey, err
		}
		lastKey = strconv.FormatInt(row.ID, 10)
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, lastKey, err
	}
	if err := rows.Close(); err != nil {
		return 0, lastKey, err
	}
	for _, row := range batch {
		if err := r.backfillNativeUsageRowTx(ctx, tx, row); err != nil {
			return len(batch), lastKey, err
		}
	}
	return len(batch), lastKey, nil
}

func (r *Repository) backfillNativeUsageRowTx(ctx context.Context, tx *sqlx.Tx, row legacyNativeUsageRow) error {
	if row.BindingID != "" {
		var bindingExists bool
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_execution_bindings WHERE binding_id = ?)`), row.BindingID).Scan(&bindingExists); err != nil {
			return err
		}
		if bindingExists {
			err := r.RetainNativeUsageTx(ctx, tx, row.native())
			if errors.Is(err, errUsageScopeErasing) {
				return nil
			}
			return err
		}
		row.BindingID = ""
	}
	native := row.native()
	if binding, err := r.savedNativeBindingTx(ctx, tx, native); err != nil {
		return err
	} else if binding != "" {
		native.BindingID = binding
		return r.RetainNativeUsageTx(ctx, tx, native)
	}
	target, err := r.readTaskUsageTargetTx(ctx, tx, row.TaskID, "", true)
	if errors.Is(err, sql.ErrNoRows) {
		return r.recordUsageMigrationGapTx(ctx, tx, usageSourceNative, strconv.FormatInt(row.ID, 10), row.TaskID, "", "", "task incarnation is no longer available")
	}
	if err != nil {
		return err
	}
	native.BindingID, err = r.legacyNativeBindingTx(ctx, tx, row, target)
	if errors.Is(err, errUsageScopeErasing) {
		return nil
	}
	if errors.Is(err, errUsageScopeUnproven) {
		return r.recordUsageMigrationGapTx(ctx, tx, usageSourceNative, strconv.FormatInt(row.ID, 10), row.TaskID, target.WorkspaceID, target.OrganizationID, errUsageScopeUnproven.Error())
	}
	if err != nil {
		return err
	}
	return r.RetainNativeUsageTx(ctx, tx, native)
}

func (r *Repository) backfillExternalUsagePageTx(ctx context.Context, tx *sqlx.Tx, source string, cursor usagePreservationCursor, limit int) (int, string, error) {
	table := "session_usage_" + source
	expr := dialect.ByteOrderedText(r.db.DriverName(), "id")
	query := `SELECT id, workspace_id, task_id, COALESCE(session_id, '') AS session_id,
		scope_id, subject_id, actor_id, actor_status, actor_kind, usage_binding_id,
		COALESCE(coverage_key, '') AS coverage_key, CAST(collected_at AS TEXT) AS collected_at
		FROM ` + table + ` WHERE ` + expr + ` > ? AND ` + expr + ` <= ? ORDER BY ` + expr + ` ASC LIMIT ?`
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), cursor.LastKey, cursor.HighWater, limit)
	if err != nil {
		return 0, "", err
	}
	batch := make([]legacyExternalUsageRow, 0, limit)
	lastKey := cursor.LastKey
	for rows.Next() {
		var row legacyExternalUsageRow
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return 0, lastKey, err
		}
		lastKey = row.ID
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, lastKey, err
	}
	if err := rows.Close(); err != nil {
		return 0, lastKey, err
	}
	for _, row := range batch {
		if err := r.backfillExternalUsageRowTx(ctx, tx, table, row); err != nil {
			return len(batch), lastKey, err
		}
	}
	return len(batch), lastKey, nil
}

func (r *Repository) backfillExternalUsageRowTx(ctx context.Context, tx *sqlx.Tx, table string, row legacyExternalUsageRow) error {
	if row.ScopeID != "" && row.SubjectID != "" {
		err := r.retainLegacyExternalUsageTx(ctx, tx, table, row, usagePreservationTarget{})
		if errors.Is(err, errUsageScopeErasing) {
			return nil
		}
		return err
	}
	target, err := r.readTaskUsageTargetTx(ctx, tx, row.TaskID, "", false)
	if errors.Is(err, sql.ErrNoRows) {
		return r.recordUsageMigrationGapTx(ctx, tx, table, row.ID, row.TaskID, row.WorkspaceID, "", "task incarnation is no longer available")
	}
	if err != nil {
		return err
	}
	if target.WorkspaceID != row.WorkspaceID {
		return r.recordUsageMigrationGapTx(ctx, tx, table, row.ID, row.TaskID, row.WorkspaceID, target.OrganizationID, "source workspace does not match the live task")
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, target.WorkspaceID, target.OrganizationID); err != nil {
		if errors.Is(err, errUsageScopeErasing) {
			return nil
		}
		return err
	}
	return r.retainLegacyExternalUsageTx(ctx, tx, table, row, target)
}

func (r *Repository) recordUsageMigrationGapTx(ctx context.Context, tx *sqlx.Tx, source, recordKey, taskID, workspaceID, organizationID, reason string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_history_migration_gaps
		(source_key, record_key, task_id, workspace_id, organization_id, reason, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(source_key, record_key) DO UPDATE SET
		reason = excluded.reason, detected_at = excluded.detected_at`),
		source, recordKey, taskID, workspaceID, organizationID, reason, time.Now().UTC())
	return err
}
