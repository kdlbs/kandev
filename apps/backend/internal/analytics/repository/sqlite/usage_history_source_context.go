package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

const (
	usageSourceEligible = "eligible"
	usageSourceMixed    = "mixed"
	usageSourceUnknown  = "unknown"
)

// CaptureUsageCollectionSessionTx captures ownership at session creation. Later
// discovery can refresh source metadata but cannot manufacture this evidence.
func (r *Repository) CaptureUsageCollectionSessionTx(ctx context.Context, tx *sqlx.Tx, task, session string) error {
	var workspace, organization string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.workspace_id,COALESCE(w.org_id,'') FROM tasks t
		JOIN workspaces w ON w.id=t.workspace_id WHERE t.id = ?`), task).Scan(&workspace, &organization); err != nil {
		return err
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, workspace, organization); err != nil {
		return err
	}
	item, err := r.bindExternalUsageScopeTx(ctx, tx, models.SessionUsageMeasurement{WorkspaceID: workspace, TaskID: task, SessionID: session, CollectedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if item.ScopeID != usageScopeID(workspace, organization) {
		return ErrUsageNotVisible
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE subject_id = ? ORDER BY sequence DESC LIMIT 1`), item.SubjectID); err != nil {
		return err
	}
	if _, err := r.refreshUsageSourceContextTx(ctx, tx, target, true); err != nil {
		return err
	}
	return r.captureCollectionTargetTx(ctx, tx, item.SubjectID)
}

func sourceContextKey(target retainedTarget) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-source-context:"+target.WorkspaceID+":"+target.TaskID+":"+target.TaskIncarnationID+":"+target.SessionID+":"+target.SessionIncarnationID)).String()
}

func (r *Repository) refreshUsageSourceContextTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, created bool) (string, error) {
	unsafe, err := r.usageSourceScopeUnsafeTx(ctx, tx, target)
	if err != nil {
		return "", err
	}
	key := sourceContextKey(target)
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_contexts (context_id,scope_id,workspace_id,trusted,mixed)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(context_id) DO UPDATE SET
		mixed = CASE WHEN usage_collection_contexts.mixed = 1 OR usage_collection_contexts.scope_id <> excluded.scope_id OR excluded.mixed = 1 THEN 1 ELSE 0 END,
		trusted = CASE WHEN usage_collection_contexts.trusted = 1 OR excluded.trusted = 1 THEN 1 ELSE 0 END, scope_id = excluded.scope_id`),
		key, target.ScopeID, target.WorkspaceID, boolInt(created), boolInt(unsafe))
	if err != nil {
		return "", err
	}
	var trusted, mixed int
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT trusted,mixed FROM usage_collection_contexts WHERE context_id = ?`), key).Scan(&trusted, &mixed); err != nil {
		return "", err
	}
	if mixed != 0 {
		return usageSourceMixed, nil
	}
	if trusted == 0 {
		return usageSourceUnknown, nil
	}
	return usageSourceEligible, nil
}

func (r *Repository) admitUsageSourceContextTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, event string) error {
	if event != "" {
		return nil
	}
	status, err := r.refreshUsageSourceContextTx(ctx, tx, target, false)
	if err != nil {
		return err
	}
	if status != usageSourceEligible {
		return errUsageScopeUnproven
	}
	return nil
}

func (r *Repository) admitLegacyUsageSourceTx(ctx context.Context, tx *sqlx.Tx, m *models.SessionUsageMeasurement) error {
	if m.SessionID == "" {
		return nil
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE subject_id = ? ORDER BY sequence DESC LIMIT 1`), m.SubjectID); err != nil {
		return err
	}
	if err := r.admitUsageSourceContextTx(ctx, tx, target, ""); err != nil {
		return err
	}
	return r.deriveExternalUsageAttributionTx(ctx, tx, target, "", m)
}

func (r *Repository) prepareUsageCollectionTargetsTx(ctx context.Context, tx *sqlx.Tx, scope, workspace string, limit int) (bool, error) {
	var org string
	if err := tx.GetContext(ctx, &org, r.db.Rebind(`SELECT COALESCE(org_id,'') FROM workspaces WHERE id = ?`), workspace); err != nil {
		return false, err
	}
	if err := ensureUsageScopeTx(ctx, tx, r.db, scope, workspace, org, time.Now().UTC()); err != nil {
		return false, err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_preparation (scope_id,last_key) VALUES (?, '') ON CONFLICT(scope_id) DO NOTHING`), scope)
	if err != nil {
		return false, err
	}
	query := `SELECT last_key FROM usage_collection_preparation WHERE scope_id = ?`
	if r.db.DriverName() == usageDriverPostgres {
		query += usageForUpdate
	}
	var last string
	if err := tx.GetContext(ctx, &last, r.db.Rebind(query), scope); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	count, next, err := r.captureUsageTargetPageTx(ctx, tx, workspace, "", "", last, limit)
	if err != nil {
		return false, err
	}
	ready := count < limit
	if ready {
		next = ""
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_collection_preparation SET last_key = ? WHERE scope_id = ?`), next, scope)
	return ready, err
}
