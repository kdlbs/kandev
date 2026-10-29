package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	usageHistoryBatchLimit = 200
	usageFenceActive       = "active"
	usageFenceErasing      = "erasing"
	usageFenceErased       = "erased"
	usagePreserving        = "preserving"
	usagePreservationReady = "ready"
	usagePreservationDone  = "deleted"
)

var _ usagehistory.Lifecycle = (*Repository)(nil)

var errUsageScopeErasing = errors.New("usage scope erasure is in progress or complete")

type usagePreservationTarget struct {
	Kind                 string
	FenceID              string
	WorkspaceID          string
	OrganizationID       string
	TaskID               string
	TaskCreatedAt        time.Time
	TaskIncarnationID    string
	SessionID            string
	SessionIncarnationID string
}

type usagePreservationCursor struct {
	Source    string `db:"source_key"`
	HighWater string `db:"high_water"`
	LastKey   string `db:"last_key"`
	Complete  int    `db:"complete"`
}

// AdmitUsageWriteTx serializes an incoming source write with task/session
// preservation and captured-scope erasure in the same database transaction.
func (r *Repository) AdmitUsageWriteTx(ctx context.Context, tx *sqlx.Tx, taskID, workspaceID, sessionID string) error {
	if tx == nil {
		return errors.New("usage write admission requires a transaction")
	}
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, sessionID, true)
	if err != nil {
		return err
	}
	if workspaceID != "" && target.WorkspaceID != workspaceID {
		return fmt.Errorf("usage task %q is outside workspace %q", taskID, workspaceID)
	}
	if err := r.checkTaskPreservationFenceTx(ctx, tx, target.TaskID, target.TaskCreatedAt); err != nil {
		return err
	}
	if sessionID != "" {
		if err := r.checkSessionPreservationFenceTx(ctx, tx, target.TaskID, target.TaskCreatedAt, sessionID, target.SessionIncarnationID); err != nil {
			return err
		}
	}
	if err := r.checkManagedDeleteFenceTx(ctx, tx, taskID); err != nil {
		return err
	}
	return r.ensureUsageScopeWritableTx(ctx, tx, target.WorkspaceID, target.OrganizationID)
}

// PreserveTaskUsage captures source high-water marks under the task-row lock,
// then copies bounded keyset pages while its durable fence rejects new writes.
func (r *Repository) PreserveTaskUsage(ctx context.Context, taskID string) error {
	target, err := r.beginUsagePreservation(ctx, taskID, "")
	if err != nil {
		return err
	}
	if err := r.refreshUsageCollectionTargets(ctx, target.WorkspaceID, target.TaskID, target.SessionID); err != nil {
		return err
	}
	return r.runUsagePreservation(ctx, target)
}

// PreserveSessionUsage retains rows before pruning clears the session's live
// reference or removes turn/binding evidence.
func (r *Repository) PreserveSessionUsage(ctx context.Context, taskID, sessionID, incarnationID string) error {
	target, err := r.beginUsagePreservation(ctx, taskID, sessionID)
	if err != nil {
		return err
	}
	if target.SessionIncarnationID != incarnationID {
		_ = r.ReleaseSessionUsage(ctx, taskID, sessionID, target.SessionIncarnationID)
		return errors.New("session incarnation changed before usage preservation")
	}
	if err := r.refreshUsageCollectionTargets(ctx, target.WorkspaceID, target.TaskID, target.SessionID); err != nil {
		return err
	}
	return r.runUsagePreservation(ctx, target)
}

func (r *Repository) readTaskUsageTargetTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string, lock bool) (usagePreservationTarget, error) {
	target := usagePreservationTarget{TaskID: taskID, SessionID: sessionID}
	query := `SELECT t.workspace_id, COALESCE(w.org_id, ''), t.created_at FROM tasks t JOIN workspaces w ON w.id = t.workspace_id WHERE t.id = ?`
	if lock && dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF t`
	}
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), taskID).Scan(&target.WorkspaceID, &target.OrganizationID, &target.TaskCreatedAt)
	if err != nil {
		return target, fmt.Errorf("resolve usage preservation task %q: %w", taskID, err)
	}
	target.TaskIncarnationID = usageTaskIncarnationID(target.TaskCreatedAt)
	if sessionID != "" {
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COALESCE(NULLIF(queue_incarnation_id, ''), id) FROM task_sessions WHERE id = ? AND task_id = ?`), sessionID, taskID).Scan(&target.SessionIncarnationID)
		if err != nil {
			return target, fmt.Errorf("resolve usage preservation session %q: %w", sessionID, err)
		}
	}
	return target, nil
}

func usagePreservationID(kind, taskID string, taskCreatedAt time.Time, sessionID, sessionIncarnationID string) string {
	seed := strings.Join([]string{kind, taskID, usageTaskIncarnationID(taskCreatedAt), sessionID, sessionIncarnationID}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-usage-preservation\x00"+seed)).String()
}

func (r *Repository) checkTaskPreservationFenceTx(ctx context.Context, tx *sqlx.Tx, taskID string, taskCreatedAt time.Time) error {
	var state string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_preservation_fences WHERE task_id = ? AND task_created_at = ? AND state IN (?, ?)
		ORDER BY created_at DESC LIMIT 1`), taskID, taskCreatedAt, usagePreserving, usagePreservationReady).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("usage writes are fenced while task %q history is being preserved", taskID)
}

func (r *Repository) checkSessionPreservationFenceTx(ctx context.Context, tx *sqlx.Tx, taskID string, taskCreatedAt time.Time, sessionID, incarnationID string) error {
	var state string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_preservation_fences
		WHERE fence_kind = 'session' AND task_id = ? AND task_created_at = ? AND session_id = ?
		AND session_incarnation_id = ? AND state IN (?, ?) ORDER BY created_at DESC LIMIT 1`),
		taskID, taskCreatedAt, sessionID, incarnationID, usagePreserving, usagePreservationReady).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("usage writes are fenced while session %q history is being preserved", sessionID)
}

func (r *Repository) checkManagedDeleteFenceTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	marker := dialect.JSONExtract(r.db.DriverName(), "COALESCE(NULLIF(resource_snapshot, ''), '{}')", "managed_delete")
	var active bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM task_resource_cleanup_jobs WHERE task_id = ?
		AND state IN ('prepared', 'pending', 'running', 'retry_wait', 'waiting_for_clean') AND `+marker+` IS NOT NULL)`), taskID).Scan(&active)
	if err != nil && !db.IsMissingTableError(err) {
		return err
	}
	if active {
		return fmt.Errorf("usage writes are fenced by managed deletion of task %q", taskID)
	}
	return nil
}

func (r *Repository) ensureUsageScopeWritableTx(ctx context.Context, tx *sqlx.Tx, workspaceID, organizationID string) error {
	for _, entry := range []struct{ kind, id string }{{"workspace", workspaceID}, {"organization", organizationID}} {
		if entry.id == "" {
			continue
		}
		if err := r.lockUsageScopeFenceTx(ctx, tx, entry.kind, entry.id); err != nil {
			return err
		}
		var state string
		query := `SELECT state FROM usage_scope_fences WHERE fence_kind = ? AND fence_id = ?`
		if dialect.IsPostgres(r.db.DriverName()) {
			query += usageForUpdate
		}
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), entry.kind, entry.id).Scan(&state); err != nil {
			return err
		}
		if state != usageFenceActive {
			return fmt.Errorf("%w: %s %q is %s", errUsageScopeErasing, entry.kind, entry.id, state)
		}
	}
	return nil
}

func (r *Repository) lockUsageScopeFenceTx(ctx context.Context, tx *sqlx.Tx, kind, id string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_scope_fences (fence_kind, fence_id, state, generation, updated_at)
		VALUES (?, ?, ?, 0, ?) ON CONFLICT(fence_kind, fence_id) DO NOTHING`), kind, id, usageFenceActive, time.Now().UTC())
	return err
}

func (r *Repository) CompleteTaskUsageTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, "", false)
	if err != nil {
		return err
	}
	if err := r.requireNativeUsagePreservedTx(ctx, tx, target); err != nil {
		return err
	}
	fenceID := usagePreservationID(usageWorkFallback, taskID, target.TaskCreatedAt, "", "")
	if err := r.detachUsageLinksTx(ctx, tx, taskID, "", ""); err != nil {
		return err
	}
	return r.markUsagePreservationDeletedTx(ctx, tx, fenceID)
}

func (r *Repository) CompleteSessionUsageTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID, incarnationID string) error {
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, sessionID, false)
	if err != nil {
		return err
	}
	if target.SessionIncarnationID != incarnationID {
		return errors.New("session incarnation changed before retained-link detachment")
	}
	if err := r.requireNativeUsagePreservedTx(ctx, tx, target); err != nil {
		return err
	}
	fenceID := usagePreservationID(usageSubjectSession, taskID, target.TaskCreatedAt, sessionID, incarnationID)
	if err := r.detachUsageLinksTx(ctx, tx, taskID, sessionID, incarnationID); err != nil {
		return err
	}
	return r.markUsagePreservationDeletedTx(ctx, tx, fenceID)
}

func (r *Repository) detachUsageLinksTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID, sessionIncarnationID string) error {
	var query string
	var args []any
	if sessionID == "" {
		query = `UPDATE usage_live_links SET live_task_id = NULL, live_session_id = NULL WHERE live_task_id = ?`
		args = []any{taskID}
	} else {
		query = `UPDATE usage_live_links SET live_session_id = NULL WHERE live_session_id = ? AND subject_id IN (
			SELECT subject_id FROM usage_subjects WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?)`
		args = []any{sessionID, taskID, sessionID, sessionIncarnationID}
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}

func (r *Repository) markUsagePreservationDeletedTx(ctx context.Context, tx *sqlx.Tx, fenceID string) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_preservation_fences SET state = ?, updated_at = ?
		WHERE fence_id = ? AND state = ?`), usagePreservationDone, time.Now().UTC(), fenceID, usagePreservationReady)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("usage preservation is not complete; destructive cleanup is fenced")
	}
	return nil
}

func (r *Repository) ReleaseTaskUsage(ctx context.Context, taskID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, "", true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	fenceID := usagePreservationID(usageWorkFallback, taskID, target.TaskCreatedAt, "", "")
	return r.releaseUsageFenceTx(ctx, tx, fenceID)
}

func (r *Repository) ReleaseSessionUsage(ctx context.Context, taskID, sessionID, incarnationID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	target, err := r.readTaskUsageTargetTx(ctx, tx, taskID, sessionID, true)
	if err != nil {
		return err
	}
	if target.SessionIncarnationID != incarnationID {
		return nil
	}
	fenceID := usagePreservationID(usageSubjectSession, taskID, target.TaskCreatedAt, sessionID, incarnationID)
	return r.releaseUsageFenceTx(ctx, tx, fenceID)
}

func (r *Repository) releaseUsageFenceTx(ctx context.Context, tx *sqlx.Tx, fenceID string) error {
	var state string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM usage_preservation_fences WHERE fence_id = ?`), fenceID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}
	if state == usagePreservationDone {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_cursors WHERE fence_id = ?`), fenceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_preservation_fences WHERE fence_id = ? AND state IN (?, ?)`), fenceID, usagePreserving, usagePreservationReady); err != nil {
		return err
	}
	return tx.Commit()
}
