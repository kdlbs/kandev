package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) beginBackgroundWorkTx(ctx context.Context, sessionID string) (*sqlx.Tx, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		_, err = tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`), "background-work:"+sessionID)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return tx, nil
}

// SettleBackgroundWorkloads commits the workload and run projections together.
// Terminal parents with open runs are included to repair earlier partial writes.
func (r *Repository) SettleBackgroundWorkloads(ctx context.Context, sessionID string, finishedAt time.Time) ([]*models.BackgroundWorkload, error) {
	tx, err := r.beginBackgroundWorkTx(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	query, args, err := sqlx.In(`SELECT w.* FROM task_session_background_work w
  WHERE w.session_id = ? AND (w.state NOT IN (?) OR EXISTS (
   SELECT 1 FROM task_session_background_runs r WHERE r.session_id=w.session_id
    AND r.workload_id=w.id AND r.state NOT IN (?))) ORDER BY w.id`,
		sessionID, streams.TerminalRunStates, streams.TerminalRunStates)
	if err != nil {
		return nil, err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var workloads []*models.BackgroundWorkload
	if err := tx.SelectContext(ctx, &workloads, tx.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, work := range workloads {
		if streams.RunState(work.State).IsTerminal() {
			continue
		}
		work.State = string(streams.RunStateEnded)
		work.FinishedAt = &finishedAt
		work.Revision++
		_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_background_work
   SET state=?, finished_at=?, revision=?, updated_at=? WHERE session_id=? AND id=?`),
			work.State, finishedAt, work.Revision, finishedAt, sessionID, work.ID)
		if err != nil {
			return nil, err
		}
	}
	query, args, err = sqlx.In(`UPDATE task_session_background_runs SET state=?, finished_at=?, updated_at=?
  WHERE session_id=? AND state NOT IN (?)`, streams.RunStateEnded, finishedAt, finishedAt, sessionID, streams.TerminalRunStates)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return workloads, nil
}

// reconcileBackgroundRun preserves final runs and fences delayed starts behind
// a terminal workload. A late final observation can still supply the outcome.
func reconcileBackgroundRun(ctx context.Context, tx *sqlx.Tx, run *models.BackgroundRun) (bool, error) {
	if streams.RunState(run.State).IsTerminal() {
		return true, nil
	}
	var work models.BackgroundWorkload
	err := tx.GetContext(ctx, &work, tx.Rebind(`SELECT state, finished_at FROM task_session_background_work WHERE session_id=? AND id=?`), run.SessionID, run.WorkloadID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var existing models.BackgroundRun
	err = tx.GetContext(ctx, &existing, tx.Rebind(`SELECT state FROM task_session_background_runs WHERE session_id=? AND id=?`), run.SessionID, run.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if streams.RunState(existing.State).IsTerminal() {
		return false, nil
	}
	if streams.RunState(work.State).IsTerminal() {
		run.State = string(streams.RunStateEnded)
		run.FinishedAt = work.FinishedAt
		run.ExitCode = nil
	}
	return true, nil
}
