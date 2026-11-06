package repository

import (
	"context"
	"database/sql"
	"github.com/kandev/kandev/internal/db/dialect"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const sourceRowForUpdate = " FOR UPDATE"

type sourceGuardQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Repository) ensurePromptSourceMutableTx(ctx context.Context, tx sourceGuardQuery, workflowID string) error {
	query := `SELECT id FROM workflows WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += sourceRowForUpdate
	}
	var source string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), workflowID).Scan(&source)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var locked int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM prompt_source_instances WHERE workflow_id = ? AND source_locked = 1`), workflowID).Scan(&locked); err != nil {
		return err
	}
	if locked > 0 {
		return taskmodels.ErrPromptSourceLocked
	}

	return nil
}
func (r *Repository) ensureStepPromptSourceMutableTx(ctx context.Context, tx sourceGuardQuery, stepID string) error {
	var workflowID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workflow_id FROM workflow_steps WHERE id = ?`), stepID).Scan(&workflowID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return r.ensurePromptSourceMutableTx(ctx, tx, workflowID)
}
