package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/prompts/models"
)

// addSourceColumns adds the sync ownership columns. Existing rows default to
// manual with no kept copy and no conflict, so upgrades keep every prompt
// editable.
func (r *sqliteRepository) addSourceColumns() error {
	columns := []struct{ name, ddl string }{
		{"source", `ALTER TABLE custom_prompts ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`},
		{"source_path", `ALTER TABLE custom_prompts ADD COLUMN source_path TEXT NOT NULL DEFAULT ''`},
		{"alternate_content", `ALTER TABLE custom_prompts ADD COLUMN alternate_content TEXT NOT NULL DEFAULT ''`},
		{"conflict", `ALTER TABLE custom_prompts ADD COLUMN conflict INTEGER NOT NULL DEFAULT 0`},
	}
	for _, column := range columns {
		exists, err := db.ColumnExists(r.db, "custom_prompts", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := r.db.Exec(column.ddl); err != nil {
			return err
		}
	}
	return nil
}

// promptCountLockKey names the advisory lock shared by every count-limited
// prompt insert.
const promptCountLockKey = "custom_prompts:count-limit"

// insertCountLimited runs a count-limited INSERT and returns
// ErrPromptListLimit when the limit kept it from inserting a row.
func (r *sqliteRepository) insertCountLimited(ctx context.Context, query string, args ...any) error {
	result, err := r.execCountLimited(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrPromptListLimit
	}
	return nil
}

// execCountLimited runs a count-limited INSERT. On PostgreSQL the count and
// insert run under one transaction-scoped advisory lock shared by manual and
// synced creation, so concurrent creates cannot both pass the limit; SQLite's
// single writer already serializes the statement.
func (r *sqliteRepository) execCountLimited(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return r.db.ExecContext(ctx, query, args...)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, promptCountLockKey); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

type promptScanner interface {
	Scan(dest ...any) error
}

// promptColumns is the column list scanPrompt reads.
const promptColumns = "id, name, content, builtin, source, source_path, alternate_content, conflict, created_at, updated_at"

// scanPrompt reads one row selected with promptColumns.
func scanPrompt(row promptScanner) (*models.Prompt, error) {
	prompt := &models.Prompt{}
	var builtinInt, conflictInt int
	if err := row.Scan(
		&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt,
		&prompt.Source, &prompt.SourcePath, &prompt.AlternateContent, &conflictInt,
		&prompt.CreatedAt, &prompt.UpdatedAt,
	); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
	prompt.Conflict = conflictInt == 1
	return prompt, nil
}

// CreateSyncedPrompt inserts a prompt owned by Prompt Sync, subject to the
// same prompt-count limit as manual creation.
func (r *sqliteRepository) CreateSyncedPrompt(ctx context.Context, name, content, sourcePath string) (*models.Prompt, error) {
	now := time.Now().UTC()
	prompt := &models.Prompt{
		ID: uuid.New().String(), Name: name, Content: content,
		Source: models.PromptSourceSync, SourcePath: sourcePath, CreatedAt: now, UpdatedAt: now,
	}
	err := r.insertCountLimited(ctx, r.db.Rebind(`
		INSERT INTO custom_prompts (id, name, content, builtin, source, source_path, created_at, updated_at)
		SELECT ?, ?, ?, 0, ?, ?, ?, ?
		WHERE (SELECT COUNT(*) FROM custom_prompts) < ?
	`), prompt.ID, prompt.Name, prompt.Content, prompt.Source, prompt.SourcePath,
		prompt.CreatedAt, prompt.UpdatedAt, maxPromptListItems)
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

// UpdateSyncedPrompt replaces a synced prompt's content and source path. It
// reports false, without writing, when the row is not owned by sync.
func (r *sqliteRepository) UpdateSyncedPrompt(ctx context.Context, id, content, sourcePath string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET content = ?, source_path = ?, updated_at = ?
		WHERE id = ? AND source = ?
	`, content, sourcePath, time.Now().UTC(), id, models.PromptSourceSync)
}

// DeleteSyncedPrompt deletes a synced prompt. It reports false, without
// deleting, when the row is not owned by sync or is a built-in prompt.
func (r *sqliteRepository) DeleteSyncedPrompt(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx,
		`DELETE FROM custom_prompts WHERE id = ? AND source = ? AND builtin = ?`, id, models.PromptSourceSync, 0)
}

// ReleaseOptions are the user's choices when Prompt Sync is removed.
type ReleaseOptions struct {
	// UseOther names the prompts that take their other version (kept local
	// version or repository version) as their content.
	UseOther []string
	// DeleteRepositoryOnly names the synced prompts the user reviewed and chose
	// to delete. Only those that still came only from the repository (not
	// built-in, no kept local version) are deleted; any other is released.
	DeleteRepositoryOnly []string
}

// ReleaseSyncedPrompts converts every synced prompt and override to a manual
// prompt, keeping identity and current content, drops other versions and
// conflicts, and returns the number of prompts released or deleted. All
// steps run in one transaction.
func (r *sqliteRepository) ReleaseSyncedPrompts(ctx context.Context, opts ReleaseOptions) (int64, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, name := range opts.UseOther {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE custom_prompts SET content = alternate_content
			WHERE name = ? AND source IN (?, ?) AND alternate_content <> ''
		`), name, models.PromptSourceSync, models.PromptSourceOverride); err != nil {
			return 0, err
		}
	}
	var deleted int64
	for _, name := range opts.DeleteRepositoryOnly {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			DELETE FROM custom_prompts WHERE name = ? AND source = ? AND alternate_content = '' AND builtin = ?
		`), name, models.PromptSourceSync, 0)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += count
	}
	// Advancing updated_at rejects any save based on a pre-release revision.
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE custom_prompts SET source = ?, source_path = '', alternate_content = '', conflict = 0, updated_at = ?
		WHERE source IN (?, ?)
	`), models.PromptSourceManual, time.Now().UTC(), models.PromptSourceSync, models.PromptSourceOverride)
	if err != nil {
		return 0, err
	}
	released, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return deleted + released, tx.Commit()
}
