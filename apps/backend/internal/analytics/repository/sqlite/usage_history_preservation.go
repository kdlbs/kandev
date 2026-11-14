package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) beginUsagePreservation(ctx context.Context, taskID, sessionID string) (usagePreservationTarget, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return usagePreservationTarget{}, err
	}
	defer func() { _ = tx.Rollback() }()
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, sessionID, true)
	if err != nil {
		return usagePreservationTarget{}, err
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, target.WorkspaceID, target.OrganizationID); err != nil {
		return usagePreservationTarget{}, err
	}
	kind := usageWorkFallback
	if sessionID != "" {
		kind = usageSubjectSession
	}
	target.Kind = kind
	target.FenceID = usagePreservationID(kind, taskID, target.TaskCreatedAt, sessionID, target.SessionIncarnationID)
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_preservation_fences (
		fence_id, fence_kind, workspace_id, organization_id, task_id, task_created_at,
		session_id, session_incarnation_id, state, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(fence_id) DO NOTHING`),
		target.FenceID, target.Kind, target.WorkspaceID, target.OrganizationID, taskID, target.TaskCreatedAt,
		sessionID, target.SessionIncarnationID, usagePreserving, now, now)
	if err != nil {
		return usagePreservationTarget{}, fmt.Errorf("create retained usage preservation fence: %w", err)
	}
	var state string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_preservation_fences WHERE fence_id = ?`), target.FenceID).Scan(&state); err != nil {
		return usagePreservationTarget{}, fmt.Errorf("read retained usage preservation fence: %w", err)
	}
	if state == usagePreservationDone {
		return usagePreservationTarget{}, errors.New("usage preservation fence belongs to a completed deletion")
	}
	for _, source := range []string{usageSourceNative, "measurements", "buckets"} {
		highWater, highErr := r.usageSourceHighWaterTx(ctx, tx, source, taskID, sessionID)
		if highErr != nil {
			return usagePreservationTarget{}, highErr
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_preservation_cursors (
			fence_id, source_key, high_water, last_key, complete, updated_at
		) VALUES (?, ?, ?, '', 0, ?) ON CONFLICT(fence_id, source_key) DO NOTHING`), target.FenceID, source, highWater, now)
		if err != nil {
			return usagePreservationTarget{}, fmt.Errorf("capture %s usage preservation high-water: %w", source, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return usagePreservationTarget{}, err
	}
	return target, nil
}

func (r *Repository) runUsagePreservation(ctx context.Context, target usagePreservationTarget) error {
	for _, source := range []string{usageSourceNative, "measurements", "buckets"} {
		for {
			done, err := r.preserveUsageSourcePage(ctx, target, source, usageHistoryBatchLimit)
			if err != nil {
				return err
			}
			if done {
				break
			}
		}
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_preservation_fences SET state = ?, updated_at = ?
		WHERE fence_id = ? AND state = ?`), usagePreservationReady, time.Now().UTC(), target.FenceID, usagePreserving)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var state string
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_preservation_fences WHERE fence_id = ?`), target.FenceID).Scan(&state); err != nil {
			return err
		}
		if state != usagePreservationReady {
			return fmt.Errorf("usage preservation fence ended in state %q", state)
		}
	}
	return tx.Commit()
}

func (r *Repository) preserveUsageSourcePage(ctx context.Context, target usagePreservationTarget, source string, limit int) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.readTaskUsageTargetTx(ctx, tx, target.TaskID, "", true)
	if err != nil {
		return false, err
	}
	if !current.TaskCreatedAt.Equal(target.TaskCreatedAt) {
		return false, errors.New("task incarnation changed during usage preservation")
	}
	cursor, err := r.readUsagePreservationCursorTx(ctx, tx, target.FenceID, source)
	if err != nil {
		return false, err
	}
	if cursor.Complete != 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	// The cursor names the last retained row in this bounded batch. Its copies
	// and checkpoint commit together, so a failed page leaves both unchanged.
	count, lastKey, err := r.preserveUsageSourceBatchTx(ctx, tx, target, source, cursor, limit)
	if err != nil {
		return false, err
	}
	complete := count < limit
	if err := r.advanceUsagePreservationCursorTx(ctx, tx, target.FenceID, source, lastKey, complete); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return complete, nil
}

func (r *Repository) readUsagePreservationCursorTx(ctx context.Context, tx *sqlx.Tx, fenceID, source string) (usagePreservationCursor, error) {
	var cursor usagePreservationCursor
	query := `SELECT source_key, high_water, last_key, complete FROM usage_preservation_cursors WHERE fence_id = ? AND source_key = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	err := tx.GetContext(ctx, &cursor, r.db.Rebind(query), fenceID, source)
	return cursor, err
}

