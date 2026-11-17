package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type participantTaskScope struct{ workspace, workflow, step string }

func (r *Repository) participantTaskScope(ctx context.Context, tx *sqlx.Tx, id string) (participantTaskScope, error) {
	var scope participantTaskScope
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workspace_id,''),COALESCE(workflow_id,''),COALESCE(workflow_step_id,'') FROM tasks WHERE id=?`), id).Scan(&scope.workspace, &scope.workflow, &scope.step)
	return scope, err
}
func (r *Repository) lockParticipantTaskScope(ctx context.Context, tx *sqlx.Tx, id string) (bool, error) {
	if !dialect.IsPostgres(r.db.DriverName()) {
		if err := internaldb.LockTaskHierarchy(ctx, tx, r.db.DriverName(), r.db.Rebind, nil, nil); err != nil {
			return false, err
		}
	}
	before, err := r.participantTaskScope(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := internaldb.LockTaskHierarchy(ctx, tx, r.db.DriverName(), r.db.Rebind, []string{before.workspace}, nil); err != nil {
			return false, err
		}
	}
	after, err := r.participantTaskScope(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if before != after {
		return false, repoerrors.ErrWorkflowChangeConflict
	}
	return true, nil
}

func (r *Repository) participantScopeTxOptions() *sql.TxOptions {
	if dialect.IsPostgres(r.db.DriverName()) {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}
