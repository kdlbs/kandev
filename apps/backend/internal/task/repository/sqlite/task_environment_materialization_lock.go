package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const taskEnvironmentMaterializationLockNamespace = "task-environment-materialization:"

// Identity discovery does not authorize a state transition. Lock the owner
// task before the environment advisory lock, then read mutable claim state in
// a separate READ COMMITTED statement after this function returns.
func (r *Repository) lockTaskEnvironmentMaterializationTx(ctx context.Context, tx *sqlx.Tx, environmentID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) || environmentID == "" {
		return nil
	}
	var ownerTaskID string
	query := r.db.Rebind(`SELECT task_id FROM task_environments WHERE id = ?`)
	if err := tx.QueryRowContext(ctx, query, environmentID).Scan(&ownerTaskID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("resolve materialization owner identity: %w", err)
	}
	if err := r.lockTaskRowInTx(ctx, tx, ownerTaskID); err != nil {
		return err
	}
	var currentOwnerTaskID string
	if err := tx.QueryRowContext(ctx, query, environmentID).Scan(&currentOwnerTaskID); err != nil {
		return err
	}
	if currentOwnerTaskID != ownerTaskID {
		return ErrTaskEnvironmentOwnershipChanged
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		taskEnvironmentMaterializationLockNamespace+environmentID)
	return err
}

func (r *Repository) lockTaskMaterializationForBindingTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	var environmentID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM task_environments WHERE task_id = ?`), taskID).Scan(&environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.lockTaskEnvironmentMaterializationTx(ctx, tx, environmentID)
}

type sharedMaterializationIdentity struct {
	groupOwnerTaskID       string
	environmentID          string
	environmentOwnerTaskID string
}

func (r *Repository) sharedMaterializationIdentityTx(ctx context.Context, tx *sqlx.Tx, groupID string) (sharedMaterializationIdentity, error) {
	var identity sharedMaterializationIdentity
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT g.owner_task_id, g.materialized_environment_id, COALESCE(e.task_id, '')
		FROM task_workspace_groups g
		LEFT JOIN task_environments e ON e.id = g.materialized_environment_id
		WHERE g.id = ?
	`), groupID).Scan(&identity.groupOwnerTaskID, &identity.environmentID, &identity.environmentOwnerTaskID)
	return identity, err
}

// Shared bindings acquire all known task rows in one order, including the
// group owner before its first environment exists. Identity changes while
// waiting fail closed rather than extending the lock set out of order.
func (r *Repository) lockSharedMaterializationTasksTx(ctx context.Context, tx *sqlx.Tx, groupID, taskID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	identity, err := r.sharedMaterializationIdentityTx(ctx, tx, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: shared workspace group is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	if err != nil {
		return err
	}
	taskIDs := []string{taskID, identity.groupOwnerTaskID, identity.environmentOwnerTaskID}
	sort.Strings(taskIDs)
	previous := ""
	for _, id := range taskIDs {
		if id == "" || id == previous {
			continue
		}
		if err := r.lockTaskRowInTx(ctx, tx, id); err != nil {
			return err
		}
		previous = id
	}
	fresh, err := r.sharedMaterializationIdentityTx(ctx, tx, groupID)
	if err != nil {
		return err
	}
	if fresh != identity {
		if identity.environmentID == "" && fresh.environmentID != "" {
			return &models.WorkspacePreparingError{EnvironmentID: fresh.environmentID, Reason: "retry after the shared workspace election completes"}
		}
		return fmt.Errorf("%w: shared workspace identity changed during admission", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}
