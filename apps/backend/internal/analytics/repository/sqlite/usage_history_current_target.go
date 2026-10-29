package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

type currentUsageTargetSelector pluginsdk.UsageCollectionCurrentTargetQueryV1

func (r *Repository) collectionQueryScopeTx(ctx context.Context, tx *sqlx.Tx, query pluginsdk.UsageCollectionTargetQueryV1) (string, error) {
	if query.CurrentTarget != nil && r.db.DriverName() == usageDriverPostgres {
		if !usagehistory.WorkspaceAllowed(ctx, query.WorkspaceID) {
			return "", ErrUsageNotVisible
		}
		var id string
		if err := tx.GetContext(ctx, &id, r.db.Rebind(`SELECT id FROM workspaces WHERE id = ? FOR UPDATE`), query.WorkspaceID); err != nil {
			return "", ErrUsageNotVisible
		}
	}
	return r.authorizedUsageScopeTx(ctx, tx, query.WorkspaceID)
}

func (r *Repository) listCurrentUsageTargetTx(ctx context.Context, tx *sqlx.Tx, source, scope string, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	page := pluginsdk.UsageCollectionTargetPageV1{CurrentTarget: true}
	if query.CurrentTarget == nil || query.SnapshotID != "" || query.Cursor != "" {
		return page, ErrUsageNotVisible
	}
	target, err := r.currentUsageTargetTx(ctx, tx, source, scope, query.WorkspaceID, currentUsageTargetSelector(*query.CurrentTarget))
	if err != nil {
		return page, err
	}
	if target != nil {
		page.Targets = []pluginsdk.UsageCollectionTargetV1{target.dto()}
	}
	return page, nil
}

func (r *Repository) currentUsageTargetTx(ctx context.Context, tx *sqlx.Tx, source, scope, workspace string, query currentUsageTargetSelector) (*retainedTarget, error) {
	if err := query.validate(); err != nil {
		return nil, err
	}
	if query.SubjectBinding != "" {
		return r.currentRetainedUsageTargetTx(ctx, tx, source, scope, workspace, query)
	}
	task, session, err := r.currentUsageSessionTx(ctx, tx, workspace, query)
	if err != nil || session == "" {
		return nil, err
	}
	item, err := r.bindExternalUsageScopeTx(ctx, tx, models.SessionUsageMeasurement{
		WorkspaceID: workspace, TaskID: task, SessionID: session, CollectedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings
		WHERE scope_id = ? AND subject_id = ? ORDER BY sequence DESC LIMIT 1`), scope, item.SubjectID); err != nil {
		return nil, err
	}
	if !query.matches(target) {
		return nil, nil
	}
	if err := r.revalidateCurrentUsageTargetTx(ctx, tx, &target); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_grants (source,binding_id,scope_id)
		VALUES (?, ?, ?) ON CONFLICT(source,binding_id) DO NOTHING`), source, target.BindingID, scope)
	return &target, err
}

func (r *Repository) refreshRetainedUsageTargetLiveContextTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget) error {
	live, err := r.usageTargetIncarnationIsLiveTx(ctx, tx, target)
	if err != nil || !live {
		return err
	}
	_, err = r.bindExternalUsageScopeTx(ctx, tx, models.SessionUsageMeasurement{
		WorkspaceID: target.WorkspaceID, TaskID: target.TaskID, SessionID: target.SessionID, CollectedAt: time.Now().UTC(),
	})
	return err
}

func (q currentUsageTargetSelector) validate() error {
	values := []string{q.TaskID, q.SessionID, q.ExternalSessionID, q.SubjectBinding}
	any := false
	for _, value := range values {
		if strings.TrimSpace(value) != value {
			return ErrUsageNotVisible
		}
		any = any || value != ""
	}
	if !any {
		return ErrUsageNotVisible
	}
	return nil
}

func (q currentUsageTargetSelector) matches(v retainedTarget) bool {
	return (q.TaskID == "" || q.TaskID == v.TaskID) &&
		(q.SessionID == "" || q.SessionID == v.SessionID) &&
		(q.ExternalSessionID == "" || q.ExternalSessionID == v.ExternalSessionID)
}

