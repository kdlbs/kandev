package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) archiveTaskSessionEvidenceTx(ctx context.Context, tx *sqlx.Tx, session *models.TaskSession) error {
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	if err := messagequeue.DeleteSessionInTransaction(ctx, tx, r.db, identity); err != nil && !db.IsMissingTableError(err) {
		return fmt.Errorf("close session queue: %w", err)
	}
	now := r.nowUTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET archived_at = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ? AND archived_at IS NULL
	`), now, now, session.ID, session.TaskID, session.QueueIncarnationID)
	if err != nil {
		return fmt.Errorf("archive session evidence: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count archived session rows: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("archive session evidence changed during cleanup: affected=%d", affected)
	}
	return r.insertTaskSessionCleanupReceiptTx(ctx, tx, session, "archived", true)
}

func (r *Repository) insertTaskSessionCleanupReceiptTx(
	ctx context.Context, tx *sqlx.Tx, session *models.TaskSession, disposition string, evidenceRetained bool,
) error {
	var workspaceID string
	if err := tx.GetContext(ctx, &workspaceID, r.db.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), session.TaskID); err != nil {
		return fmt.Errorf("read cleanup task workspace: %w", err)
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_cleanup_receipts
			(task_id, workspace_id, session_id, disposition, queue_before_count, evidence_retained, occurred_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
	`), session.TaskID, workspaceID, session.ID, disposition, evidenceRetained, r.nowUTC())
	if err != nil {
		return fmt.Errorf("record session cleanup receipt: %w", err)
	}
	return nil
}

func (r *Repository) GetTaskSessionCleanupReceipt(
	ctx context.Context, taskID, sessionID string,
) (*models.TaskSessionCleanupReceipt, error) {
	receipt := &models.TaskSessionCleanupReceipt{}
	err := r.ro.GetContext(ctx, receipt, r.ro.Rebind(`
		SELECT task_id, workspace_id, session_id, disposition,
			queue_before_count, evidence_retained, occurred_at
		FROM task_session_cleanup_receipts WHERE task_id = ? AND session_id = ?
	`), taskID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read session cleanup receipt: %w", err)
	}
	return receipt, nil
}
