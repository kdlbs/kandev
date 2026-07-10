// Package repository persists ordinary user terminals in SQLite.
//
// The bottom-panel and script terminals are not persisted here — guards in
// the terminal service decide which terminals get a row. This package is
// scope-agnostic; it just stores what it's told.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/terminal/models"
)

// ErrNotFound is returned by Get when no terminal matches the id.
var ErrNotFound = errors.New("terminal not found")

// Repository persists user terminals.
type Repository struct {
	db  *sqlx.DB // writer
	ro  *sqlx.DB // reader
	log *logger.Logger
}

// NewWithDB constructs a Repository sharing the supplied writer/reader pools.
// initSchema creates the table on first call; subsequent calls are no-ops.
func NewWithDB(writer, reader *sqlx.DB, log *logger.Logger) (*Repository, error) {
	r := &Repository{db: writer, ro: reader, log: log}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("init terminal schema: %w", err)
	}
	return r, nil
}

func (r *Repository) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS user_terminals (
			id              TEXT PRIMARY KEY,
			task_id         TEXT NOT NULL,
			environment_id  TEXT NOT NULL,
			seq             INTEGER NOT NULL,
			custom_name     TEXT,
			state           TEXT NOT NULL DEFAULT 'open',
			initial_command TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id, seq)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_terminals_task ON user_terminals(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_terminals_env ON user_terminals(environment_id)`,
	}
	for _, s := range stmts {
		if _, err := r.db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// Create inserts a new terminal row. The seq is computed as MAX(seq)+1 for
// the task — never reused, so deletes leave gaps.
//
// Seq computation and insertion are folded into a single INSERT … SELECT
// statement so the MAX read and the new row write happen atomically inside
// SQLite's per-statement transaction. Two concurrent creates for the same
// task can no longer pick the same seq — the second statement reads the
// first's already-inserted row.
//
// The caller supplies the id (must be a UUID that's also used as the
// agentctl PTY id). initialCommand is "" for plain shells.
func (r *Repository) Create(ctx context.Context, taskID, envID, id, initialCommand string) (*models.Terminal, error) {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_terminals (id, task_id, environment_id, seq, custom_name, state, initial_command)
		 SELECT ?, ?, ?, COALESCE(MAX(seq), 0) + 1, NULL, 'open', ?
		 FROM user_terminals WHERE task_id = ?`,
		id, taskID, envID, initialCommand, taskID,
	); err != nil {
		return nil, fmt.Errorf("insert terminal: %w", err)
	}

	return r.Get(ctx, id)
}

// Get returns the terminal by id, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, id string) (*models.Terminal, error) {
	var t models.Terminal
	err := r.ro.GetContext(ctx, &t,
		`SELECT id, task_id, environment_id, seq, custom_name, state, initial_command, created_at
		 FROM user_terminals WHERE id = ?`,
		id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get terminal: %w", err)
	}
	return &t, nil
}

// ListByTask returns terminals for taskID ordered by seq ascending.
// includeParked controls whether parked rows are included.
func (r *Repository) ListByTask(ctx context.Context, taskID string, includeParked bool) ([]*models.Terminal, error) {
	q := `SELECT id, task_id, environment_id, seq, custom_name, state, initial_command, created_at
	      FROM user_terminals WHERE task_id = ?`
	args := []any{taskID}
	if !includeParked {
		q += ` AND state = 'open'`
	}
	q += ` ORDER BY seq ASC`

	var rows []*models.Terminal
	if err := r.ro.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("list terminals: %w", err)
	}
	return rows, nil
}

// Rename sets or clears the custom_name. Pass nil to clear, a non-empty
// string to set.
func (r *Repository) Rename(ctx context.Context, id string, name *string) error {
	var nameArg any
	if name == nil {
		nameArg = nil
	} else {
		nameArg = *name
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_terminals SET custom_name = ? WHERE id = ?`,
		nameArg, id,
	)
	if err != nil {
		return fmt.Errorf("rename terminal: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetState updates the open/parked state. Does not touch any PTY — that's
// the service's job.
func (r *Repository) SetState(ctx context.Context, id string, state models.TerminalState) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_terminals SET state = ? WHERE id = ?`,
		string(state), id,
	)
	if err != nil {
		return fmt.Errorf("set state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a single terminal row.
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_terminals WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete terminal: %w", err)
	}
	return nil
}

// DeleteByTask removes every row for a task and returns the count. Used by
// the task.deleted cascade subscriber.
func (r *Repository) DeleteByTask(ctx context.Context, taskID string) (int, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM user_terminals WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, fmt.Errorf("delete by task: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
