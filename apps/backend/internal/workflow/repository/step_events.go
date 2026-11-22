package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/workflow/models"
)

// StepEventsUpdate replaces one step's events, provided the step still has the
// updated_at version the caller read.
type StepEventsUpdate struct {
	StepID            string
	WorkflowID        string
	Events            models.StepEvents
	ExpectedUpdatedAt time.Time
}

// StepEventsVersion is a step's new updated_at after UpdateStepEventsIfUnchanged.
type StepEventsVersion struct {
	StepID    string
	UpdatedAt time.Time
}

// UpdateStepEventsIfUnchanged writes every update in one transaction. Only the
// events column and updated_at change. If any step changed after it was read,
// nothing is written and the error wraps models.ErrWorkflowStepChanged.
func (r *Repository) UpdateStepEventsIfUnchanged(
	ctx context.Context, updates []StepEventsUpdate,
) ([]StepEventsVersion, error) {
	if len(updates) == 0 {
		return nil, nil
	}
	stepIDs := make([]string, 0, len(updates))
	workflowIDs := make([]string, 0, len(updates))
	for _, u := range updates {
		stepIDs = append(stepIDs, u.StepID)
		workflowIDs = append(workflowIDs, u.WorkflowID)
	}
	tx, err := r.db.BeginTxx(ctx, r.officeCompletionTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockOfficeEligibilityScopes(ctx, tx.Tx, nil, stepIDs, workflowIDs); err != nil {
		return nil, err
	}
	// Microseconds survive a round trip on every supported database.
	now := time.Now().UTC().Truncate(time.Microsecond)
	saved := make([]StepEventsVersion, 0, len(updates))
	for _, u := range updates {
		eventsJSON, err := json.Marshal(u.Events)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal events: %w", err)
		}
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_steps SET events = ?, updated_at = ?
			WHERE id = ? AND workflow_id = ? AND updated_at = ?
		`), string(eventsJSON), now, u.StepID, u.WorkflowID, u.ExpectedUpdatedAt)
		if err != nil {
			return nil, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rows == 0 {
			return nil, fmt.Errorf("%w: %s", models.ErrWorkflowStepChanged, u.StepID)
		}
		saved = append(saved, StepEventsVersion{StepID: u.StepID, UpdatedAt: now})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}
