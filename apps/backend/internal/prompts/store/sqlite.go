package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/prompts/models"
)

type sqliteRepository struct {
	db     *sql.DB
	ownsDB bool
}

func newSQLiteRepositoryWithDB(dbConn *sql.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(dbConn, false)
}

func newSQLiteRepository(dbConn *sql.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: dbConn, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := dbConn.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS custom_prompts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`
	_, err := r.db.Exec(schema)
	return err
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, content, created_at, updated_at
		FROM custom_prompts
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var prompts []*models.Prompt
	for rows.Next() {
		prompt := &models.Prompt{}
		if err := rows.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
			return nil, err
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prompts, nil
}

func (r *sqliteRepository) GetPromptByID(ctx context.Context, id string) (*models.Prompt, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, content, created_at, updated_at
		FROM custom_prompts
		WHERE id = ?
	`, id)
	prompt := &models.Prompt{}
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	return prompt, nil
}

func (r *sqliteRepository) GetPromptByName(ctx context.Context, name string) (*models.Prompt, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, content, created_at, updated_at
		FROM custom_prompts
		WHERE name = ?
	`, name)
	prompt := &models.Prompt{}
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	return prompt, nil
}

func (r *sqliteRepository) CreatePrompt(ctx context.Context, prompt *models.Prompt) error {
	if prompt.ID == "" {
		prompt.ID = uuid.New().String()
	}
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Content = strings.TrimSpace(prompt.Content)
	if prompt.CreatedAt.IsZero() {
		prompt.CreatedAt = time.Now().UTC()
	}
	prompt.UpdatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO custom_prompts (id, name, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, prompt.ID, prompt.Name, prompt.Content, prompt.CreatedAt, prompt.UpdatedAt)
	return err
}

func (r *sqliteRepository) UpdatePrompt(ctx context.Context, prompt *models.Prompt) error {
	if prompt == nil {
		return errors.New("prompt is nil")
	}
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Content = strings.TrimSpace(prompt.Content)
	prompt.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE custom_prompts
		SET name = ?, content = ?, updated_at = ?
		WHERE id = ?
	`, prompt.Name, prompt.Content, prompt.UpdatedAt, prompt.ID)
	return err
}

func (r *sqliteRepository) DeletePrompt(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM custom_prompts WHERE id = ?`, id)
	return err
}
