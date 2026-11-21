package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db"
)

const usageSourceAdoptionKey = "source-adoption"

const (
	usageAdoptionAdopted      = "adopted"
	usageAdoptionNoTranscript = "no_transcript"
	usageAdoptionForked       = "forked"
	usageAdoptionUnsafe       = "unsafe"
	usageAdoptionTransferred  = "transferred"
)

var usageSourceAdoptionTotal = expvar.NewMap("usage_source_adoption_total")

// BackfillUsageSourceAdoption adopts sessions that existed before collection
// evidence was captured. The cursor advances with each committed page, so a
// restart resumes without repeating a completed page, and a completed cursor
// makes every later call a no-op.
func (r *Repository) BackfillUsageSourceAdoption(ctx context.Context, limit int) (bool, error) {
	if limit < 1 {
		return false, errors.New("usage source adoption batch limit must be positive")
	}
	if limit > usageHistoryBatchLimit {
		limit = usageHistoryBatchLimit
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	cursor, err := r.ensureUsageSourceAdoptionCursorTx(ctx, tx)
	if err != nil {
		return false, err
	}
	if cursor.Complete != 0 {
		return true, tx.Commit()
	}
	var rows []struct {
		ID     string `db:"id"`
		TaskID string `db:"task_id"`
	}
	if err := tx.SelectContext(ctx, &rows, r.db.Rebind(`SELECT id, task_id FROM task_sessions
		WHERE id > ? ORDER BY id LIMIT ?`), cursor.LastKey, limit); err != nil {
		return false, err
	}
	lastKey := cursor.LastKey
	counts := make(map[string]int64)
	for _, row := range rows {
		label, err := r.adoptUsageSourceSessionTx(ctx, tx, row.TaskID, row.ID)
		if err != nil {
			return false, err
		}
		if label != "" {
			counts[label]++
		}
		lastKey = row.ID
	}
	complete := len(rows) < limit
	if complete {
		lastKey = ""
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_history_migration_state
		SET last_key = ?, complete = ?, updated_at = ? WHERE source_key = ?`),
		lastKey, boolInt(complete), time.Now().UTC(), usageSourceAdoptionKey)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	for label, count := range counts {
		usageSourceAdoptionTotal.Add(label, count)
	}
	return complete, nil
}

func (r *Repository) ensureUsageSourceAdoptionCursorTx(ctx context.Context, tx *sqlx.Tx) (usagePreservationCursor, error) {
	query := `SELECT source_key, high_water, last_key, complete FROM usage_history_migration_state WHERE source_key = ?`
	if r.db.DriverName() == usageDriverPostgres {
		query += usageForUpdate
	}
	var cursor usagePreservationCursor
	err := tx.GetContext(ctx, &cursor, r.db.Rebind(query), usageSourceAdoptionKey)
	if err == nil {
		return cursor, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return cursor, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_history_migration_state
		(source_key, high_water, last_key, complete, updated_at) VALUES (?, '', '', 0, ?)
		ON CONFLICT(source_key) DO NOTHING`), usageSourceAdoptionKey, time.Now().UTC())
	if err != nil {
		return cursor, err
	}
	if err := tx.GetContext(ctx, &cursor, r.db.Rebind(query), usageSourceAdoptionKey); err != nil {
		return cursor, err
	}
	return cursor, nil
}

func (r *Repository) usageSourceAdoptionCompleteTx(ctx context.Context, tx *sqlx.Tx) (bool, error) {
	var complete int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT complete FROM usage_history_migration_state WHERE source_key = ?`), usageSourceAdoptionKey).Scan(&complete)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return complete != 0, nil
}

func (r *Repository) adoptUsageSourceSessionTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (string, error) {
	workspace, organization, found, err := r.usageAdoptionScopeTx(ctx, tx, taskID, sessionID)
	if err != nil || !found {
		return "", err
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, workspace, organization); err != nil {
		if errors.Is(err, errUsageScopeErasing) {
			return "", nil
		}
		return "", err
	}
	item, target, trusted, err := r.usageAdoptionTargetTx(ctx, tx, workspace, taskID, sessionID)
	if err != nil || trusted {
		return "", err
	}
	reason, err := r.usageAdoptionExclusionTx(ctx, tx, target, taskID)
	if err != nil || reason != "" {
		return reason, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_collection_contexts
		SET trusted = 1, evidence = 'migration' WHERE context_id = ?`), sourceContextKey(target))
	if err != nil {
		return "", err
	}
	if err := r.captureCollectionTargetTx(ctx, tx, item.SubjectID); err != nil {
		return "", err
	}
	return usageAdoptionAdopted, nil
}

func (r *Repository) usageAdoptionScopeTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (string, string, bool, error) {
	var workspace, organization string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.workspace_id, COALESCE(w.org_id, '') FROM task_sessions ts
		JOIN tasks t ON t.id = ts.task_id JOIN workspaces w ON w.id = t.workspace_id
		WHERE ts.id = ? AND ts.task_id = ?`), sessionID, taskID).Scan(&workspace, &organization)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return workspace, organization, true, nil
}

func (r *Repository) usageAdoptionTargetTx(ctx context.Context, tx *sqlx.Tx, workspace, taskID, sessionID string) (models.SessionUsageMeasurement, retainedTarget, bool, error) {
	item, err := r.bindExternalUsageScopeTx(ctx, tx, models.SessionUsageMeasurement{
		WorkspaceID: workspace, TaskID: taskID, SessionID: sessionID, CollectedAt: time.Now().UTC(),
	})
	if err != nil {
		return item, retainedTarget{}, false, err
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings
		WHERE subject_id = ? ORDER BY sequence DESC LIMIT 1`), item.SubjectID); err != nil {
		return item, target, false, err
	}
	if err := r.captureTargetSessionTx(ctx, tx, &target); err != nil {
		return item, target, false, err
	}
	var trusted int
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT trusted FROM usage_collection_contexts
		WHERE context_id = ?`), sourceContextKey(target)).Scan(&trusted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return item, target, false, err
	}
	return item, target, trusted != 0, nil
}

func (r *Repository) usageAdoptionExclusionTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, taskID string) (string, error) {
	if target.ExternalSessionID == "" {
		return usageAdoptionNoTranscript, nil
	}
	if target.Forked {
		return usageAdoptionForked, nil
	}
	transferred, err := r.usageSourceTaskWasTransferredTx(ctx, tx, taskID)
	if err != nil || transferred {
		if transferred {
			return usageAdoptionTransferred, nil
		}
		return "", err
	}
	unsafe, err := r.usageSourceScopeUnsafeTx(ctx, tx, target)
	if err != nil {
		return "", err
	}
	if unsafe {
		return usageAdoptionUnsafe, nil
	}
	return "", nil
}

func (r *Repository) usageSourceTaskWasTransferredTx(ctx context.Context, tx *sqlx.Tx, taskID string) (bool, error) {
	exists, err := db.TableExistsContext(ctx, tx, "task_transfer_audit")
	if err != nil || !exists {
		return false, err
	}
	var transferred bool
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM task_transfer_audit WHERE task_id = ?)`), taskID).Scan(&transferred)
	return transferred, err
}
