package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) validateLocalExecutorAuthority(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget) error {
	var pid int
	var updatedAt time.Time
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT r.local_pid,r.updated_at FROM executors_running r JOIN task_sessions s ON s.id=r.session_id WHERE r.session_id=? AND r.task_id=? AND s.task_environment_id=? AND r.runtime='standalone' AND r.status!='stopped'`), target.AuthoritySessionID, target.TaskID, target.EnvironmentID).Scan(&pid, &updatedAt)
	if err != nil {
		return err
	}
	if pid <= 0 || pid != target.LocalPID || target.ResourceKey != fmt.Sprintf("local-pid:%d", pid) || !updatedAt.Equal(target.ExpectedExecutorUpdatedAt) {
		return fmt.Errorf("local controller authority changed")
	}
	return nil
}

// A verified current resource can recover an earlier episode only within its
// logical owner. The physical cause remains immutable history.
func (r *Repository) findSupersededExecutorFailure(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget, observedAt time.Time) (*models.ExecutorFailureEpisode, error) {
	if target.EnvironmentID != "" {
		return scanExecutorFailure(tx.QueryRowContext(ctx, r.db.Rebind(executorFailureSelect+` WHERE task_id=? AND session_id='' AND state='active' AND last_observed_at<? ORDER BY last_observed_at DESC,id DESC LIMIT 1`), target.TaskID, observedAt))
	}
	return scanExecutorFailure(tx.QueryRowContext(ctx, r.db.Rebind(executorFailureSelect+` WHERE task_id=? AND session_id=? AND environment_id='' AND state='active' AND last_observed_at<? ORDER BY last_observed_at DESC,id DESC LIMIT 1`), target.TaskID, target.SessionID, observedAt))
}

func (r *Repository) resolveOwnedExecutorFailures(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget, observedAt time.Time) error {
	scope := "session_id=''"
	args := []any{observedAt, observedAt, target.TaskID, observedAt}
	if target.EnvironmentID == "" {
		scope = "environment_id='' AND session_id=?"
		args = append(args, target.SessionID)
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE executor_failure_episodes SET state='resolved',current_outcome='healthy',revision=revision+1,last_observed_at=?,resolved_at=? WHERE task_id=? AND state='active' AND last_observed_at<? AND `+scope), args...)
	return err
}
