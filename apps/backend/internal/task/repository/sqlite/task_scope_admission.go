package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// lockTaskScope holds the task's workspace boundary before checking scope
// again. The second read closes the task-to-workspace race on PostgreSQL.
func (r *Repository) lockTaskScope(ctx context.Context, tx *sql.Tx, id string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return r.lockTaskHierarchy(ctx, tx, nil, []string{id})
	}
	before, err := r.taskMutationScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := r.lockTaskHierarchy(ctx, tx, []string{before[0]}, nil); err != nil {
		return err
	}
	after, err := r.taskMutationScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if before != after {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) taskMutationScope(ctx context.Context, tx *sql.Tx, id string) ([3]string, error) {
	var scope [3]string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT COALESCE(workspace_id,''), COALESCE(workflow_id,''), COALESCE(workflow_step_id,'')
		FROM tasks WHERE id = ?
	`), id).Scan(&scope[0], &scope[1], &scope[2])
	return scope, err
}

func (r *Repository) lockWorkflowCompletionPolicyTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	var workspace string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), id).Scan(&workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.lockTaskHierarchy(ctx, tx, []string{workspace}, nil); err != nil {
		return err
	}
	var current string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), id).Scan(&current); err != nil {
		return err
	}
	if current != workspace {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}
