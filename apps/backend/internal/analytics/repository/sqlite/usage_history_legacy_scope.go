package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
)

var errUsageScopeUnproven = errors.New("consumption scope is not proven")

func (r *Repository) savedNativeBindingTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) (string, error) {
	var binding string
	err := tx.GetContext(ctx, &binding, r.db.Rebind(`SELECT usage_binding_id FROM session_usage_buckets WHERE source='native' AND source_record_id=?`), native.UsageEventID)
	if err == nil && binding != "" {
		return binding, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if native.TurnID == "" {
		return "", nil
	}
	has, err := db.ColumnExistsContext(ctx, tx, "task_session_turns", "usage_binding_id")
	if err != nil || !has {
		return "", err
	}
	err = tx.GetContext(ctx, &binding, r.db.Rebind(`SELECT COALESCE(usage_binding_id,'') FROM task_session_turns WHERE id=? AND task_id=? AND task_session_id=?`), native.TurnID, native.TaskID, native.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return binding, err
}

func (r *Repository) adoptUnboundNativeTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) (string, error) {
	exists, err := db.TableExistsContext(ctx, tx, "task_usage_events")
	if err != nil || !exists {
		return "", err
	}
	hasIdentity, err := db.ColumnExistsContext(ctx, tx, "task_usage_events", "usage_event_id")
	if err != nil || !hasIdentity {
		return "", err
	}
	row := legacyNativeUsageRow{UsageEventID: native.UsageEventID, TaskID: native.TaskID, SessionID: native.SessionID, TurnID: native.TurnID, OccurredAt: native.OccurredAt, CollectedAt: native.CollectedAt}
	err = tx.GetContext(ctx, &row.ID, r.db.Rebind(`SELECT id FROM task_usage_events WHERE usage_event_id=? AND task_id=?`), native.UsageEventID, native.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	target, err := r.readTaskUsageTargetTx(ctx, tx, native.TaskID, "", true)
	if err != nil {
		return "", err
	}
	return r.legacyNativeBindingTx(ctx, tx, row, target)
}

func (r *Repository) recordUnprovenNativeScopeTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) error {
	target, err := r.readTaskUsageTargetTx(ctx, tx, native.TaskID, "", false)
	if err != nil {
		return err
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, target.WorkspaceID, target.OrganizationID); err != nil {
		if errors.Is(err, errUsageScopeErasing) {
			return nil
		}
		return err
	}
	return r.recordUsageMigrationGapTx(ctx, tx, usageSourceNative, native.UsageEventID, native.TaskID, target.WorkspaceID, target.OrganizationID, errUsageScopeUnproven.Error())
}

func (r *Repository) proveUnboundNativeScopeTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage, scope trustedUsageScope) error {
	var birth retainedTarget
	err := tx.GetContext(ctx, &birth, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE task_id=? AND task_incarnation_id=? AND session_id=? AND session_incarnation_id=? AND source_scope_status='eligible' ORDER BY sequence LIMIT 1`),
		native.TaskID, scope.TaskIncarnationID, scope.SessionID, scope.SessionIncarnationID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && birth.ScopeID != scope.ScopeID) {
		return errUsageScopeUnproven
	}
	if err != nil {
		return err
	}
	if birth.SessionStartedAt != nil && native.OccurredAt.Before(parseTimeString(*birth.SessionStartedAt)) {
		return errUsageScopeUnproven
	}
	return r.admitUsageSourceContextTx(ctx, tx, birth, "")
}

func (r *Repository) provenLegacyNativeScopeTx(ctx context.Context, tx *sqlx.Tx, row legacyNativeUsageRow, target usagePreservationTarget) (trustedUsageScope, error) {
	var scope trustedUsageScope
	var upper int64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT a.scope_id, s.organization_id, a.native_upper_bound FROM usage_legacy_adoptions a JOIN usage_scopes s ON s.scope_id=a.scope_id
		WHERE a.task_id=? AND a.task_created_at=? AND a.session_id=?`), row.TaskID, target.TaskCreatedAt, row.SessionID).Scan(&scope.ScopeID, &scope.OrganizationID, &upper)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return scope, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		upper, err = r.adoptLegacyNativeScopeTx(ctx, tx, row, target, &scope)
		if err != nil {
			return scope, err
		}
	}
	if row.ID > upper {
		if scope.ScopeID != usageScopeID(target.WorkspaceID, target.OrganizationID) {
			return scope, errUsageScopeUnproven
		}
		var high int64
		if err := tx.GetContext(ctx, &high, `SELECT COALESCE(MAX(id),0) FROM task_usage_events`); err != nil {
			return scope, err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_legacy_adoptions SET native_upper_bound=? WHERE task_id=? AND task_created_at=? AND session_id=?`), high, row.TaskID, target.TaskCreatedAt, row.SessionID); err != nil {
			return scope, err
		}
	}
	scope.WorkspaceID, scope.TaskIncarnationID = target.WorkspaceID, target.TaskIncarnationID
	var birth retainedTarget
	if err := tx.GetContext(ctx, &birth, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE scope_id=? AND task_id=? AND task_incarnation_id=? AND session_id=? AND source_scope_status='eligible' ORDER BY sequence LIMIT 1`), scope.ScopeID, row.TaskID, target.TaskIncarnationID, row.SessionID); err != nil {
		return scope, err
	}
	if scope.ScopeID == usageScopeID(target.WorkspaceID, target.OrganizationID) {
		if err := r.admitUsageSourceContextTx(ctx, tx, birth, ""); err != nil {
			return scope, errUsageScopeUnproven
		}
	}
	scope.SubjectID, scope.SessionID, scope.SessionIncarnationID = birth.SubjectID, birth.SessionID, birth.SessionIncarnationID
	return scope, r.ensureUsageScopeWritableTx(ctx, tx, scope.WorkspaceID, scope.OrganizationID)
}

