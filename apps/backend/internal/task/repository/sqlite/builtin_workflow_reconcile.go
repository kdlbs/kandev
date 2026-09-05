// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// tryHealWorkflowStepEvents performs the shared read-modify-write operation
// used by builtin workflow reconcilers. The mutator returns true when the
// desired state is already present. A zero-row compare-and-swap result asks
// the caller to retry with a fresh events value.
func (r *Repository) tryHealWorkflowStepEvents(
	stepID, operation string,
	mutate func(*wfmodels.StepEvents) bool,
) (applied, retry bool, err error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return false, false, fmt.Errorf("begin %s reconciliation tx: %w", operation, err)
	}
	defer func() { _ = tx.Rollback() }()

	var rawEvents sql.NullString
	if err := tx.QueryRow(tx.Rebind(`SELECT events FROM workflow_steps WHERE id = ?`), stepID).Scan(&rawEvents); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, false, nil
		}
		return false, false, fmt.Errorf("read step events: %w", err)
	}

	var events wfmodels.StepEvents
	if rawEvents.String != "" {
		if err := json.Unmarshal([]byte(rawEvents.String), &events); err != nil {
			return false, false, fmt.Errorf("parse step events: %w", err)
		}
	}

	if mutate(&events) {
		return true, false, nil
	}
	updated, err := json.Marshal(events)
	if err != nil {
		return false, false, fmt.Errorf("marshal step events: %w", err)
	}

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
	res, err := tx.Exec(tx.Rebind(`
		UPDATE workflow_steps SET events = ?, updated_at = ?
		WHERE id = ? AND `+guardClause), args...)
	if err != nil {
		return false, false, fmt.Errorf("write step events: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, false, fmt.Errorf("check %s reconciliation write: %w", operation, err)
	}
	if affected == 0 {
		return false, true, nil
	}
	if err := tx.Commit(); err != nil {
		return false, false, fmt.Errorf("commit %s reconciliation: %w", operation, err)
	}
	return true, false, nil
}
