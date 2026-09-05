package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// CreateWorkspace creates a new workspace
func (r *Repository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	r.prepareWorkspace(workspace)
	return r.insertWorkspace(ctx, r.db, workspace)
}

func (r *Repository) prepareWorkspace(workspace *models.Workspace) {
	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now
	if workspace.TaskPrefix == "" {
		workspace.TaskPrefix = "KAN"
	}
}

func (r *Repository) insertWorkspace(ctx context.Context, exec sqlx.ExtContext, workspace *models.Workspace) error {
	_, err := exec.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspaces (
			id,
			name,
			description,
			owner_id,
			org_id,
			unit_id,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), workspace.ID, workspace.Name, workspace.Description, workspace.OwnerID, workspace.OrgID, workspace.UnitID, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.DefaultConfigAgentProfileID, workspace.TaskPrefix, workspace.TaskSequence, workspace.OfficeWorkflowID, workspace.CreatedAt, workspace.UpdatedAt)

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
		SELECT id, name, description, owner_id, org_id, unit_id, default_executor_id, default_environment_id, default_agent_profile_id, default_config_agent_profile_id, task_prefix, task_sequence, office_workflow_id, created_at, updated_at
		FROM workspaces WHERE id = ?
	`), id).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.Description,
		&workspace.OwnerID,
		&workspace.OrgID,
		&workspace.UnitID,
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
		return nil, workspaceNotFoundError(id)
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
			unit_id = ?,
			default_executor_id = ?,
			default_environment_id = ?,
			default_agent_profile_id = ?,
			default_config_agent_profile_id = ?,
			updated_at = ?
		WHERE id = ?
	`), workspace.Name, workspace.Description, workspace.UnitID, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.DefaultConfigAgentProfileID, workspace.UpdatedAt, workspace.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return workspaceNotFoundError(workspace.ID)
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
		return workspaceNotFoundError(id)
	}
	return nil
}

// DeleteWorkspaceCascade deletes a workspace and its task/workflow rows in one transaction.
func (r *Repository) DeleteWorkspaceCascade(
	ctx context.Context,
	id string,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, nil, nil)
}

// DeleteWorkspaceCascadeWithName deletes a workspace and its task/workflow rows
// in one transaction, only if the current workspace name matches name.
func (r *Repository) DeleteWorkspaceCascadeWithName(
	ctx context.Context,
	id, name string,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, &name, nil)
}

// DeleteWorkspaceCascadeWithSecretCleanup deletes the workspace cascade and
// invokes cleanup on the same transaction before commit.
func (r *Repository) DeleteWorkspaceCascadeWithSecretCleanup(
	ctx context.Context,
	id string,
	cleanup func(context.Context, *sqlx.Tx) error,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, nil, cleanup)
}

// DeleteWorkspaceCascadeWithNameAndSecretCleanup is the confirmation-aware
// transactional variant of DeleteWorkspaceCascadeWithName.
func (r *Repository) DeleteWorkspaceCascadeWithNameAndSecretCleanup(
	ctx context.Context,
	id, name string,
	cleanup func(context.Context, *sqlx.Tx) error,
) ([]*models.Task, []*models.Workflow, error) {
	return r.deleteWorkspaceCascade(ctx, id, &name, cleanup)
}

func (r *Repository) deleteWorkspaceCascade(
	ctx context.Context,
	id string,
	expectedName *string,
	cleanup func(context.Context, *sqlx.Tx) error,
) ([]*models.Task, []*models.Workflow, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the workspace row BEFORE inventorying its tasks: task creation
	// takes the same lock, so a task created after this point either commits
	// before the inventory (and is purged with the rest) or blocks until the
	// cascade commits, when the workspace is gone and its insert fails. An
	// unlocked inventory could miss a task created mid-cascade and leave its
	// queued messages orphaned after the task rows are deleted.
	if err := r.lockWorkspaceRowInTx(ctx, tx, id); err != nil {
		return nil, nil, err
	}
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
	// Establish the global lock order task-row -> queue-session before
	// purging the queues: lifecycle admission takes the task row first and
	// then the session lock, so taking session locks first here would invert
	// the order and deadlock the two on Postgres. Guard every affected task
	// row in stable sorted id order, WITHOUT reordering the inventory returned
	// to the caller (listWorkspaceCascadeDeleteTasks orders created_at ASC,
	// id ASC and the service consumes that order).
	lockIDs := make([]string, len(tasks))
	for i, task := range tasks {
		lockIDs[i] = task.ID
	}
	sort.Strings(lockIDs)
	for _, taskID := range lockIDs {
		if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
			return nil, nil, fmt.Errorf("guard cascade task row %s: %w", taskID, err)
		}
	}
	if err := r.purgeWorkspaceTaskQueuesInTx(ctx, tx, tasks); err != nil {
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
		return nil, nil, workspaceNotFoundError(id)
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM tasks
		WHERE workspace_id = ?
	`), id); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflow_steps
		WHERE workflow_id IN (
			SELECT id
			FROM workflows
			WHERE workspace_id = ?
		)
	`), id); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflows
		WHERE workspace_id = ?
	`), id); err != nil {
		return nil, nil, err
	}
	if cleanup != nil {
		if err := cleanup(ctx, tx); err != nil {
			return nil, nil, fmt.Errorf("workspace secret cleanup: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	for _, task := range tasks {
		r.notifyTaskQueuePurged(ctx, task.ID)
	}
	return tasks, workflows, nil
}

func (r *Repository) purgeWorkspaceTaskQueuesInTx(ctx context.Context, tx *sqlx.Tx, tasks []*models.Task) error {
	for _, task := range tasks {
		// The tasks still exist at this point (deletion happens later in the
		// cascade), so the authoritative session set is discoverable now.
		sessions, err := r.taskQueueSessionsInTx(ctx, tx, task.ID)
		if err != nil {
			return fmt.Errorf("task queue sessions for cascade task %s: %w", task.ID, err)
		}
		if err := r.purgeTaskQueueInTx(ctx, tx, task.ID, sessions); err != nil {
			return fmt.Errorf("purge task queue for workspace cascade task %s: %w", task.ID, err)
		}
	}
	return nil
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
		return "", workspaceNotFoundError(id)
	}
	return currentName, err
}

func workspaceNotFoundError(id string) error {
	return fmt.Errorf("%w: %s", repoerrors.ErrWorkspaceNotFound, id)
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
// ClaimUnownedWorkspaces assigns every pre-auth workspace (empty owner_id) to
// ownerID. Called by the auth setup wizard when promoting the instance's
// single user to the admin account; idempotent.
func (r *Repository) ClaimUnownedWorkspaces(ctx context.Context, ownerID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workspaces SET owner_id = ? WHERE owner_id = '' OR owner_id IS NULL
	`), ownerID)
	return err
}

func (r *Repository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, description, owner_id, org_id, unit_id, default_executor_id, default_environment_id, default_agent_profile_id, default_config_agent_profile_id, task_prefix, task_sequence, office_workflow_id, created_at, updated_at
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
			&workspace.OrgID,
			&workspace.UnitID,
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