func (r *Repository) adoptLegacyNativeScopeTx(ctx context.Context, tx *sqlx.Tx, row legacyNativeUsageRow, target usagePreservationTarget, scope *trustedUsageScope) (int64, error) {
	var birth retainedTarget
	err := tx.GetContext(ctx, &birth, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE task_id=? AND task_incarnation_id=? AND session_id=? AND source_scope_status='eligible' ORDER BY sequence LIMIT 1`), row.TaskID, target.TaskIncarnationID, row.SessionID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && birth.ScopeID != usageScopeID(target.WorkspaceID, target.OrganizationID)) {
		return 0, errUsageScopeUnproven
	}
	if err != nil {
		return 0, err
	}
	var high string
	err = tx.GetContext(ctx, &high, `SELECT high_water FROM usage_history_migration_state WHERE source_key='native'`)
	if errors.Is(err, sql.ErrNoRows) {
		high, err = r.globalUsageSourceHighWaterTx(ctx, tx, usageSourceNative)
	}
	if err != nil {
		return 0, err
	}
	upper, err := strconv.ParseInt(high, 10, 64)
	if err != nil {
		return 0, err
	}
	scope.ScopeID, scope.OrganizationID = birth.ScopeID, target.OrganizationID
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_legacy_adoptions (scope_id,workspace_id,task_id,task_created_at,session_id,native_upper_bound)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(task_id,task_created_at,session_id) DO NOTHING`), scope.ScopeID, target.WorkspaceID, row.TaskID, target.TaskCreatedAt, row.SessionID, upper)
	return upper, err
}

func (r *Repository) legacyNativeBindingTx(ctx context.Context, tx *sqlx.Tx, row legacyNativeUsageRow, target usagePreservationTarget) (string, error) {
	scope, err := r.provenLegacyNativeScopeTx(ctx, tx, row, target)
	if err != nil {
		return "", err
	}
	binding := usagehistory.Binding{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-legacy-adoption:"+scope.SubjectID+":"+row.UsageEventID)).String(),
		Kind: usagehistory.BindingKindTurn, WorkspaceID: scope.WorkspaceID, OrganizationID: scope.OrganizationID, TaskID: row.TaskID, TaskIncarnationID: scope.TaskIncarnationID,
		SessionID: scope.SessionID, SessionIncarnationID: scope.SessionIncarnationID, ActorStatus: usageUnknown, ActorKind: usageUnknown, ActorSource: "legacy_adoption", CapturedAt: time.Now().UTC()}
	binding.TurnID = row.TurnID
	if err := r.AdmitBindingTx(ctx, tx, binding); err != nil {
		return "", err
	}
	return binding.ID, nil
}
