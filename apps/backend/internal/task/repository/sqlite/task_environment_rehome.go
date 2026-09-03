package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// ClaimTaskEnvironmentRehome atomically turns the current ready/stopped
// binding back into a fresh materialization owned by sessionID. A false claim
// means another caller already changed the binding. Physical repository rows
// are removed only after the loss gate has allowed a fresh materialization.
func (r *Repository) ClaimTaskEnvironmentRehome(
	ctx context.Context,
	taskID, environmentID, sessionID string,
	allowPossibleDataLoss bool,
) (bool, error) {
	if taskID == "" || environmentID == "" || sessionID == "" {
		return false, fmt.Errorf("task, environment, and session are required for workspace rehome")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return false, err
	}
	if !allowPossibleDataLoss {
		safe, assessErr := r.environmentSnapshotProvesRecoverableTx(ctx, tx, environmentID)
		if assessErr != nil {
			return false, assessErr
		}
		if !safe {
			return false, models.ErrWorkspaceRehomeNeedsAuthorization
		}
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments
		SET status = ?, materialization_session_id = ?, workspace_path = '',
			control_port = 0, container_id = '', sandbox_id = '', updated_at = ?
		WHERE id = ? AND task_id = ? AND status IN (?, ?)
	`), string(models.TaskEnvironmentStatusCreating), sessionID, r.nowUTC(), environmentID, taskID,
		string(models.TaskEnvironmentStatusReady), string(models.TaskEnvironmentStatusStopped))
	if err != nil {
		return false, fmt.Errorf("claim task environment rehome: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_environment_repos WHERE task_environment_id = ?`), environmentID); err != nil {
		return false, fmt.Errorf("clear superseded workspace inventory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) environmentSnapshotProvesRecoverableTx(ctx context.Context, tx *sqlx.Tx, environmentID string) (bool, error) {
	var ahead int
	var remoteBranch string
	var filesJSON string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT ahead, remote_branch, files FROM task_session_git_snapshots
		WHERE task_environment_id = ? ORDER BY created_at DESC, id DESC LIMIT 1
	`), environmentID).Scan(&ahead, &remoteBranch, &filesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("assess workspace loss: %w", err)
	}
	var files map[string]interface{}
	if filesJSON != "" && filesJSON != "null" {
		if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
			return false, nil
		}
	}
	return ahead == 0 && remoteBranch != "" && len(files) == 0, nil
}
