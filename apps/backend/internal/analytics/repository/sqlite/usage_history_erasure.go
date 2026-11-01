package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) EraseWorkspaceUsageTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	if workspaceID == "" {
		return errors.New("workspace usage erasure requires a workspace id")
	}
	var organizationID string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COALESCE(org_id, '') FROM workspaces WHERE id = ?`), workspaceID).Scan(&organizationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := r.beginScopeErasureTx(ctx, tx, "workspace", workspaceID); err != nil {
		return err
	}
	if err := r.eraseUsageForWorkspaceTx(ctx, tx, workspaceID); err != nil {
		return err
	}
	if err := r.finishScopeErasureTx(ctx, tx, "workspace", workspaceID); err != nil {
		return err
	}
	// The organization lifecycle erases captured organization scopes separately.
	return nil
}

// ResetWorkspaceUsageHistory clears retained rows for a reused E2E workspace.
// It advances the same durable fence generation as erasure, then reopens the
// workspace scope only after every row and checkpoint is removed.
func (r *Repository) ResetWorkspaceUsageHistory(ctx context.Context, workspaceID string) error {
	if workspaceID == "" {
		return errors.New("workspace usage reset requires a workspace id")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockUsageScopeFenceTx(ctx, tx, "workspace", workspaceID); err != nil {
		return err
	}
	query := `SELECT state FROM usage_scope_fences WHERE fence_kind = 'workspace' AND fence_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	var state string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), workspaceID).Scan(&state); err != nil {
		return err
	}
	if state == usageFenceErasing {
		return errors.New("workspace usage erasure is already in progress")
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scope_fences SET state = ?, generation = generation + 1, updated_at = ?
		WHERE fence_kind = 'workspace' AND fence_id = ?`), usageFenceErasing, time.Now().UTC(), workspaceID); err != nil {
		return err
	}
	if err := r.eraseUsageForWorkspaceTx(ctx, tx, workspaceID); err != nil {
		return err
	}
	if err := r.resetUsageSourceGuardsForE2ETx(ctx, tx, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scope_fences SET state = ?, generation = generation + 1, updated_at = ?
		WHERE fence_kind = 'workspace' AND fence_id = ? AND state = ?`), usageFenceActive, time.Now().UTC(), workspaceID, usageFenceErasing); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) EraseOrganizationUsage(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return errors.New("organization usage erasure requires an organization id")
	}
	if err := r.BeginOrganizationUsageErasure(ctx, organizationID); err != nil {
		return err
	}
	for {
		done, err := r.eraseOrganizationUsageBatch(ctx, organizationID, usageHistoryBatchLimit)
		if err != nil {
			return err
		}
		if done {
			break
		}
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.finishScopeErasureTx(ctx, tx, "organization", organizationID); err != nil {
		return err
	}
	return tx.Commit()
}

// BeginOrganizationUsageErasure durably fences retained binding admission and
// backfill before organization-owned workspace cleanup starts.
func (r *Repository) BeginOrganizationUsageErasure(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return errors.New("organization usage erasure requires an organization id")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.beginScopeErasureTx(ctx, tx, "organization", organizationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) beginScopeErasureTx(ctx context.Context, tx *sqlx.Tx, kind, id string) error {
	if err := r.lockUsageScopeFenceTx(ctx, tx, kind, id); err != nil {
		return err
	}
	var state string
	query := `SELECT state FROM usage_scope_fences WHERE fence_kind = ? AND fence_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), kind, id).Scan(&state); err != nil {
		return err
	}
	if state == usageFenceErased {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scope_fences SET state = ?, generation = generation + 1, updated_at = ?
		WHERE fence_kind = ? AND fence_id = ?`), usageFenceErasing, time.Now().UTC(), kind, id)
	return err
}

