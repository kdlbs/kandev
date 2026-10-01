package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
)

// ReorderSteps commits a complete step order without rewriting step content.
func (r *Repository) ReorderSteps(ctx context.Context, workflowID string, stepIDs []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if dialect.IsPostgres(r.db.DriverName()) {
		// The parent lock serializes reorders and prevents new membership until commit.
		var id string
		err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id FROM workflows WHERE id = ? FOR UPDATE`), workflowID).Scan(&id)
		if err == sql.ErrNoRows {
			return models.ErrWorkflowStepNotFound
		}
		if err != nil {
			return err
		}
	}
	var existing []string
	if err := tx.SelectContext(ctx, &existing, tx.Rebind(`SELECT id FROM workflow_steps WHERE workflow_id = ?`), workflowID); err != nil {
		return err
	}
	if err := validateStepOrderMembership(existing, stepIDs); err != nil {
		return err
	}
	now := time.Now().UTC()
	for position, id := range stepIDs {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_steps SET position = ?, updated_at = ? WHERE id = ? AND workflow_id = ?
		`), position, now, id, workflowID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return models.ErrWorkflowStepNotFound
		}
	}
	return tx.Commit()
}

func validateStepOrderMembership(existing, requested []string) error {
	members := make(map[string]bool, len(existing))
	for _, id := range existing {
		members[id] = false
	}
	for _, id := range requested {
		seen, exists := members[id]
		if !exists {
			return models.ErrWorkflowStepNotFound
		}
		if seen {
			return fmt.Errorf("workflow step order contains duplicate IDs")
		}
		members[id] = true
	}
	if len(existing) != len(requested) {
		return fmt.Errorf("workflow step order must include every step")
	}
	return nil
}