func (r *Repository) currentUsageSessionTx(ctx context.Context, tx *sqlx.Tx, workspace string, query currentUsageTargetSelector) (string, string, error) {
	if query.SessionID == "" && query.ExternalSessionID != "" {
		return r.currentTranscriptSessionTx(ctx, tx, workspace, query)
	}
	statement := `SELECT ts.task_id,ts.id FROM task_sessions ts JOIN tasks t ON t.id=ts.task_id WHERE t.workspace_id = ?`
	args := []any{workspace}
	if query.TaskID != "" {
		statement += ` AND ts.task_id = ?`
		args = append(args, query.TaskID)
	}
	if query.SessionID != "" {
		statement += ` AND ts.id = ?`
		args = append(args, query.SessionID)
	}
	var rows []struct {
		TaskID string `db:"task_id"`
		ID     string `db:"id"`
	}
	if err := tx.SelectContext(ctx, &rows, r.db.Rebind(statement+` LIMIT 2`), args...); err != nil {
		return "", "", err
	}
	if len(rows) != 1 {
		return "", "", nil
	}
	return rows[0].TaskID, rows[0].ID, nil
}

func (r *Repository) currentTranscriptSessionTx(ctx context.Context, tx *sqlx.Tx, workspace string, query currentUsageTargetSelector) (string, string, error) {
	rows, err := r.currentTranscriptLiveSessionsTx(ctx, tx, workspace, query)
	if err != nil || len(rows) != 1 {
		return "", "", err
	}
	return rows[0].TaskID, rows[0].SessionID, nil
}

func (r *Repository) usageTargetIncarnationIsLiveTx(ctx context.Context, tx *sqlx.Tx, v retainedTarget) (bool, error) {
	var created time.Time
	var incarnation, workspace string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.workspace_id,t.created_at,COALESCE(NULLIF(ts.queue_incarnation_id,''),ts.id)
		FROM task_sessions ts JOIN tasks t ON t.id=ts.task_id WHERE ts.id = ? AND ts.task_id = ?`), v.SessionID, v.TaskID).
		Scan(&workspace, &created, &incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && workspace == v.WorkspaceID && usageTaskIncarnationID(created) == v.TaskIncarnationID && incarnation == v.SessionIncarnationID, err
}

func (r *Repository) currentRetainedUsageTargetTx(ctx context.Context, tx *sqlx.Tx, source, scope, workspace string, query currentUsageTargetSelector) (*retainedTarget, error) {
	var target retainedTarget
	err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT b.* FROM usage_collection_bindings b
		JOIN usage_collection_grants g ON g.binding_id=b.binding_id WHERE b.binding_id = ? AND g.source = ?
		AND b.scope_id = ? AND b.workspace_id = ?`), query.SubjectBinding, source, scope, workspace)
	if err != nil {
		return nil, ErrUsageNotVisible
	}
	if !query.matches(target) {
		return nil, nil
	}
	if err := r.refreshRetainedUsageTargetLiveContextTx(ctx, tx, target); err != nil {
		return nil, err
	}
	if err := r.revalidateCurrentUsageTargetTx(ctx, tx, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) revalidateCurrentUsageTargetTx(ctx context.Context, tx *sqlx.Tx, target *retainedTarget) error {
	status, err := r.refreshUsageSourceContextTx(ctx, tx, *target, false)
	if err != nil {
		return err
	}
	target.SourceScopeStatus = status
	return r.captureTargetActorTx(ctx, tx, target)
}

func (r *Repository) indexCurrentUsageTargetTx(ctx context.Context, tx *sqlx.Tx, v retainedTarget) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_current_targets
		(scope_id,subject_id,binding_id,task_id,session_id,external_session_id) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_id,subject_id) DO UPDATE SET binding_id=excluded.binding_id,
		external_session_id=excluded.external_session_id`), v.ScopeID, v.SubjectID, v.BindingID, v.TaskID, v.SessionID, v.ExternalSessionID)
	return err
}
