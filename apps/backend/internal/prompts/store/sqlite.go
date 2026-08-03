package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/prompts/models"
)

const (
	builtinChangesWalkthroughPromptID = "builtin-changes-walkthrough"
	// Historical prompt hashes use the same TrimSpace normalization as promptcfg.Get.
	changesWalkthroughV1SHA256 = "23a82694ef3b6d0220da2879c1c351cf5ee4926c2bc54a52fa4f7d5182bcb111"
	changesWalkthroughV2SHA256 = "7a28dc81df4bff75b4fb8d66d6b9118febe5daf7f4b570e3b8ef8c74ac3e3146"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

func newSQLiteRepository(writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
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
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	// Seed built-in prompts
	if err := r.seedBuiltinPrompts(); err != nil {
		return fmt.Errorf("failed to seed built-in prompts: %w", err)
	}

	return nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		ORDER BY builtin DESC, name ASC
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
		var builtinInt int
		if err := rows.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
			return nil, err
		}
		prompt.Builtin = builtinInt == 1
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prompts, nil
}

func (r *sqliteRepository) GetPromptByID(ctx context.Context, id string) (*models.Prompt, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE id = ?
	`), id)
	prompt := &models.Prompt{}
	var builtinInt int
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
	return prompt, nil
}

func (r *sqliteRepository) GetPromptByName(ctx context.Context, name string) (*models.Prompt, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE name = ?
	`), name)
	prompt := &models.Prompt{}
	var builtinInt int
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
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

	builtinInt := 0
	if prompt.Builtin {
		builtinInt = 1
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), prompt.ID, prompt.Name, prompt.Content, builtinInt, prompt.CreatedAt, prompt.UpdatedAt)
	return err
}

func (r *sqliteRepository) UpdatePrompt(ctx context.Context, prompt *models.Prompt) error {
	if prompt == nil {
		return errors.New("prompt is nil")
	}
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Content = strings.TrimSpace(prompt.Content)
	prompt.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE custom_prompts
		SET name = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), prompt.Name, prompt.Content, prompt.UpdatedAt, prompt.ID)
	return err
}

func (r *sqliteRepository) DeletePrompt(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM custom_prompts WHERE id = ?`), id)
	return err
}

// seedBuiltinPrompts inserts the default built-in prompts on first run.
// Existing prompts are not overwritten, so user customizations are preserved.
func (r *sqliteRepository) seedBuiltinPrompts() error {
	for _, prompt := range r.getBuiltinPrompts() {
		_, err := r.db.Exec(r.db.Rebind(`
			INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?)
			ON CONFLICT DO NOTHING
		`), prompt.ID, prompt.Name, prompt.Content, prompt.CreatedAt, prompt.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert built-in prompt %s: %w", prompt.ID, err)
		}
		if prompt.ID == builtinChangesWalkthroughPromptID {
			if err := r.refreshLegacyChangesWalkthroughPrompt(prompt); err != nil {
				return err
			}
		}
	}
	return nil
}

// refreshLegacyChangesWalkthroughPrompt updates exact, untouched revisions
// shipped before the required step shape was documented. User edits change
// updated_at, and the conditional update prevents racing a concurrent edit.
func (r *sqliteRepository) refreshLegacyChangesWalkthroughPrompt(current *models.Prompt) error {
	var storedContent string
	var builtinInt int
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow(r.db.Rebind(`
		SELECT content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE id = ?
	`), current.ID).Scan(&storedContent, &builtinInt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read stored changes walkthrough prompt: %w", err)
	}
	if builtinInt != 1 || !createdAt.Equal(updatedAt) || !isLegacyChangesWalkthroughPrompt(storedContent) {
		return nil
	}

	_, err = r.db.Exec(r.db.Rebind(`
		UPDATE custom_prompts
		SET content = ?
		WHERE id = ? AND builtin = 1 AND content = ? AND created_at = ? AND updated_at = ?
	`), current.Content, current.ID, storedContent, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("refresh stored changes walkthrough prompt: %w", err)
	}
	return nil
}

func isLegacyChangesWalkthroughPrompt(content string) bool {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	switch hash {
	case changesWalkthroughV1SHA256, changesWalkthroughV2SHA256:
		return true
	default:
		return false
	}
}

// getBuiltinPrompts returns the predefined built-in prompts loaded from embedded markdown files.
func (r *sqliteRepository) getBuiltinPrompts() []*models.Prompt {
	now := time.Now().UTC()
	return []*models.Prompt{
		{ID: "builtin-code-review", Name: "code-review", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("code-review")},
		{ID: "builtin-open-pr", Name: "open-pr", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("open-pr")},
		{ID: "builtin-merge-base", Name: "merge-base", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("merge-base")},
		{ID: "builtin-ci-auto-fix", Name: "ci-auto-fix", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("ci-auto-fix")},
		{ID: builtinChangesWalkthroughPromptID, Name: "changes-walkthrough", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("changes-walkthrough")},
	}
}
