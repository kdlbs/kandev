package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// taskColumns is the common column list for all task SELECT queries.
const taskColumns = `id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, is_ephemeral, parent_id, archived_at, created_at, updated_at, assignee_agent_instance_id, origin, project_id, requires_approval, execution_policy, execution_state, labels, identifier`

// CreateTask creates a new task
func (r *Repository) CreateTask(ctx context.Context, task *models.Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}
	if task.Labels == "" {
		task.Labels = "[]"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, is_ephemeral, parent_id, created_at, updated_at, assignee_agent_instance_id, origin, project_id, requires_approval, execution_policy, execution_state, labels, identifier)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), task.ID, task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.IsEphemeral, task.ParentID, task.CreatedAt, task.UpdatedAt, task.AssigneeAgentInstanceID, task.Origin, task.ProjectID, task.RequiresApproval, task.ExecutionPolicy, task.ExecutionState, task.Labels, task.Identifier)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to rollback task insert: %w", rollbackErr)
		}
		return err
	}

	return tx.Commit()
}

// GetTask retrieves a task by ID
func (r *Repository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskColumns+` FROM tasks WHERE id = ?`), id)
	task, err := r.scanSingleTask(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, err
}

// UpdateTask updates an existing task
func (r *Repository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, parent_id = ?, updated_at = ?, assignee_agent_instance_id = ?, origin = ?, project_id = ?, requires_approval = ?, execution_policy = ?, execution_state = ?, labels = ?, identifier = ?
		WHERE id = ?
	`), task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.ParentID, task.UpdatedAt, task.AssigneeAgentInstanceID, task.Origin, task.ProjectID, task.RequiresApproval, task.ExecutionPolicy, task.ExecutionState, task.Labels, task.Identifier, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	return nil
}

// DeleteTasksByWorkspace deletes all tasks for a workspace (E2E cleanup).
// Relies on CASCADE foreign keys to remove sessions, messages, turns, etc.
func (r *Repository) DeleteTasksByWorkspace(ctx context.Context, workspaceID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM tasks WHERE workspace_id = ?`), workspaceID)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

