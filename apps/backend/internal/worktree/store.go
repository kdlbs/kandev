package worktree

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// SQLiteStore implements Store interface using SQLite.
type SQLiteStore struct {
	db *sqlx.DB
}

// NewSQLiteStore creates a new SQLite-backed worktree store.
// It uses the provided sqlx.DB connection and ensures the task_session_worktrees table exists.
func NewSQLiteStore(db *sqlx.DB) (*SQLiteStore, error) {
	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize worktree schema: %w", err)
	}
	return store, nil
}

// initSchema creates the task_session_worktrees table if it doesn't exist.
func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS task_session_worktrees (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		worktree_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		merged_at TIMESTAMP,
		deleted_at TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		UNIQUE(session_id, worktree_id)
	);

	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_session_id ON task_session_worktrees(session_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_worktree_id ON task_session_worktrees(worktree_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_repository_id ON task_session_worktrees(repository_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_status ON task_session_worktrees(status);
	`

	_, err := s.db.Exec(schema)
	return err
}

// CreateWorktree persists a new worktree record.
func (s *SQLiteStore) CreateWorktree(ctx context.Context, wt *Worktree) error {
	if wt.ID == "" {
		wt.ID = uuid.New().String()
	}
	if wt.SessionID == "" {
		return fmt.Errorf("session ID is required to persist worktree")
	}
	if wt.Status == "" {
		wt.Status = StatusActive
	}
	now := time.Now().UTC()
	if wt.CreatedAt.IsZero() {
		wt.CreatedAt = now
	}
	if wt.UpdatedAt.IsZero() {
		wt.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO task_session_worktrees (
			id, session_id, worktree_id, repository_id, position,
			worktree_path, worktree_branch, status,
			created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, worktree_id) DO UPDATE SET
			repository_id = excluded.repository_id,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			status = excluded.status,
			updated_at = excluded.updated_at,
			merged_at = excluded.merged_at,
			deleted_at = excluded.deleted_at
	`), uuid.New().String(), wt.SessionID, wt.ID, wt.RepositoryID, 0,
		wt.Path, wt.Branch, wt.Status,
		wt.CreatedAt, wt.UpdatedAt, wt.MergedAt, wt.DeletedAt)

	return err
}

// GetWorktreeByID retrieves a worktree by its unique ID.
func (s *SQLiteStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	wt := &Worktree{}
	var mergedAt, deletedAt sql.NullTime
	var repositoryPath, baseBranch sql.NullString

	err := s.db.QueryRowContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		LEFT JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE tsw.worktree_id = ?
	`), id).Scan(
		&wt.ID,
		&wt.SessionID,
		&wt.TaskID,
		&wt.RepositoryID,
		&repositoryPath,
		&wt.Path,
		&wt.Branch,
		&baseBranch,
		&wt.Status,
		&wt.CreatedAt,
		&wt.UpdatedAt,
		&mergedAt,
		&deletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found, return nil without error
	}
	if err != nil {
		return nil, err
	}

	if repositoryPath.Valid {
		wt.RepositoryPath = repositoryPath.String
	}
	if baseBranch.Valid {
		wt.BaseBranch = baseBranch.String
	}
	if mergedAt.Valid {
		wt.MergedAt = &mergedAt.Time
	}
	if deletedAt.Valid {
		wt.DeletedAt = &deletedAt.Time
	}

	return wt, nil
}

func scanWorktreeRow(row *sql.Row) (*Worktree, error) {
	wt := &Worktree{}
	var mergedAt, deletedAt sql.NullTime
	var repositoryPath, baseBranch sql.NullString

	err := row.Scan(
		&wt.ID,
		&wt.SessionID,
		&wt.TaskID,
		&wt.RepositoryID,
		&repositoryPath,
		&wt.Path,
		&wt.Branch,
		&baseBranch,
		&wt.Status,
		&wt.CreatedAt,
		&wt.UpdatedAt,
		&mergedAt,
		&deletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if repositoryPath.Valid {
		wt.RepositoryPath = repositoryPath.String
	}
	if baseBranch.Valid {
		wt.BaseBranch = baseBranch.String
	}
	if mergedAt.Valid {
		wt.MergedAt = &mergedAt.Time
	}
	if deletedAt.Valid {
		wt.DeletedAt = &deletedAt.Time
	}

	return wt, nil
}

// GetWorktreeBySessionID retrieves the worktree by session ID.
func (s *SQLiteStore) GetWorktreeBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	row := s.db.QueryRowContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		INNER JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE tsw.session_id = ? AND tsw.status = ?
	`), sessionID, StatusActive)
	return scanWorktreeRow(row)
}

// GetWorktreeByTaskID retrieves the most recent active worktree by task ID.
// Since multiple worktrees can exist per task, this returns the most recently created active one.
func (s *SQLiteStore) GetWorktreeByTaskID(ctx context.Context, taskID string) (*Worktree, error) {
	row := s.db.QueryRowContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		INNER JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE s.task_id = ? AND tsw.status = ?
		ORDER BY tsw.created_at DESC LIMIT 1
	`), taskID, StatusActive)
	return scanWorktreeRow(row)
}

// GetWorktreesByTaskID retrieves all worktrees for a task.
func (s *SQLiteStore) GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		INNER JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE s.task_id = ? ORDER BY tsw.created_at DESC
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// GetWorktreesByRepositoryID retrieves all worktrees for a repository.
func (s *SQLiteStore) GetWorktreesByRepositoryID(ctx context.Context, repoID string) ([]*Worktree, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		LEFT JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE tsw.repository_id = ?
	`), repoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// UpdateWorktree updates an existing worktree record.
func (s *SQLiteStore) UpdateWorktree(ctx context.Context, wt *Worktree) error {
	wt.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE task_session_worktrees SET
			repository_id = ?, worktree_path = ?, worktree_branch = ?,
			status = ?, updated_at = ?, merged_at = ?, deleted_at = ?
		WHERE worktree_id = ?
	`
	args := []interface{}{
		wt.RepositoryID,
		wt.Path,
		wt.Branch,
		wt.Status,
		wt.UpdatedAt,
		wt.MergedAt,
		wt.DeletedAt,
		wt.ID,
	}
	if wt.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, wt.SessionID)
	}

	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), args...)
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
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM task_session_worktrees WHERE worktree_id = ?`), id)
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
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT
			tsw.worktree_id,
			tsw.session_id,
			s.task_id,
			tsw.repository_id,
			r.local_path,
			tsw.worktree_path,
			tsw.worktree_branch,
			s.base_branch,
			tsw.status,
			tsw.created_at,
			tsw.updated_at,
			tsw.merged_at,
			tsw.deleted_at
		FROM task_session_worktrees tsw
		LEFT JOIN task_sessions s ON tsw.session_id = s.id
		LEFT JOIN repositories r ON tsw.repository_id = r.id
		WHERE tsw.status = ?
	`), StatusActive)
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
		var repositoryPath, baseBranch sql.NullString

		err := rows.Scan(
			&wt.ID,
			&wt.SessionID,
			&wt.TaskID,
			&wt.RepositoryID,
			&repositoryPath,
			&wt.Path,
			&wt.Branch,
			&baseBranch,
			&wt.Status,
			&wt.CreatedAt,
			&wt.UpdatedAt,
			&mergedAt,
			&deletedAt,
		)
		if err != nil {
			return nil, err
		}

		if repositoryPath.Valid {
			wt.RepositoryPath = repositoryPath.String
		}
		if baseBranch.Valid {
			wt.BaseBranch = baseBranch.String
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
