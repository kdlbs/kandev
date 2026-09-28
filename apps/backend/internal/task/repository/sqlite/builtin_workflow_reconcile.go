// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// stepEventsRetryCause names why a reconciliation attempt asks its caller to
// retry the same row in a fresh transaction, rather than treating the
// attempt's error as terminal. It is a closed set of two values.
type stepEventsRetryCause string

const (
	stepEventsRetryCauseCASConflict    stepEventsRetryCause = "cas_conflict"
	stepEventsRetryCauseLockContention stepEventsRetryCause = "lock_contention"
)

// stepEventsDBSite names one of the database operations inside
// tryHealWorkflowStepEvents that workflowStepEventsDBFailpoint can
// intercept.
type stepEventsDBSite string

const (
	stepEventsDBSiteBegin        stepEventsDBSite = "begin"
	stepEventsDBSiteRead         stepEventsDBSite = "read"
	stepEventsDBSiteWrite        stepEventsDBSite = "write"
	stepEventsDBSiteRowsAffected stepEventsDBSite = "rows_affected"
	stepEventsDBSiteCommit       stepEventsDBSite = "commit"
)

// stepEventsHealOutcome is the result of one reconciliation attempt.
type stepEventsHealOutcome struct {
	// applied is true when the row already had the action, the row does not
	// exist, or the write committed — all "nothing left to do" outcomes.
	applied bool
	// retryCause is empty when no retry is wanted; otherwise cas_conflict or
	// lock_contention.
	retryCause stepEventsRetryCause
	// retryErr is the wrapped database error when retryCause is
	// lock_contention; nil otherwise.
	retryErr error
}

// stepEventsOp runs op unless the test-only workflowStepEventsDBFailpoint
// returns a non-nil error for site, in which case op is skipped and the
// failpoint's error substitutes for it.
func (r *Repository) stepEventsOp(stepID string, site stepEventsDBSite, op func() error) error {
	if r.workflowStepEventsDBFailpoint != nil {
		if err := r.workflowStepEventsDBFailpoint(stepID, site); err != nil {
			return err
		}
	}
	return op()
}

// classifyStepEventsErr classifies a wrapped attempt error as a retryable
// lock-contention outcome or a terminal error, using the repository's
// existing transient-contention classifier.
func classifyStepEventsErr(err error) (stepEventsHealOutcome, error) {
	if isRetryableStateRaceError(err) {
		return stepEventsHealOutcome{retryCause: stepEventsRetryCauseLockContention, retryErr: err}, nil
	}
	return stepEventsHealOutcome{}, err
}

// tryHealWorkflowStepEvents performs the shared read-modify-write operation
// used by builtin workflow reconcilers. The mutator returns true when the
// desired state is already present. A transient database contention error
// at any operation, or a zero-row compare-and-swap result, asks the caller
// to retry the same row in a fresh transaction; every other error is
// terminal for the row.
func (r *Repository) tryHealWorkflowStepEvents(
	stepID, operation string,
	mutate func(*wfmodels.StepEvents) bool,
) (stepEventsHealOutcome, error) {
	var tx *sqlx.Tx
	beginErr := r.stepEventsOp(stepID, stepEventsDBSiteBegin, func() error {
		var err error
		tx, err = r.db.BeginTxx(r.migrationContext(), nil)
		return err
	})
	if beginErr != nil {
		return classifyStepEventsErr(fmt.Errorf("begin %s reconciliation tx: %w", operation, beginErr))
	}
	defer func() { _ = tx.Rollback() }()

	var rawEvents sql.NullString
	readErr := r.stepEventsOp(stepID, stepEventsDBSiteRead, func() error {
		return tx.QueryRowContext(r.migrationContext(), tx.Rebind(`SELECT events FROM workflow_steps WHERE id = ?`), stepID).Scan(&rawEvents)
	})
	if readErr != nil {
		if errors.Is(readErr, sql.ErrNoRows) {
			return stepEventsHealOutcome{applied: true}, nil
		}
		return classifyStepEventsErr(fmt.Errorf("read step events: %w", readErr))
	}
	if r.workflowStepEventsAfterReadHook != nil {
		r.workflowStepEventsAfterReadHook(stepID)
	}

	var events wfmodels.StepEvents
	if rawEvents.String != "" {
		if err := json.Unmarshal([]byte(rawEvents.String), &events); err != nil {
			return stepEventsHealOutcome{}, fmt.Errorf("parse step events: %w", err)
		}
	}

	if mutate(&events) {
		return stepEventsHealOutcome{applied: true}, nil
	}
	updated, err := json.Marshal(events)
	if err != nil {
		return stepEventsHealOutcome{}, fmt.Errorf("marshal step events: %w", err)
	}

	return r.writeHealedStepEvents(tx, stepID, operation, rawEvents, updated)
}

// writeHealedStepEvents runs the guarded UPDATE, RowsAffected, and Commit
// sites of a reconciliation attempt, classifying any error at each site.
func (r *Repository) writeHealedStepEvents(
	tx *sqlx.Tx, stepID, operation string, rawEvents sql.NullString, updated []byte,
) (stepEventsHealOutcome, error) {
	// events is TEXT and nullable; NULL requires "IS NULL" on both dialects
	// since neither treats a bound parameter after IS/= as NULL-matching.
	guardClause := "events = ?"
	guardArg := any(rawEvents.String)
	if !rawEvents.Valid {
		guardClause = "events IS NULL"
		guardArg = nil
	}
	args := []any{string(updated), time.Now().UTC(), stepID}
	if guardArg != nil {
		args = append(args, guardArg)
	}

	var res sql.Result
	writeErr := r.stepEventsOp(stepID, stepEventsDBSiteWrite, func() error {
		var err error
		res, err = tx.ExecContext(r.migrationContext(), tx.Rebind(`
			UPDATE workflow_steps SET events = ?, updated_at = ?
			WHERE id = ? AND `+guardClause), args...)
		return err
	})
	if writeErr != nil {
		return classifyStepEventsErr(fmt.Errorf("write step events: %w", writeErr))
	}

	var affected int64
	rowsErr := r.stepEventsOp(stepID, stepEventsDBSiteRowsAffected, func() error {
		var err error
		affected, err = res.RowsAffected()
		return err
	})
	if rowsErr != nil {
		return classifyStepEventsErr(fmt.Errorf("check %s reconciliation write: %w", operation, rowsErr))
	}
	if affected == 0 {
		return stepEventsHealOutcome{retryCause: stepEventsRetryCauseCASConflict}, nil
	}

	commitErr := r.stepEventsOp(stepID, stepEventsDBSiteCommit, tx.Commit)
	if commitErr != nil {
		return classifyStepEventsErr(fmt.Errorf("commit %s reconciliation: %w", operation, commitErr))
	}
	return stepEventsHealOutcome{applied: true}, nil
}

// runStepEventsRetryLoop drives up to budget reconciliation attempts for one
// reconciliation target. It returns (nil, nil) once an attempt applies the
// desired state, (nil, err) when an attempt returns a non-transient error,
// and a non-nil outcome pointer carrying the last attempt's retry cause when
// the budget is exhausted without applying.
func runStepEventsRetryLoop(budget int, attempt func() (stepEventsHealOutcome, error)) (*stepEventsHealOutcome, error) {
	var last stepEventsHealOutcome
	for i := 0; i < budget; i++ {
		outcome, err := attempt()
		if err != nil {
			return nil, err
		}
		if outcome.applied || outcome.retryCause == "" {
			return nil, nil
		}
		last = outcome
	}
	return &last, nil
}
