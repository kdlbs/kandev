package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// CreateWorkspace creates a new workspace
func (r *Repository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now
	if workspace.TaskPrefix == "" {
		workspace.TaskPrefix = "KAN"
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspaces (
			id,
			name,
			description,
			owner_id,
			default_executor_id,
			default_environment_id,
			default_agent_profile_id,
			default_config_agent_profile_id,
			task_prefix,
			task_sequence,
			office_workflow_id,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), workspace.ID, workspace.Name, workspace.Description, workspace.OwnerID, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.DefaultConfigAgentProfileID, workspace.TaskPrefix, workspace.TaskSequence, workspace.OfficeWorkflowID, workspace.CreatedAt, workspace.UpdatedAt)

	return err
}

// GetWorkspace retrieves a workspace by ID
func (r *Repository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	workspace := &models.Workspace{}
	var defaultExecutorID sql.NullString
	var defaultEnvironmentID sql.NullString
	var defaultAgentProfileID sql.NullString
	var defaultConfigAgentProfileID sql.NullString

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, description, owner_id, default_executor_id, default_environment_id, default_agent_profile_id, default_config_agent_profile_id, task_prefix, task_sequence, office_workflow_id, created_at, updated_at
		FROM workspaces WHERE id = ?
	`), id).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.Description,
		&workspace.OwnerID,
		&defaultExecutorID,
		&defaultEnvironmentID,
		&defaultAgentProfileID,
		&defaultConfigAgentProfileID,
		&workspace.TaskPrefix,
		&workspace.TaskSequence,
		&workspace.OfficeWorkflowID,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)
	if defaultExecutorID.Valid && defaultExecutorID.String != "" {
		workspace.DefaultExecutorID = &defaultExecutorID.String
	}
	if defaultEnvironmentID.Valid && defaultEnvironmentID.String != "" {
		workspace.DefaultEnvironmentID = &defaultEnvironmentID.String
	}
	if defaultAgentProfileID.Valid && defaultAgentProfileID.String != "" {
		workspace.DefaultAgentProfileID = &defaultAgentProfileID.String
	}
	if defaultConfigAgentProfileID.Valid && defaultConfigAgentProfileID.String != "" {
		workspace.DefaultConfigAgentProfileID = &defaultConfigAgentProfileID.String
	}

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return workspace, err
}

// UpdateWorkspace updates an existing workspace
func (r *Repository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	workspace.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workspaces
		SET name = ?,
			description = ?,
			default_executor_id = ?,
			default_environment_id = ?,
			default_agent_profile_id = ?,
			default_config_agent_profile_id = ?,
			updated_at = ?
		WHERE id = ?
	`), workspace.Name, workspace.Description, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.DefaultConfigAgentProfileID, workspace.UpdatedAt, workspace.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace not found: %s", workspace.ID)
	}
	return nil
}

// DeleteWorkspace deletes a workspace by ID
func (r *Repository) DeleteWorkspace(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workspaces WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace not found: %s", id)
	}
	return nil
}

// DeleteWorkspaceCascade deletes a workspace and its task/workflow rows in one transaction.
func (r *Repository) DeleteWorkspaceCascade(
	ctx context.Context,
	id string,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, nil)
}

// DeleteWorkspaceCascadeWithName deletes a workspace and its task/workflow rows
// in one transaction, only if the current workspace name matches name.
func (r *Repository) DeleteWorkspaceCascadeWithName(
	ctx context.Context,
	id, name string,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, &name)
}

func (r *Repository) deleteWorkspaceCascade(
	ctx context.Context,
	id string,
	expectedName *string,
) ([]*models.Task, []*models.Workflow, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if expectedName != nil {
		if err := r.confirmWorkspaceNameForCascadeDelete(ctx, tx, id, *expectedName); err != nil {
			return nil, nil, err
		}
	}
	tasks, err := r.listWorkspaceCascadeDeleteTasks(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}
	workflows, err := r.listWorkspaceCascadeDeleteWorkflows(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}

	rows, err := r.deleteWorkspaceCascadeRow(ctx, tx, id, expectedName)
	if err != nil {
		return nil, nil, err
	}
	if rows == 0 {
		if expectedName != nil {
			return nil, nil, r.confirmedWorkspaceDeleteMismatch(ctx, tx, id)
		}
		return nil, nil, fmt.Errorf("workspace not found: %s", id)
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM tasks
		WHERE workspace_id = ?
	`), id); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflows
		WHERE workspace_id = ?
	`), id); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return tasks, workflows, nil
}

