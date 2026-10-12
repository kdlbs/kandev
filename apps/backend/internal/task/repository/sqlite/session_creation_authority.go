package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) authorizeManualCreation(ctx context.Context, exec taskSessionExecutor, kind, id, taskID string) error {
	if !models.ExplicitManualCreation(ctx, taskID) {
		return nil
	}
	switch exec.(type) {
	case *sqlx.Tx, *sql.Tx:
	default:
		return errors.New("manual creation requires a transaction")
	}
	_, err := exec.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_creation_authorizations(kind,creation_id,task_id) VALUES (?,?,?)`), kind, id, taskID)
	return err
}

func (r *Repository) clearManualCreation(ctx context.Context, exec taskSessionExecutor, kind, id, taskID string) error {
	if !models.ExplicitManualCreation(ctx, taskID) {
		return nil
	}
	_, err := exec.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_creation_authorizations WHERE kind=? AND creation_id=?`), kind, id)
	return err
}
