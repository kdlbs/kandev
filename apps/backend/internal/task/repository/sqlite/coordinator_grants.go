package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
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
	workspaceID, taskID, sessionID, executionID, automationID string,
) (bool, error) {
	var metadataJSON string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT task.metadata
		FROM workspace_coordinator_grants coordinator_grant
		JOIN tasks task
			ON task.id = coordinator_grant.coordinator_task_id
			AND task.workspace_id = coordinator_grant.workspace_id
		JOIN workspaces workspace
			ON workspace.id = coordinator_grant.workspace_id
			AND COALESCE(workspace.owner_id, '') = coordinator_grant.created_by_user_id
		JOIN task_sessions session
			ON session.id = ? AND session.task_id = task.id
		JOIN executors_running execution
			ON execution.session_id = session.id
			AND execution.task_id = task.id
			AND execution.agent_execution_id = ?
		WHERE coordinator_grant.workspace_id = ? AND coordinator_grant.coordinator_task_id = ?
			AND task.origin = ?
			AND session.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
			AND execution.status IN ('starting', 'running', 'ready')
	`), sessionID, executionID, workspaceID, taskID, models.TaskOriginAutomationRun).Scan(&metadataJSON)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read current coordinator grant: %w", err)
	}
	var metadata map[string]interface{}
	if json.Unmarshal([]byte(metadataJSON), &metadata) != nil {
		return false, nil
	}
	return automationID != "" && models.StringFromAny(metadata["automation_id"]) == automationID, nil
}

func (r *Repository) designateAutomationCoordinatorTx(
	ctx context.Context, tx *sql.Tx, task *models.Task, createdAt time.Time,
) error {
	if task == nil || task.Origin != models.TaskOriginAutomationRun ||
		models.StringFromAny(task.Metadata["automation_id"]) == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspace_coordinator_grants (
			workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at
		)
		SELECT task.workspace_id, task.id, COALESCE(workspace.owner_id, ''), ?, ?
		FROM tasks task
		JOIN workspaces workspace ON workspace.id = task.workspace_id
		WHERE task.id = ? AND task.workspace_id = ? AND task.origin = ?
		ON CONFLICT(workspace_id) DO UPDATE SET
			coordinator_task_id = excluded.coordinator_task_id,
			created_by_user_id = excluded.created_by_user_id,
			updated_at = excluded.updated_at
	`), createdAt, createdAt, task.ID, task.WorkspaceID, models.TaskOriginAutomationRun)
	if err != nil {
		return fmt.Errorf("designate automation coordinator: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read automation coordinator designation: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("designate automation coordinator: task %s is not a trusted automation run", task.ID)
	}
	return nil
}

// DesignateAutomationCoordinator refreshes the current grant when a durable
// continuation task is reused. The task row is locked and its persisted
// origin/automation identity is revalidated before the workspace grant moves.
func (r *Repository) DesignateAutomationCoordinator(
	ctx context.Context, taskID, automationID string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT workspace_id, origin, metadata FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += postgresForUpdateClause
	}
	var task models.Task
	var metadataJSON string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(
		&task.WorkspaceID, &task.Origin, &metadataJSON,
	); err != nil {
		return fmt.Errorf("read automation coordinator task: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataJSON), &task.Metadata); err != nil ||
		task.Origin != models.TaskOriginAutomationRun || automationID == "" ||
		models.StringFromAny(task.Metadata["automation_id"]) != automationID {
		return fmt.Errorf("task %s is not the requested trusted automation run", taskID)
	}
	task.ID = taskID
	if err := r.designateAutomationCoordinatorTx(ctx, tx, &task, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}
