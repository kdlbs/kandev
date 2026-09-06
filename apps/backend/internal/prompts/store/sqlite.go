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
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/prompts/models"
)

const (
	builtinChangesWalkthroughPromptID = "builtin-changes-walkthrough"
	builtinCIAutoFixPromptID          = "builtin-ci-auto-fix"
	// Historical prompt hashes use the same TrimSpace normalization as promptcfg.Get.
	changesWalkthroughV1SHA256 = "23a82694ef3b6d0220da2879c1c351cf5ee4926c2bc54a52fa4f7d5182bcb111"
	changesWalkthroughV2SHA256 = "7a28dc81df4bff75b4fb8d66d6b9118febe5daf7f4b570e3b8ef8c74ac3e3146"
	ciAutoFixV1SHA256          = "b86760225be4ba1d814b424fc975e8c44c2a41b552a1f417c6233ce4a42d6058"
	ciAutoFixV2SHA256          = "d96decbf95ef289f6fdb3141bfa22c26f91dcd44cf5ad241e9b29a8f39058cd2"
	ciAutoFixV3SHA256          = "3ae2242d19d3837af39afdca2cc7f9c0587a6c22c5279e5e1e5173279d00f472"
)
const maxPromptListItems = 2000

var ErrPromptListLimit = errors.New("prompt list limit exceeded")

var ErrPromptReferenceCandidateLimit = errors.New("prompt reference candidate limit exceeded")

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
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		ORDER BY builtin DESC, name ASC
		LIMIT ?
	`), maxPromptListItems+1)
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
	if len(prompts) > maxPromptListItems {
		return nil, ErrPromptListLimit
	}
	return prompts, nil
}

// ListPromptsForReferenceExpansion bounds rows and aggregate bytes materialized
// for inline reference resolution while reporting whether additional
// candidates exist.
func (r *sqliteRepository) ListPromptsForReferenceExpansion(
	ctx context.Context,
	limit, maxNameBytes, maxContentBytes, maxTotalNameBytes, maxTotalContentBytes int,
) ([]*models.Prompt, bool, error) {
	if limit < 1 {
		return nil, false, nil
	}
	nameLength := dialect.ByteLength(r.ro.DriverName(), "name")
	contentLength := dialect.ByteLength(r.ro.DriverName(), "content")
	query := fmt.Sprintf(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE %s > 0
			AND %s <= ?
			AND %s <= ?
		ORDER BY builtin DESC, name ASC
		LIMIT ?
	`, nameLength, nameLength, contentLength)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), maxNameBytes, maxContentBytes, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = rows.Close()
	}()

	prompts := make([]*models.Prompt, 0, limit)
	totalNameBytes := 0
	totalContentBytes := 0
	truncated := false
	for rows.Next() {
		prompt := &models.Prompt{}
		var builtinInt int
		if err := rows.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
			return nil, false, err
		}
		if len(prompts) >= limit {
			truncated = true
			break
		}
		if totalNameBytes+len(prompt.Name) > maxTotalNameBytes ||
			totalContentBytes+len(prompt.Content) > maxTotalContentBytes {
			return nil, false, ErrPromptReferenceCandidateLimit
		}
		totalNameBytes += len(prompt.Name)
		totalContentBytes += len(prompt.Content)
		prompt.Builtin = builtinInt == 1
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return prompts, truncated, nil
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

	query := `
		INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	args := []any{prompt.ID, prompt.Name, prompt.Content, builtinInt, prompt.CreatedAt, prompt.UpdatedAt}
	if !prompt.Builtin {
		query = `
			INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
			SELECT ?, ?, ?, ?, ?, ?
			WHERE (SELECT COUNT(*) FROM custom_prompts) < ?
		`
		args = append(args, maxPromptListItems)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err == nil && !prompt.Builtin {
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if rows == 0 {
			return ErrPromptListLimit
		}
	}
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
		if prompt.ID == builtinCIAutoFixPromptID {
			if err := r.refreshLegacyCIAutoFixPrompt(prompt); err != nil {
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

// refreshLegacyCIAutoFixPrompt updates the shipped CI auto-fix revisions that
// predate the current feedback and actionability guidance. The unchanged
// timestamp guard preserves every user edit, including edits that happen to
// retain the built-in flag.
func (r *sqliteRepository) refreshLegacyCIAutoFixPrompt(current *models.Prompt) error {
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
		return fmt.Errorf("read stored CI auto-fix prompt: %w", err)
	}
	if builtinInt != 1 || !createdAt.Equal(updatedAt) || !isLegacyCIAutoFixPrompt(storedContent) {
		return nil
	}

	_, err = r.db.Exec(r.db.Rebind(`
		UPDATE custom_prompts
		SET content = ?
		WHERE id = ? AND builtin = 1 AND content = ? AND created_at = ? AND updated_at = ?
	`), current.Content, current.ID, storedContent, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("refresh stored CI auto-fix prompt: %w", err)
	}
	return nil
}

func isLegacyCIAutoFixPrompt(content string) bool {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(content))))
	switch hash {
	case ciAutoFixV1SHA256, ciAutoFixV2SHA256, ciAutoFixV3SHA256:
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
		{ID: builtinCIAutoFixPromptID, Name: "ci-auto-fix", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("ci-auto-fix")},
		{ID: "builtin-mr-auto-fix", Name: "mr-auto-fix", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("mr-auto-fix")},
		{ID: builtinChangesWalkthroughPromptID, Name: "changes-walkthrough", Builtin: true, CreatedAt: now, UpdatedAt: now, Content: promptcfg.Get("changes-walkthrough")},
	}
}
