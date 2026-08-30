package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// initCoordinatorGrantSchema owns the shared Coordinator authorization seam.
// Pending-move cancellation and workflow routing both depend on this relation;
// neither creates a parallel grant representation.
func (r *Repository) initCoordinatorGrantSchema() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (coordinator_task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_workspace_coordinator_grants_task
			ON workspace_coordinator_grants(coordinator_task_id);
	`)
	if err != nil {
		return fmt.Errorf("init workspace coordinator grant schema: %w", err)
	}
	return nil
}

// IsCurrentCoordinatorGrant verifies both the durable same-workspace grant and
// the live execution that supplied the server-attested MCP principal.
func (r *Repository) IsCurrentCoordinatorGrant(
	ctx context.Context,
	workspaceID, taskID, sessionID, executionID string,
) (bool, error) {
	var one int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1
		FROM workspace_coordinator_grants grant
		JOIN tasks task
			ON task.id = grant.coordinator_task_id
			AND task.workspace_id = grant.workspace_id
		JOIN task_sessions session
			ON session.id = ? AND session.task_id = task.id
		JOIN executors_running execution
			ON execution.session_id = session.id
			AND execution.task_id = task.id
			AND execution.agent_execution_id = ?
		WHERE grant.workspace_id = ? AND grant.coordinator_task_id = ?
			AND session.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
			AND execution.status IN ('starting', 'running', 'ready')
	`), sessionID, executionID, workspaceID, taskID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read current coordinator grant: %w", err)
	}
	return true, nil
}
