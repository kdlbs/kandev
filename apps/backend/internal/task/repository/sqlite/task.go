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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), task.ID, task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
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
	task := &models.Task{}
	var metadata string
	var archivedAt sql.NullTime
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, archived_at, created_at, updated_at
		FROM tasks WHERE id = ?
	`), id).Scan(&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID, &task.Title, &task.Description, &task.State, &task.Priority, &task.Position, &metadata, &archivedAt, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if archivedAt.Valid {
		task.ArchivedAt = &archivedAt.Time
	}
	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// UpdateTask updates an existing task
func (r *Repository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`), task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.UpdatedAt, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	return nil
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

// ListTasks returns all non-archived tasks for a workflow
func (r *Repository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasks")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, archived_at, created_at, updated_at
		FROM tasks
		WHERE workflow_id = ? AND archived_at IS NULL
		ORDER BY position
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// CountTasksByWorkflow returns the number of non-archived tasks in a workflow
func (r *Repository) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_id = ? AND archived_at IS NULL`), workflowID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountTasksByWorkflowStep returns the number of non-archived tasks in a workflow step
func (r *Repository) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ? AND archived_at IS NULL`), stepID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListTasksByWorkflowStep returns all non-archived tasks in a workflow step
func (r *Repository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, archived_at, created_at, updated_at
		FROM tasks
		WHERE workflow_step_id = ? AND archived_at IS NULL ORDER BY position
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
func (r *Repository) ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int, includeArchived bool) ([]*models.Task, int, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasksByWorkspace")
	defer span.End()
	// Calculate offset
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	archiveFilter := ""
	if !includeArchived {
		archiveFilter = " AND archived_at IS NULL"
	}

	var rows *sql.Rows
	var total int
	var err error

	if query == "" {
		rows, total, err = r.queryAllTasks(ctx, workspaceID, archiveFilter, pageSize, offset)
	} else {
		rows, total, err = r.searchTasks(ctx, workspaceID, query, archiveFilter, pageSize, offset, includeArchived)
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
func (r *Repository) queryAllTasks(ctx context.Context, workspaceID, archiveFilter string, pageSize, offset int) (*sql.Rows, int, error) {
	var total int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`+archiveFilter), workspaceID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, archived_at, created_at, updated_at
		FROM tasks
		WHERE workspace_id = ?`+archiveFilter+`
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`), workspaceID, pageSize, offset)
	return rows, total, err
}

// searchTasks fetches tasks matching a search query for a workspace with pagination.
func (r *Repository) searchTasks(ctx context.Context, workspaceID, query, archiveFilter string, pageSize, offset int, includeArchived bool) (*sql.Rows, int, error) {
	searchPattern := "%" + query + "%"
	like := dialect.Like(r.ro.DriverName())

	tFilter := ""
	if !includeArchived {
		tFilter = " AND t.archived_at IS NULL"
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
	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(countQuery), workspaceID, searchPattern, searchPattern, searchPattern, searchPattern).Scan(&total); err != nil {
		return nil, 0, err
	}

	selectQuery := fmt.Sprintf(`
		SELECT DISTINCT t.id, t.workspace_id, t.workflow_id, t.workflow_step_id, t.title, t.description, t.state, t.priority, t.position, t.metadata, t.archived_at, t.created_at, t.updated_at
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
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(selectQuery), workspaceID, searchPattern, searchPattern, searchPattern, searchPattern, pageSize, offset)
	return rows, total, err
}

// scanTasks is a helper to scan task rows
func (r *Repository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var archivedAt sql.NullTime
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.WorkflowID,
			&task.WorkflowStepID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&task.Position,
			&metadata,
			&archivedAt,
			&task.CreatedAt,
			&task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			task.ArchivedAt = &archivedAt.Time
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
		SELECT t.id, t.workspace_id, t.workflow_id, t.workflow_step_id, t.title, t.description, t.state, t.priority, t.position, t.metadata, t.archived_at, t.created_at, t.updated_at
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
