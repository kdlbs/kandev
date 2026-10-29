package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type officeEligibilityLocator struct{ workspace, workflow, step string }

// lockOfficeEligibilityScopes serializes slate and workflow policy writers
// through the workspace exclusion used by task admission. Locators are checked
// again after locking; a changed source never acquires another workspace late.
func (r *Repository) lockOfficeEligibilityScopes(ctx context.Context, tx *sql.Tx, taskIDs, stepIDs, workflowIDs []string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		// The workflow repository also supports standalone schemas without tasks.
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET id=id WHERE 0`); err != nil {
			return err
		}
	}
	before, err := r.officeEligibilityLocators(ctx, tx, taskIDs, stepIDs, workflowIDs)
	if err != nil {
		return err
	}
	workspaces := make([]string, 0, len(before))
	for _, scope := range before {
		workspaces = append(workspaces, scope.workspace)
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := db.LockTaskHierarchy(ctx, tx, r.db.DriverName(), r.db.Rebind, workspaces, nil); err != nil {
			return err
		}
	}
	after, err := r.officeEligibilityLocators(ctx, tx, taskIDs, stepIDs, workflowIDs)
	if err != nil {
		return err
	}
	if len(before) != len(after) {
		return repoerrors.ErrWorkflowChangeConflict
	}
	for key, scope := range before {
		if after[key] != scope {
			return repoerrors.ErrWorkflowChangeConflict
		}
	}
	return nil
}

func (r *Repository) officeEligibilityLocators(ctx context.Context, tx *sql.Tx, taskIDs, stepIDs, workflowIDs []string) (map[string]officeEligibilityLocator, error) {
	result := make(map[string]officeEligibilityLocator)
	if err := r.officeTaskEligibilityLocators(ctx, tx, taskIDs, result); err != nil {
		return nil, err
	}
	for _, id := range stepIDs {
		if id == "" {
			continue
		}
		var scope officeEligibilityLocator
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT w.workspace_id,s.workflow_id FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE s.id=?`), id).Scan(&scope.workspace, &scope.workflow)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result["step:"+id] = scope
	}
	for _, id := range workflowIDs {
		if id == "" {
			continue
		}
		var scope officeEligibilityLocator
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), id).Scan(&scope.workspace)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result["workflow:"+id] = scope
	}
	return result, nil
}

func (r *Repository) officeTaskEligibilityLocators(ctx context.Context, tx *sql.Tx, taskIDs []string, result map[string]officeEligibilityLocator) error {
	if len(taskIDs) == 0 {
		return nil
	}
	present, err := r.officeTaskTablePresent(ctx, tx)
	if err != nil || !present {
		return err
	}
	for _, id := range taskIDs {
		if id == "" {
			continue
		}
		var scope officeEligibilityLocator
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workspace_id,''),COALESCE(workflow_id,''),COALESCE(workflow_step_id,'') FROM tasks WHERE id=?`), id).Scan(&scope.workspace, &scope.workflow, &scope.step)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		result["task:"+id] = scope
	}
	return nil
}

func (r *Repository) officeTaskTablePresent(ctx context.Context, tx *sql.Tx) (bool, error) {
	var present bool
	query := `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks')`
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `SELECT to_regclass('tasks') IS NOT NULL`
	}
	err := tx.QueryRowContext(ctx, query).Scan(&present)
	return present, err
}

type officeParticipantLocator struct {
	step, task string
	present    bool
}

func (r *Repository) officeParticipantLocator(ctx context.Context, tx *sql.Tx, id string) (officeParticipantLocator, error) {
	var row officeParticipantLocator
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT step_id,task_id FROM workflow_step_participants WHERE id=?`), id).Scan(&row.step, &row.task)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	row.present = err == nil
	return row, err
}
func (r *Repository) lockOfficeParticipantID(ctx context.Context, tx *sql.Tx, id, stepID, taskID string) (bool, error) {
	if !dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET id=id WHERE 0`); err != nil {
			return false, err
		}
	}
	before, err := r.officeParticipantLocator(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if err := r.lockOfficeEligibilityScopes(ctx, tx, []string{taskID, before.task}, []string{stepID, before.step}, nil); err != nil {
		return false, err
	}
	after, err := r.officeParticipantLocator(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if before != after {
		return false, repoerrors.ErrWorkflowChangeConflict
	}
	return before.present, nil
}

func (r *Repository) officeCompletionTxOptions() *sql.TxOptions {
	if dialect.IsPostgres(r.db.DriverName()) {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}