func (r *Repository) finishScopeErasureTx(ctx context.Context, tx *sqlx.Tx, kind, id string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scope_fences SET state = ?, generation = generation + 1, updated_at = ?
		WHERE fence_kind = ? AND fence_id = ? AND state = ?`), usageFenceErased, time.Now().UTC(), kind, id, usageFenceErasing)
	return err
}

func (r *Repository) eraseUsageForWorkspaceTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	scopeFilter := `scope_id IN (SELECT scope_id FROM usage_scopes WHERE workspace_id = ?)`
	args := []any{workspaceID}
	if err := r.deleteNativeLedgerForWorkspaceTx(ctx, tx, workspaceID); err != nil {
		return err
	}
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+table+` WHERE workspace_id = ? OR `+scopeFilter), append([]any{workspaceID}, args...)...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_live_links WHERE `+scopeFilter), args...); err != nil {
		return err
	}
	if err := r.deleteUsageScopeChildrenTx(ctx, tx, scopeFilter, args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_cursors WHERE fence_id IN (SELECT fence_id FROM usage_preservation_fences WHERE workspace_id = ?)`), workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_fences WHERE workspace_id = ?`), workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_history_migration_gaps WHERE workspace_id = ?`), workspaceID); err != nil && !db.IsMissingTableError(err) {
		return err
	}
	return nil
}

func (r *Repository) eraseOrganizationUsageBatch(ctx context.Context, organizationID string, limit int) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockUsageScopeFenceTx(ctx, tx, "organization", organizationID); err != nil {
		return false, err
	}
	var state string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_scope_fences WHERE fence_kind = 'organization' AND fence_id = ?`), organizationID).Scan(&state); err != nil {
		return false, err
	}
	if state != usageFenceErasing {
		return state == usageFenceErased, tx.Commit()
	}
	guarded, err := r.preserveOrganizationUsageSourcesBatchTx(ctx, tx, organizationID, limit)
	if err != nil {
		return false, err
	}
	if !guarded {
		return false, tx.Commit()
	}
	scopeFilter := `scope_id IN (SELECT scope_id FROM usage_scopes WHERE organization_id = ?)`
	nativeDeleted, err := r.deleteNativeLedgerForOrganizationBatchTx(ctx, tx, scopeFilter, limit, organizationID)
	if err != nil {
		return false, err
	}
	if nativeDeleted == limit {
		return false, tx.Commit()
	}
	done, err := r.eraseOrganizationExternalUsageBatchTx(ctx, tx, scopeFilter, organizationID, limit)
	if err != nil {
		return false, err
	}
	if !done {
		return false, tx.Commit()
	}
	done, err = r.eraseUsageProjectionBatchTx(ctx, tx, scopeFilter, limit, organizationID)
	if err != nil {
		return false, err
	}
	if !done {
		return false, tx.Commit()
	}
	if err := r.eraseOrganizationUsageMetadataTx(ctx, tx, scopeFilter, organizationID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) eraseOrganizationExternalUsageBatchTx(ctx context.Context, tx *sqlx.Tx, scopeFilter, organizationID string, limit int) (bool, error) {
	legacyOrgFilter := `workspace_id IN (SELECT id FROM workspaces WHERE org_id = ?)`
	externalFilter := `(` + scopeFilter + `) OR (` + legacyOrgFilter + `)`
	externalArgs := []any{organizationID, organizationID}
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		ids, err := r.usageRowIDsByScopeTx(ctx, tx, table, externalFilter, limit, externalArgs...)
		if err != nil {
			return false, err
		}
		if len(ids) > 0 {
			if err := r.deleteUsageRowsByIDTx(ctx, tx, table, ids); err != nil {
				return false, err
			}
			if err := r.deleteUsageLiveLinksBySourceIDsTx(ctx, tx, table, ids); err != nil {
				return false, err
			}
		}
		if len(ids) == limit {
			return false, nil
		}
	}
	return true, nil
}

func (r *Repository) eraseOrganizationUsageMetadataTx(ctx context.Context, tx *sqlx.Tx, scopeFilter, organizationID string) error {
	if err := r.deleteUsageScopeChildrenTx(ctx, tx, scopeFilter, organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_cursors WHERE fence_id IN (SELECT fence_id FROM usage_preservation_fences WHERE organization_id = ?)`), organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_fences WHERE organization_id = ?`), organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_history_migration_gaps WHERE organization_id = ?`), organizationID); err != nil && !db.IsMissingTableError(err) {
		return err
	}
	return nil
}

func (r *Repository) deleteNativeLedgerForWorkspaceBatchTx(ctx context.Context, tx *sqlx.Tx, workspace string, limit int) (int, error) {
	var ids []string
	query := `SELECT e.usage_event_id FROM task_usage_events e WHERE EXISTS (
		SELECT 1 FROM session_usage_buckets b WHERE b.source = 'native' AND b.source_record_id = e.usage_event_id AND b.workspace_id = ?
	) OR EXISTS (SELECT 1 FROM usage_execution_bindings ub WHERE ub.binding_id = e.usage_binding_id AND ub.workspace_id = ?)
	OR EXISTS (SELECT 1 FROM usage_legacy_adoptions a WHERE a.task_id=e.task_id AND e.id<=a.native_upper_bound AND a.workspace_id = ?)
	OR EXISTS (SELECT 1 FROM usage_history_migration_gaps g WHERE g.source_key='native' AND g.workspace_id = ? AND g.task_id=e.task_id
		AND (g.record_key=e.usage_event_id OR g.record_key=CAST(e.id AS TEXT)))
	OR EXISTS (SELECT 1 FROM tasks t WHERE t.id=e.task_id AND t.workspace_id = ?) ORDER BY e.id LIMIT ?`
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(query), workspace, workspace, workspace, workspace, workspace, limit); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_usage_events WHERE usage_event_id IN (`+placeholders(len(ids))+`)`), stringArgs(ids)...)
	return len(ids), err
}