func (r *Repository) deleteWorkspaceCascadeRow(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
	expectedName *string,
) (int64, error) {
	var result sql.Result
	var err error
	if expectedName == nil {
		result, err = tx.ExecContext(ctx, r.db.Rebind(`
			DELETE FROM workspaces
			WHERE id = ?
		`), id)
	} else {
		// Keep the name predicate on the delete itself so a rename racing after
		// the confirmation cannot delete the newly renamed workspace.
		result, err = tx.ExecContext(ctx, r.db.Rebind(`
			DELETE FROM workspaces
			WHERE id = ? AND name = ?
		`), id, *expectedName)
	}
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

func (r *Repository) confirmWorkspaceNameForCascadeDelete(ctx context.Context, tx *sqlx.Tx, id, name string) error {
	currentName, err := r.workspaceNameForCascadeDelete(ctx, tx, id)
	if err != nil {
		return err
	}
	if currentName != name {
		return repoerrors.ErrWorkspaceNameMismatch
	}
	return nil
}

func (r *Repository) workspaceNameForCascadeDelete(ctx context.Context, tx *sqlx.Tx, id string) (string, error) {
	var currentName string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT name
		FROM workspaces
		WHERE id = ?
	`), id).Scan(&currentName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("workspace not found: %s", id)
	}
	return currentName, err
}

func (r *Repository) confirmedWorkspaceDeleteMismatch(ctx context.Context, tx *sqlx.Tx, id string) error {
	if _, err := r.workspaceNameForCascadeDelete(ctx, tx, id); err != nil {
		return err
	}
	return repoerrors.ErrWorkspaceNameMismatch
}

func (r *Repository) listWorkspaceCascadeDeleteTasks(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
) ([]*models.Task, error) {
	rows, err := tx.QueryContext(ctx, r.db.Rebind(fmt.Sprintf(`
		SELECT %s
		FROM tasks
		WHERE workspace_id = ?
		ORDER BY created_at ASC, id ASC
	`, taskSelectColumns("tasks"))), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

func (r *Repository) listWorkspaceCascadeDeleteWorkflows(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
) ([]*models.Workflow, error) {
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`
		SELECT `+workflowSelectColumns+`
		FROM workflows
		WHERE workspace_id = ?
		ORDER BY sort_order ASC, created_at ASC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanWorkflowRows(rows)
}

// ListWorkspaces returns all workspaces
func (r *Repository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, description, owner_id, default_executor_id, default_environment_id, default_agent_profile_id, default_config_agent_profile_id, task_prefix, task_sequence, office_workflow_id, created_at, updated_at
		FROM workspaces ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Workspace
	for rows.Next() {
		workspace := &models.Workspace{}
		var defaultExecutorID sql.NullString
		var defaultEnvironmentID sql.NullString
		var defaultAgentProfileID sql.NullString
		var defaultConfigAgentProfileID sql.NullString
		if err := rows.Scan(
			&workspace.ID,
			&workspace.Name,
			&workspace.Description,
			&workspace.OwnerID,
			&defaultExecutorID,
			&defaultEnvironmentID,
			&defaultAgentProfileID,
			&defaultConfigAgentProfileID,
			&workspace.TaskPrefix,
			&workspace.TaskSequence,
			&workspace.OfficeWorkflowID,
			&workspace.CreatedAt,
			&workspace.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if defaultExecutorID.Valid && defaultExecutorID.String != "" {
			workspace.DefaultExecutorID = &defaultExecutorID.String
		}
		if defaultEnvironmentID.Valid && defaultEnvironmentID.String != "" {
			workspace.DefaultEnvironmentID = &defaultEnvironmentID.String
		}
		if defaultAgentProfileID.Valid && defaultAgentProfileID.String != "" {
			workspace.DefaultAgentProfileID = &defaultAgentProfileID.String
		}
		if defaultConfigAgentProfileID.Valid && defaultConfigAgentProfileID.String != "" {
			workspace.DefaultConfigAgentProfileID = &defaultConfigAgentProfileID.String
		}
		result = append(result, workspace)
	}
	return result, rows.Err()
}