// DeleteTask deletes a task by ID
func (r *Repository) DeleteTask(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM tasks WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasks returns all non-archived, non-ephemeral tasks for a workflow
func (r *Repository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasks")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE workflow_id = ? AND archived_at IS NULL AND is_ephemeral = 0
		ORDER BY created_at ASC
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// CountTasksByWorkflow returns the number of non-archived, non-ephemeral tasks in a workflow
func (r *Repository) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_id = ? AND archived_at IS NULL AND is_ephemeral = 0`), workflowID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountTasksByWorkflowStep returns the number of non-archived, non-ephemeral tasks in a workflow step
func (r *Repository) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ? AND archived_at IS NULL AND is_ephemeral = 0`), stepID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListTasksByWorkflowStep returns all non-archived, non-ephemeral tasks in a workflow step
func (r *Repository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE workflow_step_id = ? AND archived_at IS NULL AND is_ephemeral = 0 ORDER BY created_at ASC
	`), workflowStepID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// ListTasksByWorkspace returns paginated tasks for a workspace with total count
// If query is non-empty, filters by task title, description, repository name, or repository path
// If includeArchived is false, archived tasks are excluded
// If includeEphemeral is false, ephemeral tasks are excluded
// If onlyEphemeral is true, only ephemeral tasks are returned
func (r *Repository) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasksByWorkspace")
	defer span.End()
	// Calculate offset
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// Build filter conditions
	filter := ""
	if onlyEphemeral {
		// Only ephemeral tasks
		filter += " AND is_ephemeral = 1"
	} else if !includeEphemeral {
		// Exclude ephemeral tasks
		filter += " AND is_ephemeral = 0"
	}
	// If includeEphemeral is true and onlyEphemeral is false, include both

	if !includeArchived {
		filter += " AND archived_at IS NULL"
	}

	if excludeConfig {
		filter += " AND (metadata IS NULL OR json_extract(metadata, '$.config_mode') IS NOT 1)"
	}

	var rows *sql.Rows
	var total int
	var err error

	if query == "" {
		rows, total, err = r.queryAllTasks(ctx, workspaceID, filter, workflowID, repositoryID, pageSize, offset)
	} else {
		rows, total, err = r.searchTasks(ctx, workspaceID, query, filter, workflowID, repositoryID, pageSize, offset, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
	}

	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	tasks, err := r.scanTasks(rows)
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// queryAllTasks fetches all tasks (no search) for a workspace with pagination.
func (r *Repository) queryAllTasks(ctx context.Context, workspaceID, taskFilter, workflowID, repositoryID string, pageSize, offset int) (*sql.Rows, int, error) {
	args := []interface{}{workspaceID}
	if workflowID != "" {
		taskFilter += " AND workflow_id = ?"
		args = append(args, workflowID)
	}
	if repositoryID != "" {
		taskFilter += " AND id IN (SELECT task_id FROM task_repositories WHERE repository_id = ?)"
		args = append(args, repositoryID)
	}
	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`+taskFilter), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE workspace_id = ?`+taskFilter+`
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`), append(append([]interface{}{}, args...), pageSize, offset)...)
	return rows, total, err
}

// searchTasks fetches tasks matching a search query for a workspace with pagination.
func (r *Repository) searchTasks(ctx context.Context, workspaceID, query, filter, workflowID, repositoryID string, pageSize, offset int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) (*sql.Rows, int, error) {
	searchPattern := "%" + query + "%"
	like := dialect.Like(r.ro.DriverName())

	// Build task filter
	tFilter := ""
	if onlyEphemeral {
		tFilter += " AND t.is_ephemeral = 1"
	} else if !includeEphemeral {
		tFilter += " AND t.is_ephemeral = 0"
	}
	if !includeArchived {
		tFilter += " AND t.archived_at IS NULL"
	}
	if excludeConfig {
		tFilter += " AND (t.metadata IS NULL OR json_extract(t.metadata, '$.config_mode') IS NOT 1)"
	}

	// Collect extra filter args in query-argument order
	var extraArgs []interface{}
	if workflowID != "" {
		tFilter += " AND t.workflow_id = ?"
		extraArgs = append(extraArgs, workflowID)
	}
	if repositoryID != "" {
		tFilter += " AND tr.repository_id = ?"
		extraArgs = append(extraArgs, repositoryID)
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT t.id) FROM tasks t
		LEFT JOIN task_repositories tr ON t.id = tr.task_id
		LEFT JOIN repositories r ON tr.repository_id = r.id
		WHERE t.workspace_id = ?%s
		AND (
			t.title %s ? OR
			t.description %s ? OR
			r.name %s ? OR
			r.local_path %s ?
		)
	`, tFilter, like, like, like, like)
	countArgs := append(append([]interface{}{workspaceID}, extraArgs...), searchPattern, searchPattern, searchPattern, searchPattern)
	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(countQuery), countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	selectQuery := fmt.Sprintf(`
		SELECT DISTINCT t.id, t.workspace_id, t.workflow_id, t.workflow_step_id, t.title, t.description, t.state, t.priority, t.position, t.metadata, t.is_ephemeral, t.parent_id, t.archived_at, t.created_at, t.updated_at, t.assignee_agent_instance_id, t.origin, t.project_id, t.requires_approval, t.execution_policy, t.execution_state, t.labels, t.identifier
		FROM tasks t
		LEFT JOIN task_repositories tr ON t.id = tr.task_id
		LEFT JOIN repositories r ON tr.repository_id = r.id
		WHERE t.workspace_id = ?%s
		AND (
			t.title %s ? OR
			t.description %s ? OR
			r.name %s ? OR
			r.local_path %s ?
		)
		ORDER BY t.updated_at DESC
		LIMIT ? OFFSET ?
	`, tFilter, like, like, like, like)
	selectArgs := append(append([]interface{}{}, countArgs...), pageSize, offset)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(selectQuery), selectArgs...)
	return rows, total, err
}

// scanSingleTask scans a single row into a Task.
func (r *Repository) scanSingleTask(row *sql.Row) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	var archivedAt sql.NullTime
	var identifier sql.NullString
	err := row.Scan(
		&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID,
		&task.Title, &task.Description, &task.State, &task.Priority, &task.Position,
		&metadata, &task.IsEphemeral, &task.ParentID, &archivedAt,
		&task.CreatedAt, &task.UpdatedAt,
		&task.AssigneeAgentInstanceID, &task.Origin, &task.ProjectID,
		&task.RequiresApproval, &task.ExecutionPolicy, &task.ExecutionState,
		&task.Labels, &identifier,
	)
	if err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		task.ArchivedAt = &archivedAt.Time
	}
	if identifier.Valid {
		task.Identifier = identifier.String
	}
	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// scanTasks is a helper to scan task rows