func (r *Repository) deleteNativeLedgerForOrganizationBatchTx(ctx context.Context, tx *sqlx.Tx, scopeFilter string, limit int, organizationID string) (int, error) {
	var ids []string
	query := `SELECT e.usage_event_id FROM task_usage_events e WHERE EXISTS (
		SELECT 1 FROM session_usage_buckets b WHERE b.source = 'native' AND b.source_record_id = e.usage_event_id AND ` + scopeFilter + `
	) OR EXISTS (SELECT 1 FROM usage_execution_bindings ub WHERE ub.binding_id = e.usage_binding_id AND ` + scopeFilter + `)
	OR EXISTS (SELECT 1 FROM usage_legacy_adoptions a JOIN tasks t ON t.id=a.task_id AND t.created_at=a.task_created_at WHERE a.task_id=e.task_id AND e.id<=a.native_upper_bound AND ` + scopeFilter + `)
	OR EXISTS (SELECT 1 FROM usage_history_migration_gaps g WHERE g.source_key='native' AND g.organization_id = ? AND g.task_id=e.task_id
		AND (g.record_key=e.usage_event_id OR g.record_key=CAST(e.id AS TEXT)))
	OR EXISTS (SELECT 1 FROM tasks t JOIN workspaces w ON w.id = t.workspace_id WHERE t.id = e.task_id AND w.org_id = ?)
	ORDER BY e.id LIMIT ?`
	args := []any{organizationID, organizationID, organizationID, organizationID, organizationID, limit}
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(query), args...); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_usage_events WHERE usage_event_id IN (`+placeholders(len(ids))+`)`), stringArgs(ids)...)
	return len(ids), err
}

func (r *Repository) deleteNativeLedgerForWorkspaceTx(ctx context.Context, tx *sqlx.Tx, workspace string) error {
	for {
		deleted, err := r.deleteNativeLedgerForWorkspaceBatchTx(ctx, tx, workspace, usageHistoryBatchLimit)
		if err != nil {
			return err
		}
		if deleted < usageHistoryBatchLimit {
			return nil
		}
	}
}

func (r *Repository) usageRowIDsByScopeTx(ctx context.Context, tx *sqlx.Tx, table, scopeFilter string, limit int, args ...any) ([]string, error) {
	var ids []string
	query := `SELECT id FROM ` + table + ` WHERE ` + scopeFilter + ` ORDER BY id LIMIT ?`
	queryArgs := append(append([]any{}, args...), limit)
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(query), queryArgs...); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) deleteUsageRowsByIDTx(ctx context.Context, tx *sqlx.Tx, table string, ids []string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+table+` WHERE id IN (`+placeholders(len(ids))+`)`), stringArgs(ids)...)
	return err
}

func (r *Repository) deleteUsageLiveLinksBySourceIDsTx(ctx context.Context, tx *sqlx.Tx, table string, ids []string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_live_links WHERE source_table = ? AND observation_id IN (`+placeholders(len(ids))+`)`), append([]any{table}, stringArgs(ids)...)...)
	return err
}

func (r *Repository) deleteUsageScopeChildrenTx(ctx context.Context, tx *sqlx.Tx, scopeFilter string, args ...any) error {
	if err := r.preserveErasedUsageSourcesTx(ctx, tx, scopeFilter, args...); err != nil {
		return err
	}
	if err := r.deleteUsageProjectionScopesTx(ctx, tx, scopeFilter, args...); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM usage_collection_guard_erasure WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_current_targets WHERE ` + scopeFilter,
		`DELETE FROM usage_legacy_adoptions WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_contexts WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_preparation WHERE ` + scopeFilter,
		`DELETE FROM usage_work_identities WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_grants WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_snapshots WHERE ` + scopeFilter,
		`DELETE FROM usage_observation_sets WHERE ` + scopeFilter,
		`DELETE FROM usage_observation_items WHERE ` + scopeFilter,
		`DELETE FROM usage_collection_bindings WHERE ` + scopeFilter,
		`DELETE FROM usage_live_links WHERE ` + scopeFilter,
		`DELETE FROM usage_dirty_partitions WHERE ` + scopeFilter,
		`DELETE FROM usage_execution_bindings WHERE ` + scopeFilter,
		`DELETE FROM usage_subjects WHERE ` + scopeFilter,
		`DELETE FROM usage_scopes WHERE ` + scopeFilter,
	} {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(statement), args...); err != nil {
			return err
		}
	}
	return nil
}

func stringArgs(values []string) []any {
	args := make([]any, len(values))
	for i := range values {
		args[i] = values[i]
	}
	return args
}
