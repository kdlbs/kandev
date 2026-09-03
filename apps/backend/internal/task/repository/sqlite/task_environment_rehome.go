package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if err := r.lockGitSnapshotEnvironment(tx, environmentID); err != nil {
		return false, err
	}
	if !allowPossibleDataLoss {
		safe, assessErr := r.environmentSnapshotProvesRecoverableTx(ctx, tx, environmentID, sessionID)
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

func (r *Repository) environmentSnapshotProvesRecoverableTx(
	ctx context.Context, tx *sqlx.Tx, environmentID, sessionID string,
) (bool, error) {
	expected, complete, err := r.rehomeExpectedRepositoryNames(ctx, tx, environmentID)
	if err != nil || !complete {
		return false, err
	}

	repositoryExpr := snapshotRepositoryExpr(r.db.DriverName(), "task_session_git_snapshots")
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`
		SELECT `+repositoryExpr+`, triggered_by, ahead, remote_branch, files
		FROM task_session_git_snapshots
		WHERE task_environment_id = ? AND session_id = ?
		ORDER BY created_at DESC, id DESC
	`), environmentID, sessionID)
	if err != nil {
		return false, fmt.Errorf("assess workspace loss: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return rehomeSnapshotsProveRecoverable(rows, expected)
}

func (r *Repository) rehomeExpectedRepositoryNames(
	ctx context.Context, tx *sqlx.Tx, environmentID string,
) (map[string]struct{}, bool, error) {
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`
		SELECT repositories.name FROM task_environment_repos
		JOIN repositories ON repositories.id = task_environment_repos.repository_id
		WHERE task_environment_repos.task_environment_id = ?
		  AND task_environment_repos.deleted_at IS NULL
		  AND task_environment_repos.status NOT IN ('failed', 'deleted')
	`), environmentID)
	if err != nil {
		return nil, false, fmt.Errorf("load workspace repository inventory: %w", err)
	}
	defer func() { _ = rows.Close() }()
	expected := make(map[string]struct{})
	for rows.Next() {
		var repositoryName string
		if err := rows.Scan(&repositoryName); err != nil {
			return nil, false, fmt.Errorf("scan workspace repository inventory: %w", err)
		}
		if _, duplicate := expected[repositoryName]; duplicate {
			return expected, false, nil
		}
		expected[repositoryName] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate workspace repository inventory: %w", err)
	}
	return expected, len(expected) > 0, nil
}

func rehomeSnapshotsProveRecoverable(rows *sql.Rows, expected map[string]struct{}) (bool, error) {
	seen := make(map[string]struct{}, len(expected))
	for rows.Next() {
		var repositoryName, triggeredBy, remoteBranch, filesJSON string
		var ahead int
		if err := rows.Scan(&repositoryName, &triggeredBy, &ahead, &remoteBranch, &filesJSON); err != nil {
			return false, fmt.Errorf("scan workspace loss snapshot: %w", err)
		}
		if _, ok := seen[repositoryName]; ok {
			continue
		}
		if _, ok := expected[repositoryName]; !ok {
			return false, nil
		}
		seen[repositoryName] = struct{}{}
		var files map[string]interface{}
		if filesJSON != "" && filesJSON != "null" {
			if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
				return false, nil
			}
		}
		if triggeredBy != triggeredByAgentCompleted || ahead != 0 || remoteBranch == "" || len(files) != 0 {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate workspace loss snapshots: %w", err)
	}
	return len(seen) == len(expected), nil
}
