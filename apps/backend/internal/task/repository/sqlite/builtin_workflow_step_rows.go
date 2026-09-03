// Package sqlite provides SQLite-based repository implementations.
package sqlite

import "fmt"

// systemWorkflowStepRow identifies a single system-owned workflow_steps row
// materialized from a given (template, step name) pair.
type systemWorkflowStepRow struct{ stepID, workflowID string }

// findSystemOwnedWorkflowSteps returns every system-owned (is_system = 1)
// workflow_steps row materialized from (templateID, stepName) across every
// workspace. Shared by the startup healers that reconcile
// workflow_steps.events onto already-materialized rows (participant seats,
// on_agent_error escalation): rows are collected up front so their
// transactional per-row retry loops never run with an open read cursor over
// the same table.
func (r *Repository) findSystemOwnedWorkflowSteps(templateID, stepName string) ([]systemWorkflowStepRow, error) {
	rows, err := r.db.Query(r.db.Rebind(`
		SELECT ws.id, ws.workflow_id FROM workflow_steps ws
		JOIN workflows w ON w.id = ws.workflow_id
		WHERE w.is_system = 1 AND w.workflow_template_id = ? AND ws.name = ?
	`), templateID, stepName)
	if err != nil {
		return nil, fmt.Errorf("find system-owned step rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var targets []systemWorkflowStepRow
	for rows.Next() {
		var row systemWorkflowStepRow
		if scanErr := rows.Scan(&row.stepID, &row.workflowID); scanErr != nil {
			return nil, fmt.Errorf("scan step row: %w", scanErr)
		}
		targets = append(targets, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate step rows: %w", err)
	}
	return targets, nil
}
