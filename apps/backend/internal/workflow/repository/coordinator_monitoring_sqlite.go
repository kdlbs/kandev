package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
)

// initCoordinatorMonitoringSchema creates the workflow_coordinator_monitoring
// table. Each row records whether the Coordinator plugin monitors a given
// workflow step, plus an optional coordinator-specific prompt for that step.
// This table is host-owned storage (Settings > Workspace > Workflow
// configuration): the Coordinator plugin only reads it through a
// capability-scoped Host API, it never writes it directly.
func (r *Repository) initCoordinatorMonitoringSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workflow_coordinator_monitoring (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		selected INTEGER NOT NULL DEFAULT 0,
		prompt TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(workflow_id, workflow_step_id),
		FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
		FOREIGN KEY (workflow_step_id) REFERENCES workflow_steps(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_coordinator_monitoring_workflow
		ON workflow_coordinator_monitoring(workflow_id);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create workflow_coordinator_monitoring table: %w", err)
	}
	return nil
}

// GetCoordinatorMonitoring returns every saved coordinator-monitoring row for
// a workflow. Steps with no saved row (never checked, no prompt) are simply
// absent from the result; callers treat that as "not monitored, no prompt".
func (r *Repository) GetCoordinatorMonitoring(ctx context.Context, workflowID string) ([]models.CoordinatorStepMonitor, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT workflow_step_id, selected, prompt, updated_at
		FROM workflow_coordinator_monitoring
		WHERE workflow_id = ?
		ORDER BY workflow_step_id
	`), workflowID)
	if err != nil {
		return nil, fmt.Errorf("list coordinator monitoring: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]models.CoordinatorStepMonitor, 0)
	for rows.Next() {
		var entry models.CoordinatorStepMonitor
		var selected int
		var updatedAt string
		if err := rows.Scan(&entry.WorkflowStepID, &selected, &entry.Prompt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan coordinator monitoring row: %w", err)
		}
		entry.Selected = selected == 1
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			entry.UpdatedAt = t
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate coordinator monitoring rows: %w", err)
	}
	return result, nil
}

// ReplaceCoordinatorMonitoring atomically replaces every coordinator
// monitoring row for a workflow with entries. Every entries[i].WorkflowStepID
// must belong to workflowID; if any does not, the whole call is rejected and
// nothing is written. Entries that are neither selected nor carry a prompt
// are skipped on insert (unchecking a step and saving removes its row).
func (r *Repository) ReplaceCoordinatorMonitoring(ctx context.Context, workspaceID, workflowID string, entries []models.CoordinatorStepMonitor) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.validateCoordinatorMonitoringSteps(ctx, tx, workflowID, entries); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM workflow_coordinator_monitoring WHERE workflow_id = ?
	`), workflowID); err != nil {
		return fmt.Errorf("clear coordinator monitoring: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, entry := range entries {
		if !entry.Selected && entry.Prompt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO workflow_coordinator_monitoring (
				id, workspace_id, workflow_id, workflow_step_id, selected, prompt, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`), uuid.New().String(), workspaceID, workflowID, entry.WorkflowStepID,
			dialect.BoolToInt(entry.Selected), entry.Prompt, now, now); err != nil {
			return fmt.Errorf("insert coordinator monitoring row: %w", err)
		}
	}

	return tx.Commit()
}

// validateCoordinatorMonitoringSteps rejects the whole replace call when any
// entry references a workflow_step_id that is unknown or belongs to a
// different workflow, so a bad request never partially applies.
func (r *Repository) validateCoordinatorMonitoringSteps(ctx context.Context, tx *sqlx.Tx, workflowID string, entries []models.CoordinatorStepMonitor) error {
	if len(entries) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(`
		SELECT id FROM workflow_steps WHERE workflow_id = ?
	`), workflowID)
	if err != nil {
		return fmt.Errorf("load workflow steps for validation: %w", err)
	}
	defer func() { _ = rows.Close() }()

	valid := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan workflow step id: %w", err)
		}
		valid[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workflow step ids: %w", err)
	}

	for _, entry := range entries {
		if _, ok := valid[entry.WorkflowStepID]; !ok {
			return fmt.Errorf("workflow_step_id %q does not belong to workflow %q", entry.WorkflowStepID, workflowID)
		}
	}
	return nil
}
