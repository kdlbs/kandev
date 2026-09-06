package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// initCoordinatorGrantSchema owns the shared Coordinator authorization seam.
// Pending-move cancellation and workflow routing both depend on this relation;
// neither creates a parallel grant representation.
func (r *Repository) initCoordinatorGrantSchema() error {
	_, err := r.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_id_id
			ON tasks(workspace_id, id);
		CREATE TABLE IF NOT EXISTS workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY NOT NULL CONSTRAINT workspace_coordinator_grants_workspace_id_nonempty CHECK (workspace_id <> ''),
			coordinator_task_id TEXT NOT NULL CONSTRAINT workspace_coordinator_grants_task_id_nonempty CHECK (coordinator_task_id <> ''),
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (workspace_id, coordinator_task_id)
				REFERENCES tasks(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_workspace_coordinator_grants_task
			ON workspace_coordinator_grants(coordinator_task_id);
	`)
	if err != nil {
		return fmt.Errorf("init workspace coordinator grant schema: %w", err)
	}
	return nil
}

// migrateCoordinatorGrantSchema upgrades the pre-routing grant table, whose
// task-only foreign key did not bind the coordinator task to the grant's
// workspace. A table rebuild is required on both supported dialects because
// SQLite cannot alter foreign keys and keeping the weak constraint would leave
// the security contract ambiguous on PostgreSQL.
func (r *Repository) migrateCoordinatorGrantSchema() error {
	current, err := r.coordinatorGrantSchemaCurrent()
	if err != nil || current {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin coordinator grant schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(r.db.Rebind(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_id_id ON tasks(workspace_id, id)`)); err != nil {
		return fmt.Errorf("ensure task workspace identity: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE workspace_coordinator_grants RENAME TO workspace_coordinator_grants_legacy`); err != nil {
		return fmt.Errorf("rename legacy coordinator grants: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY NOT NULL CONSTRAINT workspace_coordinator_grants_workspace_id_nonempty CHECK (workspace_id <> ''),
			coordinator_task_id TEXT NOT NULL CONSTRAINT workspace_coordinator_grants_task_id_nonempty CHECK (coordinator_task_id <> ''),
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (workspace_id, coordinator_task_id)
				REFERENCES tasks(workspace_id, id) ON DELETE CASCADE
		)`); err != nil {
		return fmt.Errorf("create corrected coordinator grants: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		SELECT workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at
		FROM workspace_coordinator_grants_legacy`); err != nil {
		return fmt.Errorf("copy coordinator grants: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE workspace_coordinator_grants_legacy`); err != nil {
		return fmt.Errorf("drop legacy coordinator grants: %w", err)
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_workspace_coordinator_grants_task`); err != nil {
		return fmt.Errorf("drop legacy coordinator grant index: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_workspace_coordinator_grants_task ON workspace_coordinator_grants(coordinator_task_id)`); err != nil {
		return fmt.Errorf("create coordinator grant index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit coordinator grant schema migration: %w", err)
	}
	return nil
}

func (r *Repository) coordinatorGrantSchemaCurrent() (bool, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		var current bool
		err := r.db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint c
				JOIN pg_class table_ref ON table_ref.oid = c.conrelid
				WHERE table_ref.relname = 'workspace_coordinator_grants'
				  AND c.contype = 'f'
				  AND LOWER(pg_get_constraintdef(c.oid)) LIKE 'foreign key (workspace_id, coordinator_task_id) references %tasks%(workspace_id, id) on delete cascade%'
			) AND EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE tablename = 'tasks' AND indexdef ILIKE '%(workspace_id, id)%'
			) AND EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = 'workspace_coordinator_grants'::regclass
				  AND conname = 'workspace_coordinator_grants_workspace_id_nonempty'
			) AND EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = 'workspace_coordinator_grants'::regclass
				  AND conname = 'workspace_coordinator_grants_task_id_nonempty'
			)`).Scan(&current)
		return current, err
	}
	var schema string
	err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'workspace_coordinator_grants'`).Scan(&schema)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(schema), " "))
	return strings.Contains(normalized, "foreign key (workspace_id, coordinator_task_id) references tasks(workspace_id, id) on delete cascade") &&
		strings.Contains(normalized, "check (workspace_id <> '')") &&
		strings.Contains(normalized, "check (coordinator_task_id <> '')"), nil
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
