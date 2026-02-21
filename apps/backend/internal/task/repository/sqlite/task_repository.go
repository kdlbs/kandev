package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateTaskRepository creates a new task-repository link
func (r *Repository) CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	if taskRepo.ID == "" {
		taskRepo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	taskRepo.CreatedAt = now
	taskRepo.UpdatedAt = now

	metadataJSON, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_repositories (
			id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), taskRepo.ID, taskRepo.TaskID, taskRepo.RepositoryID, taskRepo.BaseBranch, taskRepo.Position, string(metadataJSON), taskRepo.CreatedAt, taskRepo.UpdatedAt)
	return err
}

// GetTaskRepository retrieves a task-repository link by ID
func (r *Repository) GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error) {
	taskRepo := &models.TaskRepository{}
	var metadataJSON string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		FROM task_repositories WHERE id = ?
	`), id).Scan(
		&taskRepo.ID,
		&taskRepo.TaskID,
		&taskRepo.RepositoryID,
		&taskRepo.BaseBranch,
		&taskRepo.Position,
		&metadataJSON,
		&taskRepo.CreatedAt,
		&taskRepo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task repository not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
		}
	}
	return taskRepo, nil
}

// ListTaskRepositories returns all repository links for a task
func (r *Repository) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		FROM task_repositories
		WHERE task_id = ?
		ORDER BY position ASC, created_at ASC
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.TaskRepository
	for rows.Next() {
		taskRepo := &models.TaskRepository{}
		var metadataJSON string
		if err := rows.Scan(
			&taskRepo.ID,
			&taskRepo.TaskID,
			&taskRepo.RepositoryID,
			&taskRepo.BaseBranch,
			&taskRepo.Position,
			&metadataJSON,
			&taskRepo.CreatedAt,
			&taskRepo.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
			}
		}
		result = append(result, taskRepo)
	}
	return result, rows.Err()
}

// UpdateTaskRepository updates an existing task-repository link
func (r *Repository) UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	taskRepo.UpdatedAt = time.Now().UTC()

	metadataJSON, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_repositories SET
			task_id = ?, repository_id = ?, base_branch = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`), taskRepo.TaskID, taskRepo.RepositoryID, taskRepo.BaseBranch, taskRepo.Position, string(metadataJSON), taskRepo.UpdatedAt, taskRepo.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task repository not found: %s", taskRepo.ID)
	}
	return nil
}

// DeleteTaskRepository deletes a task-repository link by ID
func (r *Repository) DeleteTaskRepository(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_repositories WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task repository not found: %s", id)
	}
	return nil
}

// ListTaskRepositoriesByTaskIDs returns all repository links for the given task IDs,
// grouped by task ID. This eliminates N+1 queries when loading repositories for multiple tasks.
func (r *Repository) ListTaskRepositoriesByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskRepository, error) {
	result := make(map[string][]*models.TaskRepository, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		FROM task_repositories
		WHERE task_id IN (%s)
		ORDER BY position ASC, created_at ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		taskRepo := &models.TaskRepository{}
		var metadataJSON string
		if err := rows.Scan(
			&taskRepo.ID,
			&taskRepo.TaskID,
			&taskRepo.RepositoryID,
			&taskRepo.BaseBranch,
			&taskRepo.Position,
			&metadataJSON,
			&taskRepo.CreatedAt,
			&taskRepo.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
			}
		}
		result[taskRepo.TaskID] = append(result[taskRepo.TaskID], taskRepo)
	}
	return result, rows.Err()
}

// DeleteTaskRepositoriesByTask deletes all repository links for a task
func (r *Repository) DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_repositories WHERE task_id = ?`), taskID)
	return err
}

// GetPrimaryTaskRepository returns the first (primary) repository for a task
func (r *Repository) GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error) {
	repos, err := r.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, nil
	}
	return repos[0], nil
}
