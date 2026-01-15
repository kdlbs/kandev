package worktree

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SQLiteStore implements Store interface using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed worktree store.
// It uses the provided sql.DB connection and ensures the worktrees table exists.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize worktree schema: %w", err)
	}
	return store, nil
}

// initSchema creates the worktrees table if it doesn't exist.
func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS worktrees (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		repository_path TEXT NOT NULL,
		path TEXT NOT NULL,
		branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		merged_at DATETIME,
		deleted_at DATETIME,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_worktrees_task_id ON worktrees(task_id);
	CREATE INDEX IF NOT EXISTS idx_worktrees_repository_id ON worktrees(repository_id);
	CREATE INDEX IF NOT EXISTS idx_worktrees_status ON worktrees(status);
	`

	_, err := s.db.Exec(schema)
	return err
}

// CreateWorktree persists a new worktree record.
func (s *SQLiteStore) CreateWorktree(ctx context.Context, wt *Worktree) error {
	if wt.ID == "" {
		wt.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if wt.CreatedAt.IsZero() {
		wt.CreatedAt = now
	}
	if wt.UpdatedAt.IsZero() {
		wt.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO worktrees (
			id, task_id, repository_id, repository_path, path, branch, base_branch,
			status, created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, wt.ID, wt.TaskID, wt.RepositoryID, wt.RepositoryPath, wt.Path, wt.Branch,
		wt.BaseBranch, wt.Status, wt.CreatedAt, wt.UpdatedAt, wt.MergedAt, wt.DeletedAt)

	return err
}

// GetWorktreeByID retrieves a worktree by its unique ID.
func (s *SQLiteStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	wt := &Worktree{}
	var mergedAt, deletedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, repository_id, repository_path, path, branch, base_branch,
		       status, created_at, updated_at, merged_at, deleted_at
		FROM worktrees WHERE id = ?
	`, id).Scan(
		&wt.ID, &wt.TaskID, &wt.RepositoryID, &wt.RepositoryPath, &wt.Path,
		&wt.Branch, &wt.BaseBranch, &wt.Status, &wt.CreatedAt, &wt.UpdatedAt,
		&mergedAt, &deletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found, return nil without error
	}
	if err != nil {
		return nil, err
	}

	if mergedAt.Valid {
		wt.MergedAt = &mergedAt.Time
	}
	if deletedAt.Valid {
		wt.DeletedAt = &deletedAt.Time
	}

	return wt, nil
}

// GetWorktreeByTaskID retrieves the most recent active worktree by task ID.
// Since multiple worktrees can exist per task, this returns the most recently created active one.
func (s *SQLiteStore) GetWorktreeByTaskID(ctx context.Context, taskID string) (*Worktree, error) {
	wt := &Worktree{}
	var mergedAt, deletedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, repository_id, repository_path, path, branch, base_branch,
		       status, created_at, updated_at, merged_at, deleted_at
		FROM worktrees WHERE task_id = ? AND status = ? ORDER BY created_at DESC LIMIT 1
	`, taskID, StatusActive).Scan(
		&wt.ID, &wt.TaskID, &wt.RepositoryID, &wt.RepositoryPath, &wt.Path,
		&wt.Branch, &wt.BaseBranch, &wt.Status, &wt.CreatedAt, &wt.UpdatedAt,
		&mergedAt, &deletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found, return nil without error
	}
	if err != nil {
		return nil, err
	}

	if mergedAt.Valid {
		wt.MergedAt = &mergedAt.Time
	}
	if deletedAt.Valid {
		wt.DeletedAt = &deletedAt.Time
	}

	return wt, nil
}

// GetWorktreesByTaskID retrieves all worktrees for a task.
func (s *SQLiteStore) GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, repository_id, repository_path, path, branch, base_branch,
		       status, created_at, updated_at, merged_at, deleted_at
		FROM worktrees WHERE task_id = ? ORDER BY created_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// GetWorktreesByRepositoryID retrieves all worktrees for a repository.
func (s *SQLiteStore) GetWorktreesByRepositoryID(ctx context.Context, repoID string) ([]*Worktree, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, repository_id, repository_path, path, branch, base_branch,
		       status, created_at, updated_at, merged_at, deleted_at
		FROM worktrees WHERE repository_id = ?
	`, repoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// UpdateWorktree updates an existing worktree record.
func (s *SQLiteStore) UpdateWorktree(ctx context.Context, wt *Worktree) error {
	wt.UpdatedAt = time.Now().UTC()

	result, err := s.db.ExecContext(ctx, `
		UPDATE worktrees SET
			repository_id = ?, repository_path = ?, path = ?, branch = ?, base_branch = ?,
			status = ?, updated_at = ?, merged_at = ?, deleted_at = ?
		WHERE id = ?
	`, wt.RepositoryID, wt.RepositoryPath, wt.Path, wt.Branch, wt.BaseBranch,
		wt.Status, wt.UpdatedAt, wt.MergedAt, wt.DeletedAt, wt.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("worktree not found: %s", wt.ID)
	}
	return nil
}

// DeleteWorktree removes a worktree record.
func (s *SQLiteStore) DeleteWorktree(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM worktrees WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("worktree not found: %s", id)
	}
	return nil
}

// ListActiveWorktrees returns all worktrees with status 'active'.
func (s *SQLiteStore) ListActiveWorktrees(ctx context.Context) ([]*Worktree, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, repository_id, repository_path, path, branch, base_branch,
		       status, created_at, updated_at, merged_at, deleted_at
		FROM worktrees WHERE status = ?
	`, StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// scanWorktrees is a helper to scan multiple worktree rows.
func (s *SQLiteStore) scanWorktrees(rows *sql.Rows) ([]*Worktree, error) {
	var result []*Worktree
	for rows.Next() {
		wt := &Worktree{}
		var mergedAt, deletedAt sql.NullTime

		err := rows.Scan(
			&wt.ID, &wt.TaskID, &wt.RepositoryID, &wt.RepositoryPath, &wt.Path,
			&wt.Branch, &wt.BaseBranch, &wt.Status, &wt.CreatedAt, &wt.UpdatedAt,
			&mergedAt, &deletedAt,
		)
		if err != nil {
			return nil, err
		}

		if mergedAt.Valid {
			wt.MergedAt = &mergedAt.Time
		}
		if deletedAt.Valid {
			wt.DeletedAt = &deletedAt.Time
		}

		result = append(result, wt)
	}
	return result, rows.Err()
}

// Ensure SQLiteStore implements Store interface.
var _ Store = (*SQLiteStore)(nil)

