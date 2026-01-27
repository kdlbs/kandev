package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	commonsqlite "github.com/kandev/kandev/internal/common/sqlite"
	"github.com/kandev/kandev/internal/task/models"
)

// CreateEnvironment creates a new environment
func (r *Repository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
	if environment.ID == "" {
		environment.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	environment.CreatedAt = now
	environment.UpdatedAt = now

	buildConfigJSON, err := json.Marshal(environment.BuildConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize environment build config: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, environment.ID, environment.Name, environment.Kind, commonsqlite.BoolToInt(environment.IsSystem), environment.WorktreeRoot, environment.ImageTag, environment.Dockerfile, string(buildConfigJSON), environment.CreatedAt, environment.UpdatedAt, environment.DeletedAt)
	return err
}

// GetEnvironment retrieves an environment by ID
func (r *Repository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	environment := &models.Environment{}
	var buildConfigJSON string
	var isSystem int

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at
		FROM environments WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&environment.ID, &environment.Name, &environment.Kind, &isSystem, &environment.WorktreeRoot,
		&environment.ImageTag, &environment.Dockerfile, &buildConfigJSON,
		&environment.CreatedAt, &environment.UpdatedAt, &environment.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	environment.IsSystem = isSystem == 1
	if buildConfigJSON != "" && buildConfigJSON != "{}" {
		if err := json.Unmarshal([]byte(buildConfigJSON), &environment.BuildConfig); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment build config: %w", err)
		}
	}
	return environment, nil
}

// UpdateEnvironment updates an existing environment
func (r *Repository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	environment.UpdatedAt = time.Now().UTC()

	buildConfigJSON, err := json.Marshal(environment.BuildConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize environment build config: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE environments SET name = ?, kind = ?, is_system = ?, worktree_root = ?, image_tag = ?, dockerfile = ?, build_config = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, environment.Name, environment.Kind, commonsqlite.BoolToInt(environment.IsSystem), environment.WorktreeRoot, environment.ImageTag, environment.Dockerfile, string(buildConfigJSON), environment.UpdatedAt, environment.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("environment not found: %s", environment.ID)
	}
	return nil
}

// DeleteEnvironment soft-deletes an environment by ID
func (r *Repository) DeleteEnvironment(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE environments SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("environment not found: %s", id)
	}
	return nil
}

// ListEnvironments returns all non-deleted environments
func (r *Repository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at
		FROM environments WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Environment
	for rows.Next() {
		environment := &models.Environment{}
		var buildConfigJSON string
		var isSystem int
		if err := rows.Scan(
			&environment.ID, &environment.Name, &environment.Kind, &isSystem, &environment.WorktreeRoot,
			&environment.ImageTag, &environment.Dockerfile, &buildConfigJSON,
			&environment.CreatedAt, &environment.UpdatedAt, &environment.DeletedAt,
		); err != nil {
			return nil, err
		}
		environment.IsSystem = isSystem == 1
		if buildConfigJSON != "" && buildConfigJSON != "{}" {
			if err := json.Unmarshal([]byte(buildConfigJSON), &environment.BuildConfig); err != nil {
				return nil, fmt.Errorf("failed to deserialize environment build config: %w", err)
			}
		}
		result = append(result, environment)
	}
	return result, rows.Err()
}

