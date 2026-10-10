package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) AddTaskBlockerExact(
	ctx context.Context,
	taskID, blockerTaskID, workspaceID, taskVersion, relatedVersion string,
	fence models.TaskManagementClaimFence,
) (bool, error) {
	return r.changeTaskBlockerExact(ctx, taskID, blockerTaskID, workspaceID, taskVersion, relatedVersion, fence, true)
}

func (r *Repository) RemoveTaskBlockerExact(
	ctx context.Context,
	taskID, blockerTaskID, workspaceID, taskVersion, relatedVersion string,
	fence models.TaskManagementClaimFence,
) (bool, error) {
	return r.changeTaskBlockerExact(ctx, taskID, blockerTaskID, workspaceID, taskVersion, relatedVersion, fence, false)
}

//nolint:cyclop,funlen,gocognit // The transaction updates the blocker edge and task resource versions atomically.
func (r *Repository) changeTaskBlockerExact(
	ctx context.Context,
	taskID, blockerTaskID, workspaceID, taskVersion, relatedVersion string,
	fence models.TaskManagementClaimFence,
	add bool,
) (bool, error) {
	expectedVersions := map[string]time.Time{}
	for id, raw := range map[string]string{taskID: taskVersion, blockerTaskID: relatedVersion} {
		if id == "" || raw == "" {
			return false, errors.New("exact task relation identity is incomplete")
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return false, fmt.Errorf("invalid exact task relation version: %w", err)
		}
		expectedVersions[id] = parsed
	}
	ids := []string{taskID, blockerTaskID}
	sort.Strings(ids)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, id := range ids {
		query := `SELECT workspace_id, updated_at FROM tasks WHERE id = ?`
		if dialect.IsPostgres(r.db.DriverName()) {
			query += ` FOR UPDATE`
		}
		var storedWorkspaceID string
		var updatedAt time.Time
		if err := tx.QueryRowContext(ctx, tx.Rebind(query), id).Scan(&storedWorkspaceID, &updatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, id)
			}
			return false, err
		}
		if storedWorkspaceID != workspaceID || !updatedAt.Equal(expectedVersions[id]) {
			return false, repoerrors.ErrTaskVersionConflict
		}
		if !dialect.IsPostgres(r.db.DriverName()) {
			result, err := tx.ExecContext(ctx, tx.Rebind(`
				UPDATE tasks SET updated_at = updated_at
				WHERE id = ? AND workspace_id = ? AND updated_at = ?
			`), id, workspaceID, updatedAt)
			if err != nil {
				return false, err
			}
			if affected, err := result.RowsAffected(); err != nil || affected != 1 {
				if err != nil {
					return false, err
				}
				return false, repoerrors.ErrTaskVersionConflict
			}
		}
	}

	var ownerKind, installationID, instanceKey string
	var generation int64
	claimErr := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT owner_kind, installation_id, instance_key, generation
		FROM task_management_claims WHERE task_id = ?
	`), taskID).Scan(&ownerKind, &installationID, &instanceKey, &generation)
	if claimErr != nil && !errors.Is(claimErr, sql.ErrNoRows) {
		return false, claimErr
	}
	if errors.Is(claimErr, sql.ErrNoRows) || ownerKind == "" {
		if fence.InstallationID != "" || fence.InstanceKey != "" || fence.Generation != 0 {
			return false, repoerrors.ErrTaskManagementClaimConflict
		}
	} else if ownerKind != "plugin" || fence.InstallationID != installationID || fence.InstanceKey != instanceKey || fence.Generation != generation {
		return false, repoerrors.ErrTaskManagementClaimConflict
	}

	var result sql.Result
	if add {
		result, err = tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO task_blockers (task_id, blocker_task_id, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(task_id, blocker_task_id) DO NOTHING
		`), taskID, blockerTaskID, time.Now().UTC())
	} else {
		result, err = tx.ExecContext(ctx, tx.Rebind(`
			DELETE FROM task_blockers WHERE task_id = ? AND blocker_task_id = ?
		`), taskID, blockerTaskID)
	}
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected == 0, nil
}
