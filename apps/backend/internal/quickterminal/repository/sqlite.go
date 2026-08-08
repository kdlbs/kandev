// Package repository persists server-owned Quick Terminal descriptors.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/quickterminal/models"
)

var (
	ErrNotFound      = errors.New("quick terminal tab not found")
	ErrTabIDConflict = errors.New("quick terminal tab id belongs to another user")
)

type Repository struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func NewWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	r := &Repository{db: writer, ro: reader}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("init quick terminal schema: %w", err)
	}
	return r, nil
}

func (r *Repository) initSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS quick_terminal_tabs (
			tab_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			session_id TEXT,
			sequence INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'connecting',
			exit_code INTEGER,
			error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, workspace_id, sequence)
		)`,
		`CREATE TABLE IF NOT EXISTS quick_terminal_workspace_sequences (
			user_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			next_sequence INTEGER NOT NULL,
			PRIMARY KEY(user_id, workspace_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_quick_terminal_tabs_workspace
			ON quick_terminal_tabs(user_id, workspace_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_quick_terminal_tabs_session
			ON quick_terminal_tabs(session_id)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(r.db.Rebind(statement)); err != nil {
			return err
		}
	}
	return nil
}

const tabColumns = `tab_id, user_id, workspace_id, session_id, sequence, status,
	exit_code, error, created_at, updated_at`

func (r *Repository) Create(ctx context.Context, userID, workspaceID, tabID string) (*models.Tab, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin quick terminal create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing models.Tab
	err = tx.GetContext(ctx, &existing, r.db.Rebind(
		`SELECT `+tabColumns+` FROM quick_terminal_tabs WHERE tab_id = ? AND user_id = ?`,
	), tabID, userID)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit existing quick terminal: %w", err)
		}
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("find quick terminal: %w", err)
	}

	var sequence int
	err = tx.GetContext(ctx, &sequence, r.db.Rebind(`
		INSERT INTO quick_terminal_workspace_sequences (user_id, workspace_id, next_sequence)
		VALUES (?, ?, 2)
		ON CONFLICT(user_id, workspace_id) DO UPDATE
		SET next_sequence = quick_terminal_workspace_sequences.next_sequence + 1
		RETURNING next_sequence - 1
	`), userID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("allocate quick terminal sequence: %w", err)
	}

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO quick_terminal_tabs
			(tab_id, user_id, workspace_id, sequence, status, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`), tabID, userID, workspaceID, sequence, models.StatusConnecting, "")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrTabIDConflict
		}
		return nil, fmt.Errorf("insert quick terminal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit quick terminal: %w", err)
	}
	return r.Get(ctx, userID, tabID)
}

func (r *Repository) Get(ctx context.Context, userID, tabID string) (*models.Tab, error) {
	var tab models.Tab
	err := r.ro.GetContext(ctx, &tab, r.ro.Rebind(
		`SELECT `+tabColumns+` FROM quick_terminal_tabs WHERE user_id = ? AND tab_id = ?`,
	), userID, tabID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get quick terminal: %w", err)
	}
	return &tab, nil
}

func (r *Repository) GetByTabID(ctx context.Context, tabID string) (*models.Tab, error) {
	var tab models.Tab
	err := r.ro.GetContext(ctx, &tab, r.ro.Rebind(
		`SELECT `+tabColumns+` FROM quick_terminal_tabs WHERE tab_id = ?`,
	), tabID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get quick terminal by id: %w", err)
	}
	return &tab, nil
}

func (r *Repository) List(ctx context.Context, userID, workspaceID string) ([]*models.Tab, error) {
	var tabs []*models.Tab
	if err := r.ro.SelectContext(ctx, &tabs, r.ro.Rebind(
		`SELECT `+tabColumns+` FROM quick_terminal_tabs
		 WHERE user_id = ? AND workspace_id = ? ORDER BY sequence ASC`,
	), userID, workspaceID); err != nil {
		return nil, fmt.Errorf("list quick terminals: %w", err)
	}
	return tabs, nil
}

func (r *Repository) UpdateLifecycle(
	ctx context.Context,
	userID, tabID, sessionID, status string,
	exitCode *int,
	errorText string,
) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE quick_terminal_tabs
		SET session_id = ?, status = ?, exit_code = ?, error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND tab_id = ?
	`), nullableString(sessionID), status, exitCode, errorText, userID, tabID)
	if err != nil {
		return fmt.Errorf("update quick terminal lifecycle: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateLifecycleByTabID(
	ctx context.Context,
	tabID, sessionID, status string,
	exitCode *int,
	errorText string,
) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE quick_terminal_tabs
		SET session_id = ?, status = ?, exit_code = ?, error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE tab_id = ? AND session_id = ?
	`), nullableString(sessionID), status, exitCode, errorText, tabID, sessionID)
	if err != nil {
		return fmt.Errorf("update quick terminal exit: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, userID, tabID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM quick_terminal_tabs WHERE user_id = ? AND tab_id = ?`,
	), userID, tabID)
	if err != nil {
		return fmt.Errorf("delete quick terminal: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
