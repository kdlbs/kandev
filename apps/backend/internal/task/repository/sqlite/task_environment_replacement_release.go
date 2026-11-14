package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// ReleaseTaskEnvironmentReplacement releases only the captured, unused claim.
// A renewed claim, a started session, and a live execution retain ownership.
func (r *Repository) ReleaseTaskEnvironmentReplacement(ctx context.Context, expected *models.TaskEnvironment, sessionID string) (bool, error) {
	if expected == nil || sessionID == "" || expected.MaterializationSessionID != sessionID || expected.Status != models.TaskEnvironmentStatusCreating {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, expected.ID); err != nil {
		return false, err
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, expected.ID); err != nil {
		return false, err
	}
	terminal, err := r.lockReplacementReleaseOwnerTx(ctx, tx, sessionID)
	if err != nil || !terminal {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_environments
		SET status = ?, materialization_session_id = '', updated_at = ?
		WHERE id = ? AND task_id = ? AND ownership_generation = ? AND updated_at = ?
		  AND retired_at IS NULL AND status = ? AND materialization_session_id = ?
		  AND NOT EXISTS (SELECT 1 FROM task_sessions WHERE id = ? AND state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED'))
		  AND NOT EXISTS (
		    SELECT 1 FROM executors_running er JOIN task_sessions ts ON ts.id = er.session_id
		    WHERE ts.task_environment_id = ? AND (er.status IS NULL OR er.status NOT IN (?, ?, ?))
		  )`), models.TaskEnvironmentStatusFailed, r.nowUTC(),
		expected.ID, expected.TaskID, expected.OwnershipGeneration, expected.UpdatedAt,
		models.TaskEnvironmentStatusCreating, sessionID, sessionID, expected.ID,
		models.ExecutorRunningStatusFailed, models.ExecutorRunningStatusStopped, models.ExecutorRunningStatusComplete)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) lockReplacementReleaseOwnerTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (bool, error) {
	query := `SELECT state FROM task_sessions WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE"
	}
	var state models.TaskSessionState
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), sessionID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return isTerminalWorkspaceMaterializerState(state), err
}