func (r *Repository) preserveUsageSourceBatchTx(ctx context.Context, tx *sqlx.Tx, target usagePreservationTarget, source string, cursor usagePreservationCursor, limit int) (int, string, error) {
	switch source {
	case usageSourceNative:
		return r.preserveNativeUsagePageTx(ctx, tx, target, cursor, limit)
	case "measurements", "buckets":
		return r.preserveExternalUsagePageTx(ctx, tx, target, source, cursor, limit)
	default:
		return 0, cursor.LastKey, fmt.Errorf("unsupported usage preservation source %q", source)
	}
}

func (r *Repository) advanceUsagePreservationCursorTx(ctx context.Context, tx *sqlx.Tx, fenceID, source, lastKey string, complete bool) error {
	var err error
	if complete {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_preservation_cursors SET last_key = ?, complete = 1, updated_at = ? WHERE fence_id = ? AND source_key = ?`), lastKey, time.Now().UTC(), fenceID, source)
	} else {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_preservation_cursors SET last_key = ?, updated_at = ? WHERE fence_id = ? AND source_key = ?`), lastKey, time.Now().UTC(), fenceID, source)
	}
	return err
}

func (r *Repository) preserveNativeUsagePageTx(ctx context.Context, tx *sqlx.Tx, target usagePreservationTarget, cursor usagePreservationCursor, limit int) (int, string, error) {
	high, err := strconv.ParseInt(cursor.HighWater, 10, 64)
	if err != nil {
		return 0, cursor.LastKey, fmt.Errorf("invalid native usage high-water %q: %w", cursor.HighWater, err)
	}
	last := int64(0)
	if cursor.LastKey != "" {
		last, err = strconv.ParseInt(cursor.LastKey, 10, 64)
		if err != nil {
			return 0, cursor.LastKey, fmt.Errorf("invalid native usage cursor %q: %w", cursor.LastKey, err)
		}
	}
	query := `SELECT e.id, e.usage_event_id, e.task_id, COALESCE(e.session_id, '') AS session_id, COALESCE(e.turn_id, '') AS turn_id,
		COALESCE(e.usage_binding_id, '') AS usage_binding_id, e.agent_type, e.model, e.provider, e.provider_thread_id,
		e.provider_turn_id, e.provider_response_id, e.native_scope, e.measurement_source,
		e.usage_completeness, e.usage_schema_version, e.tokens_in, e.tokens_cached_read,
		e.tokens_cached_write, e.tokens_out, e.tokens_thought, e.reasoning_output_tokens,
		e.reported_cache_write_tokens, e.reported_total_tokens, e.tokens_total, e.cost_subcents,
		e.cost_source, e.estimated, e.pricing_catalog_version, e.contract_version, e.occurred_at, e.created_at
		FROM task_usage_events e WHERE e.task_id = ? AND e.id > ? AND e.id <= ?` + sessionPredicateFor("e.session_id", target.SessionID) + ` ORDER BY e.id ASC LIMIT ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	args := []any{target.TaskID, last, high}
	if target.SessionID != "" {
		args = append(args, target.SessionID)
	}
	args = append(args, limit)
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, cursor.LastKey, err
	}
	batch := make([]legacyNativeUsageRow, 0, limit)
	for rows.Next() {
		var row legacyNativeUsageRow
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return 0, cursor.LastKey, err
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, cursor.LastKey, err
	}
	if err := rows.Close(); err != nil {
		return 0, cursor.LastKey, err
	}
	lastKey := cursor.LastKey
	for _, row := range batch {
		if err := r.preserveNativeUsageRowTx(ctx, tx, target, row.native()); err != nil {
			return len(batch), cursor.LastKey, fmt.Errorf("retain legacy native usage event %q: %w", row.UsageEventID, err)
		}
		lastKey = strconv.FormatInt(row.ID, 10)
	}
	return len(batch), lastKey, nil
}

type legacyNativeUsageRow struct {
	ID                    int64          `db:"id"`
	UsageEventID          string         `db:"usage_event_id"`
	TaskID                string         `db:"task_id"`
	SessionID             string         `db:"session_id"`
	TurnID                string         `db:"turn_id"`
	BindingID             string         `db:"usage_binding_id"`
	AgentType             string         `db:"agent_type"`
	Model                 string         `db:"model"`
	Provider              string         `db:"provider"`
	ProviderThreadID      string         `db:"provider_thread_id"`
	ProviderTurnID        string         `db:"provider_turn_id"`
	ProviderResponseID    string         `db:"provider_response_id"`
	NativeScope           string         `db:"native_scope"`
	MeasurementSource     string         `db:"measurement_source"`
	UsageCompleteness     string         `db:"usage_completeness"`
	UsageSchemaVersion    int            `db:"usage_schema_version"`
	InputTokens           int64          `db:"tokens_in"`
	CacheReadTokens       sql.NullInt64  `db:"tokens_cached_read"`
	CacheWriteTokens      sql.NullInt64  `db:"tokens_cached_write"`
	OutputTokens          sql.NullInt64  `db:"tokens_out"`
	ThoughtTokens         sql.NullInt64  `db:"tokens_thought"`
	ReasoningOutputTokens sql.NullInt64  `db:"reasoning_output_tokens"`
	ReportedCacheWrite    sql.NullInt64  `db:"reported_cache_write_tokens"`
	ReportedTotal         sql.NullInt64  `db:"reported_total_tokens"`
	TotalTokens           int64          `db:"tokens_total"`
	CostSubcents          int64          `db:"cost_subcents"`
	CostSource            string         `db:"cost_source"`
	Estimated             int            `db:"estimated"`
	PricingCatalogVersion sql.NullString `db:"pricing_catalog_version"`
	ContractVersion       int            `db:"contract_version"`
	OccurredAt            time.Time      `db:"occurred_at"`
	CollectedAt           time.Time      `db:"created_at"`
}

func (row legacyNativeUsageRow) native() usagehistory.NativeUsage {
	var costBasis string
	costPresent := false
	switch row.CostSource {
	case usageCostSourceReported:
		costBasis, costPresent = models.UsageCostBasisReported, true
	case usageCostSourceEstimated:
		costBasis, costPresent = models.UsageCostBasisEstimated, true
	}
	var bindingID string
	if row.BindingID != "" {
		bindingID = row.BindingID
	}
	return usagehistory.NativeUsage{
		BindingID: bindingID, UsageEventID: row.UsageEventID, TaskID: row.TaskID,
		SessionID: row.SessionID, TurnID: row.TurnID, AgentClient: row.AgentType,
		Model: row.Model, Provider: row.Provider, ProviderThreadID: row.ProviderThreadID,
		ProviderTurnID: row.ProviderTurnID, ProviderResponseID: row.ProviderResponseID,
		NativeScope: row.NativeScope, MeasurementSource: row.MeasurementSource,
		UsageCompleteness: row.UsageCompleteness, UsageSchemaVersion: row.UsageSchemaVersion,
		ContractVersion: row.ContractVersion, InputTokens: row.InputTokens,
		ThoughtTokens: nullableIntPtr(row.ThoughtTokens), CacheReadTokens: nullableIntPtr(row.CacheReadTokens),
		CacheWriteTokens: nullableIntPtr(row.CacheWriteTokens), OutputTokens: nullableIntPtr(row.OutputTokens),
		ReasoningOutputTokens:    nullableIntPtr(row.ReasoningOutputTokens),
		ReportedCacheWriteTokens: nullableIntPtr(row.ReportedCacheWrite), ReportedTotalTokens: nullableIntPtr(row.ReportedTotal),
		TotalTokens: row.TotalTokens, CostSubcents: row.CostSubcents, CostPresent: costPresent,
		CostBasis: costBasis, CostSource: row.CostSource, Estimated: row.Estimated != 0,
		PricingCatalogVersion: row.PricingCatalogVersion.String, TokenSemanticsVersion: row.UsageSchemaVersion,
		ReasoningInsideOutput: row.ReasoningOutputTokens.Valid, OccurredAt: row.OccurredAt, CollectedAt: row.CollectedAt,
	}
}

func (r *Repository) preserveExternalUsagePageTx(ctx context.Context, tx *sqlx.Tx, target usagePreservationTarget, source string, cursor usagePreservationCursor, limit int) (int, string, error) {
	table := "session_usage_" + source
	expr := dialect.ByteOrderedText(r.db.DriverName(), "id")
	query := `SELECT id, workspace_id, task_id, COALESCE(session_id, '') AS session_id, scope_id, subject_id,
		actor_id, actor_status, actor_kind, usage_binding_id, COALESCE(coverage_key, '') AS coverage_key,
		CAST(observed_at AS TEXT) AS observed_at, CAST(collected_at AS TEXT) AS collected_at
		FROM ` + table + ` WHERE task_id = ? AND ` + expr + ` > ? AND ` + expr + ` <= ?` + sessionPredicateFor(cursorSessionColumn(table), target.SessionID) + ` ORDER BY ` + expr + ` ASC LIMIT ?`
	args := []any{target.TaskID, cursor.LastKey, cursor.HighWater}
	if target.SessionID != "" {
		args = append(args, target.SessionID)
	}
	args = append(args, limit)
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, cursor.LastKey, err
	}
	batch := make([]legacyExternalUsageRow, 0, limit)
	for rows.Next() {
		var row legacyExternalUsageRow
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return 0, cursor.LastKey, err
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, cursor.LastKey, err
	}
	if err := rows.Close(); err != nil {
		return 0, cursor.LastKey, err
	}
	lastKey := cursor.LastKey
	for _, row := range batch {
		if err := r.retainLegacyExternalUsageTx(ctx, tx, table, row, target); err != nil {
			return len(batch), cursor.LastKey, fmt.Errorf("retain legacy external usage %s/%s: %w", table, row.ID, err)
		}
		lastKey = row.ID
	}
	return len(batch), lastKey, nil
}

func cursorSessionColumn(table string) string {
	return "session_id"
}

func sessionPredicate(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return " AND session_id = ?"
}

func sessionPredicateFor(column, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return " AND " + column + " = ?"
}

func (r *Repository) usageSourceHighWaterTx(ctx context.Context, tx *sqlx.Tx, source, taskID, sessionID string) (string, error) {
	if source == usageSourceNative {
		query := `SELECT COALESCE(MAX(id), 0) FROM task_usage_events WHERE task_id = ?` + sessionPredicate(sessionID)
		args := []any{taskID}
		if sessionID != "" {
			args = append(args, sessionID)
		}
		var high int64
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(&high); err != nil {
			return "", err
		}
		return strconv.FormatInt(high, 10), nil
	}
	table := "session_usage_" + source
	expr := dialect.ByteOrderedText(r.db.DriverName(), "id")
	query := `SELECT COALESCE(MAX(` + expr + `), '') FROM ` + table + ` WHERE task_id = ?` + sessionPredicateFor("session_id", sessionID)
	args := []any{taskID}
	if sessionID != "" {
		args = append(args, sessionID)
	}
	var high string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(&high); err != nil {
		return "", err
	}
	return high, nil
}