func (r *Repository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var archivedAt sql.NullTime
		var identifier sql.NullString
		err := rows.Scan(
			&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID,
			&task.Title, &task.Description, &task.State, &task.Priority, &task.Position,
			&metadata, &task.IsEphemeral, &task.ParentID, &archivedAt,
			&task.CreatedAt, &task.UpdatedAt,
			&task.AssigneeAgentInstanceID, &task.Origin, &task.ProjectID,
			&task.RequiresApproval, &task.ExecutionPolicy, &task.ExecutionState,
			&task.Labels, &identifier,
		)
		if err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			task.ArchivedAt = &archivedAt.Time
		}
		if identifier.Valid {
			task.Identifier = identifier.String
		}
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

// ArchiveTask sets the archived_at timestamp on a task
func (r *Repository) ArchiveTask(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?`), now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasksForAutoArchive returns tasks eligible for auto-archiving based on workflow step settings
func (r *Repository) ListTasksForAutoArchive(ctx context.Context) ([]*models.Task, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT t.id, t.workspace_id, t.workflow_id, t.workflow_step_id, t.title, t.description, t.state, t.priority, t.position, t.metadata, t.is_ephemeral, t.parent_id, t.archived_at, t.created_at, t.updated_at, t.assignee_agent_instance_id, t.origin, t.project_id, t.requires_approval, t.execution_policy, t.execution_state, t.labels, t.identifier
		FROM tasks t
		JOIN workflow_steps ws ON ws.id = t.workflow_step_id
		WHERE ws.auto_archive_after_hours > 0
			AND t.archived_at IS NULL
			AND t.updated_at <= %s
	`, dialect.NowMinusHours(drv, "ws.auto_archive_after_hours"))
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// UpdateTaskState updates the state of a task
func (r *Repository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`), state, time.Now().UTC(), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasksByProject returns all non-archived, non-ephemeral tasks for a project.
func (r *Repository) ListTasksByProject(ctx context.Context, projectID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE project_id = ? AND archived_at IS NULL AND is_ephemeral = 0
		ORDER BY created_at ASC
	`), projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListTasksByAssignee returns all non-archived, non-ephemeral tasks assigned to an agent instance.
func (r *Repository) ListTasksByAssignee(ctx context.Context, agentInstanceID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE assignee_agent_instance_id = ? AND archived_at IS NULL AND is_ephemeral = 0
		ORDER BY created_at ASC
	`), agentInstanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListTaskTree returns a flat list of non-archived tasks for a workspace, suitable for
// building a tree using each task's ParentID field.
func (r *Repository) ListTaskTree(ctx context.Context, workspaceID string, filters models.TaskTreeFilters) ([]*models.Task, error) {
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE workspace_id = ? AND archived_at IS NULL AND is_ephemeral = 0`
	args := []interface{}{workspaceID}

	if filters.ProjectID != "" {
		query += " AND project_id = ?"
		args = append(args, filters.ProjectID)
	}
	if filters.AssigneeID != "" {
		query += " AND assignee_agent_instance_id = ?"
		args = append(args, filters.AssigneeID)
	}
	if filters.WorkflowID != "" {
		query += " AND workflow_id = ?"
		args = append(args, filters.WorkflowID)
	}
	if filters.Origin != "" {
		query += " AND origin = ?"
		args = append(args, filters.Origin)
	}
	query += " ORDER BY created_at ASC"

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// IncrementTaskSequence atomically increments the workspace task_sequence and returns the new value.
func (r *Repository) IncrementTaskSequence(ctx context.Context, workspaceID string) (int, error) {
	var seq int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		UPDATE workspaces SET task_sequence = task_sequence + 1
		WHERE id = ?
		RETURNING task_sequence
	`), workspaceID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("increment task sequence for workspace %s: %w", workspaceID, err)
	}
	return seq, nil
}

// GetWorkspaceTaskPrefix returns the task prefix and orchestrate workflow ID for a workspace.
func (r *Repository) GetWorkspaceTaskPrefix(ctx context.Context, workspaceID string) (prefix, orchestrateWorkflowID string, err error) {
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(task_prefix, 'KAN'), COALESCE(orchestrate_workflow_id, '')
		FROM workspaces WHERE id = ?
	`), workspaceID).Scan(&prefix, &orchestrateWorkflowID)
	return
}
