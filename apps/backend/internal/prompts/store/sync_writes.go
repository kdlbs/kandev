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
// manual, so upgrades keep every prompt editable.
func (r *sqliteRepository) addSourceColumns() error {
	columns := []struct{ name, ddl string }{
		{"source", `ALTER TABLE custom_prompts ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`},
		{"source_path", `ALTER TABLE custom_prompts ADD COLUMN source_path TEXT NOT NULL DEFAULT ''`},
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

// countGuardedExec runs a count-limited INSERT. On PostgreSQL the count and
// insert run under one transaction-scoped advisory lock shared by manual and
// synced creation, so concurrent creates cannot both pass the limit; SQLite's
// single writer already serializes the statement.
func (r *sqliteRepository) countGuardedExec(ctx context.Context, query string, args ...any) (sql.Result, error) {
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

// scanPrompt reads one row selected as
// id, name, content, builtin, source, source_path, created_at, updated_at.
func scanPrompt(row promptScanner) (*models.Prompt, error) {
	prompt := &models.Prompt{}
	var builtinInt int
	if err := row.Scan(
		&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt,
		&prompt.Source, &prompt.SourcePath, &prompt.CreatedAt, &prompt.UpdatedAt,
	); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
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
	result, err := r.countGuardedExec(ctx, r.db.Rebind(`
		INSERT INTO custom_prompts (id, name, content, builtin, source, source_path, created_at, updated_at)
		SELECT ?, ?, ?, 0, ?, ?, ?, ?
		WHERE (SELECT COUNT(*) FROM custom_prompts) < ?
	`), prompt.ID, prompt.Name, prompt.Content, prompt.Source, prompt.SourcePath,
		prompt.CreatedAt, prompt.UpdatedAt, maxPromptListItems)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrPromptListLimit
	}
	return prompt, nil
}

// UpdateSyncedPrompt replaces a synced prompt's content and source path. It
// reports false, without writing, when the row is not owned by sync.
func (r *sqliteRepository) UpdateSyncedPrompt(ctx context.Context, id, content, sourcePath string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE custom_prompts SET content = ?, source_path = ?, updated_at = ?
		WHERE id = ? AND source = ?
	`), content, sourcePath, time.Now().UTC(), id, models.PromptSourceSync)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// DeleteSyncedPrompt deletes a synced prompt. It reports false, without
// deleting, when the row is not owned by sync.
func (r *sqliteRepository) DeleteSyncedPrompt(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM custom_prompts WHERE id = ? AND source = ?`), id, models.PromptSourceSync)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// ReleaseSyncedPrompts converts every synced prompt to a manual prompt,
// keeping identity and content, and returns the number released.
func (r *sqliteRepository) ReleaseSyncedPrompts(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE custom_prompts SET source = ?, source_path = ''
		WHERE source = ?
	`), models.PromptSourceManual, models.PromptSourceSync)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
