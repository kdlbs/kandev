package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const workflowSessionBindingsTableDDL = `
	CREATE TABLE IF NOT EXISTS task_workflow_session_bindings (
		task_id TEXT NOT NULL,
		target_key TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		session_id TEXT,
		operation_id TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (task_id, target_key),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL
	);
	CREATE INDEX IF NOT EXISTS idx_task_workflow_session_bindings_workflow
		ON task_workflow_session_bindings(workflow_id, target_key);
	CREATE INDEX IF NOT EXISTS idx_task_workflow_session_bindings_session
		ON task_workflow_session_bindings(session_id);
`

func (r *Repository) migrateWorkflowSessionBindings() error {
	return r.migrate.Apply("task_workflow_session_bindings.table", workflowSessionBindingsTableDDL)
}

// GetWorkflowSessionBinding loads the latest committed binding for a source
// step. A missing row is a valid skipped-source state.
func (r *Repository) GetWorkflowSessionBinding(
	ctx context.Context,
	taskID, targetKey string,
) (*models.WorkflowSessionBinding, error) {
	var binding models.WorkflowSessionBinding
	var sessionID sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT task_id, target_key, workflow_id, agent_profile_id,
		       session_id, operation_id, updated_at
		FROM task_workflow_session_bindings
		WHERE task_id = ? AND target_key = ?
	`), taskID, targetKey).Scan(
		&binding.TaskID,
		&binding.TargetKey,
		&binding.WorkflowID,
		&binding.AgentProfileID,
		&sessionID,
		&binding.OperationID,
		&binding.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load workflow session binding: %w", err)
	}
	if sessionID.Valid {
		binding.SessionID = sessionID.String
	}
	return &binding, nil
}

// UpsertWorkflowSessionBinding commits a source-step session choice. Versioned
// step-entry operations compare their immutable entry identity inside the same
// upsert statement, while legacy operations retain timestamp ordering. The
// bool reports whether this operation won the conditional write.
func (r *Repository) UpsertWorkflowSessionBinding(
	ctx context.Context,
	binding *models.WorkflowSessionBinding,
) (bool, error) {
	if binding == nil || binding.TaskID == "" || binding.TargetKey == "" ||
		binding.WorkflowID == "" || binding.AgentProfileID == "" || binding.OperationID == "" {
		return false, fmt.Errorf("workflow session binding identity is required")
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO task_workflow_session_bindings
			(task_id, target_key, workflow_id, agent_profile_id, session_id, operation_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id, target_key) DO UPDATE SET
			workflow_id = EXCLUDED.workflow_id,
			agent_profile_id = EXCLUDED.agent_profile_id,
			session_id = EXCLUDED.session_id,
			operation_id = EXCLUDED.operation_id,
			updated_at = EXCLUDED.updated_at
		WHERE (
			(task_workflow_session_bindings.operation_id LIKE 'workflow-step-entry-v2:%'
				AND (
					task_workflow_session_bindings.operation_id < EXCLUDED.operation_id
					OR (
						task_workflow_session_bindings.operation_id = EXCLUDED.operation_id
						AND task_workflow_session_bindings.updated_at < EXCLUDED.updated_at
					)
				))
			OR (task_workflow_session_bindings.operation_id NOT LIKE 'workflow-step-entry-v2:%'
				AND task_workflow_session_bindings.updated_at < EXCLUDED.updated_at)
		)
	`
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query),
		binding.TaskID,
		binding.TargetKey,
		binding.WorkflowID,
		binding.AgentProfileID,
		nullableBindingSessionID(binding.SessionID),
		binding.OperationID,
		binding.UpdatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("upsert workflow session binding: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func nullableBindingSessionID(sessionID string) interface{} {
	if sessionID == "" {
		return nil
	}
	return sessionID
}
