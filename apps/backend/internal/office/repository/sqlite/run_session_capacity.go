package sqlite

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// SetRunSessionCapacityReleased installs the existing admission wake callback
// before launch and event consumers start. No Office retry queue is created.
func (r *Repository) SetRunSessionCapacityReleased(notify func()) {
	r.runSessionCapacityReleased = notify
}

// ListAdmittedRunExecutions deliberately does not join runs: cancellation or
// failure of a run alone does not prove that its owned execution stopped.
// Legacy active attempts retain GradeKnown=false rather than a guessed grade.
func (r *Repository) ListAdmittedRunExecutions(ctx context.Context) ([]taskmodels.AdmittedExecution, error) {
	var rows []taskmodels.AdmittedExecution
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT id,admitted_execution_grade,admitted_execution_grade_known FROM office_run_sessions WHERE state IN (?,?) ORDER BY id`), models.RunSessionStatePreparing, models.RunSessionStateRunning)
	return rows, err
}

// BindRunSessionCandidate persists the caller's fresh execution identity before
// runtime allocation, so uncertain launch responses retain a recovery locator.
func (r *Repository) BindRunSessionCandidate(ctx context.Context, id, executionID string) (bool, error) {
	if executionID == "" {
		return false, errors.New("run candidate execution ID required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE office_run_sessions SET execution_id=?,version=version+1 WHERE id=? AND state=? AND execution_id='' AND cancel_requested_at IS NULL`), executionID, id, models.RunSessionStatePreparing)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// RequeueUnstartedRunForCapacity retains the original queue record and retry
// budget. An active predecessor or a cancelled run cannot be requeued here.
func (r *Repository) RequeueUnstartedRunForCapacity(ctx context.Context, runID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE runs SET status='queued',claimed_at=NULL WHERE id=? AND status='claimed' AND NOT EXISTS(SELECT 1 FROM office_run_sessions WHERE run_id=runs.id AND state IN (?,?))`), runID, models.RunSessionStatePreparing, models.RunSessionStateRunning)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *Repository) migrateRunSessionCapacity() error {
	if err := r.migrate.Apply("office_run_sessions.admitted_execution_grade", `ALTER TABLE office_run_sessions ADD COLUMN admitted_execution_grade TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return r.migrate.Apply("office_run_sessions.admitted_execution_grade_known", `ALTER TABLE office_run_sessions ADD COLUMN admitted_execution_grade_known INTEGER NOT NULL DEFAULT 0`)
}
