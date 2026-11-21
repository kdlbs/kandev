package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) initReviewSourceApprovalSchema() error {
	return r.migrate.Apply("task_review_source_approvals.table", `
		CREATE TABLE IF NOT EXISTS task_review_source_approvals (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
			approval TEXT NOT NULL
		)`)
}

// MergeTaskReviewSource changes the selection and approval in one transaction.
func (r *Repository) MergeTaskReviewSource(ctx context.Context, id string, source any, approval *models.ReviewSourceApproval) error {
	value, err := json.Marshal(source)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	return r.mergeTaskMetadata(ctx, id, map[string]json.RawMessage{models.MetaKeyReviewSource: value}, raw)
}

func (r *Repository) writeReviewSourceApproval(ctx context.Context, tx *sql.Tx, id string, record json.RawMessage) error {
	if string(record) == jsonNull {
		_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_review_source_approvals WHERE task_id = ?`), id)
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_review_source_approvals (task_id, approval) VALUES (?, ?)
		ON CONFLICT (task_id) DO UPDATE SET approval = excluded.approval`), id, string(record))
	return err
}

func (r *Repository) ReadReviewSourceApproval(ctx context.Context, id string) (*models.ReviewSourceApproval, error) {
	var raw string
	err := r.ro.GetContext(ctx, &raw, r.ro.Rebind(`SELECT approval FROM task_review_source_approvals WHERE task_id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var approval models.ReviewSourceApproval
	if err := json.Unmarshal([]byte(raw), &approval); err != nil {
		return nil, err
	}
	return &approval, nil
}
