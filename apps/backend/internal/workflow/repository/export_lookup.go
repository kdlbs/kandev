package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/workflow/models"
)

// GetStepTx is GetStep's transaction-accepting counterpart: the automations
// YAML export (AC-29) opens one read transaction spanning several stores and
// passes it through here rather than letting this method open its own, so
// the step read observes the same snapshot as every other read in the
// export. A missing row is reported as found=false rather than an error -
// AC-19's partial-resolution rule decides what that means, not this method.
func (r *Repository) GetStepTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.WorkflowStep, bool, error) {
	row := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE id = ?
	`), id)

	step, err := r.scanStep(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return step, true, nil
}
