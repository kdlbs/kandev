package worktree

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// SQLiteStore implements Store interface using SQLite.
//
// Physical worktrees are owned by task environments: every record is a row in
// task_environment_repos, and sessions reach them only through
// task_sessions.task_environment_id. The store resolves session-keyed lookups
// through that link; there is no session-to-worktree table.
type SQLiteStore struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewSQLiteStore creates a new SQLite-backed worktree store.
// It uses the provided writer and reader connections. The
// task_environment_repos schema is owned by the task repository's
// initializer; the store adds nothing on its own.
func NewSQLiteStore(writer, reader *sqlx.DB) (*SQLiteStore, error) {
	return &SQLiteStore{db: writer, ro: reader}, nil
}

// worktreeSelectCols is the SELECT projection shared by every worktree query.
// The session column is the ID of the session that resolved the environment
// (callers keyed on a session see their own session), and base_branch comes
// from that session's launch snapshot.
const worktreeSelectCols = `
	ter.worktree_id,
	COALESCE(s.id, '') AS session_id,
	te.task_id,
	ter.repository_id,
	r.local_path,
	ter.worktree_path,
	ter.worktree_branch,
	COALESCE(ter.branch_slug, ''),
	COALESCE(s.base_branch, ''),
	ter.status,
	ter.created_at,
	ter.updated_at,
	ter.merged_at,
	ter.deleted_at,
	ter.task_environment_id`

// rowScanner abstracts *sql.Row and *sql.Rows so worktree rows scan through
// one helper.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanWorktreeRow scans one environment-repository row into a Worktree value.
func scanWorktreeRow(row rowScanner) (*Worktree, error) {
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
		&wt.BranchSlug,
		&baseBranch,
		&wt.Status,
		&wt.CreatedAt,
		&wt.UpdatedAt,
		&mergedAt,
		&deletedAt,
		&wt.TaskEnvironmentID,
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

// resolveEnvironmentID maps a session to its task environment. Returns ""
// when the session has no environment yet (initial materialization happens
// before the environment row exists).
func (s *SQLiteStore) resolveEnvironmentID(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	var envID string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT COALESCE(task_environment_id, '')
		FROM task_sessions WHERE id = ?`), sessionID).Scan(&envID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return envID, nil
}

// CreateWorktree persists a worktree record under the session's task
// environment. The environment-repository row is the only physical-worktree
// record; multiple sessions sharing the environment observe the same rows.
//
// The insert serializes against the task cleanup barrier: PostgreSQL locks
// the owning task row and SQLite serializes the writer transaction, so a
// worktree admitted after archive/delete inventory capture is rejected with
// ErrTaskCleanupInProgress and the caller compensates the physical directory.
func (s *SQLiteStore) CreateWorktree(ctx context.Context, wt *Worktree) error {
	if wt.ID == "" {
		wt.ID = uuid.New().String()
	}
	if wt.SessionID == "" {
		return fmt.Errorf("session ID is required to persist worktree")
	}
	envID, err := s.resolveEnvironmentID(ctx, wt.SessionID)
	if err != nil {
		return fmt.Errorf("resolve environment for worktree %s: %w", wt.ID, err)
	}
	if envID == "" {
		return fmt.Errorf("%w: session %s", ErrEnvironmentNotResolved, wt.SessionID)
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
	wt.TaskEnvironmentID = envID

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var taskID string
	if err := tx.QueryRowContext(ctx, s.db.Rebind(`
		SELECT task_id FROM task_environments WHERE id = ?`), envID).Scan(&taskID); err != nil {
		return fmt.Errorf("resolve worktree owner %s: %w", envID, err)
	}
	if err := s.checkTaskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, status, created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_environment_id, repository_id, branch_slug) DO UPDATE SET
			worktree_id = excluded.worktree_id,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			status = excluded.status,
			updated_at = excluded.updated_at,
			merged_at = excluded.merged_at,
			deleted_at = excluded.deleted_at
	`), uuid.New().String(), envID, wt.RepositoryID, wt.BranchSlug,
		wt.ID, wt.Path, wt.Branch, 0,
		"", wt.Status,
		wt.CreatedAt, wt.UpdatedAt, wt.MergedAt, wt.DeletedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// checkTaskCleanupBarrierLocked rejects the persistence when a task lifecycle
// cleanup barrier is active for the owning task. PostgreSQL serializes through
// a row lock on the task; SQLite's single-writer transaction is the lock.
func (s *SQLiteStore) checkTaskCleanupBarrierLocked(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if dialect.IsPostgres(s.db.DriverName()) {
		var lockedTaskID string
		if err := tx.QueryRowContext(ctx, s.db.Rebind(
			`SELECT id FROM tasks WHERE id = ? FOR UPDATE`,
		), taskID).Scan(&lockedTaskID); err != nil {
			return fmt.Errorf("lock task for creation barrier: %w", err)
		}
	}
	var active bool
	if err := tx.QueryRowContext(ctx, s.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = ? AND state IN (?, ?, ?, ?)
		)
	`), taskID,
		models.TaskResourceCleanupStatePrepared,
		models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRunning,
		models.TaskResourceCleanupStateRetryWait,
	).Scan(&active); err != nil {
		return fmt.Errorf("check task cleanup barrier: %w", err)
	}
	if active {
		return fmt.Errorf("%w: %s", ErrTaskCleanupInProgress, taskID)
	}
	return nil
}

// GetWorktreeByID retrieves a worktree by its unique ID.
func (s *SQLiteStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		LEFT JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE ter.worktree_id = ?
		LIMIT 1
	`), id)
	return scanWorktreeRow(row)
}

// GetWorktreeBySessionID retrieves the worktree by session ID.
func (s *SQLiteStore) GetWorktreeBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		INNER JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE s.id = ? AND ter.status = ?
		  AND ter.deleted_at IS NULL
		ORDER BY ter.position ASC, ter.created_at ASC
		LIMIT 1
	`), sessionID, StatusActive)
	return scanWorktreeRow(row)
}

// GetWorktreeByTaskID retrieves the most recent active worktree by task ID.
// Since multiple worktrees can exist per task, this returns the most recently created active one.
func (s *SQLiteStore) GetWorktreeByTaskID(ctx context.Context, taskID string) (*Worktree, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		LEFT JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE te.task_id = ? AND ter.status = ?
		  AND ter.deleted_at IS NULL
		ORDER BY ter.created_at DESC LIMIT 1
	`), taskID, StatusActive)
	return scanWorktreeRow(row)
}

// GetWorktreesByTaskID retrieves all worktrees for a task.
func (s *SQLiteStore) GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		LEFT JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE te.task_id = ?
		  AND ter.deleted_at IS NULL
		  AND ter.status <> ?
		ORDER BY ter.created_at DESC
	`), taskID, StatusDeleted)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// GetWorktreesByRepositoryID retrieves all worktrees for a repository.
func (s *SQLiteStore) GetWorktreesByRepositoryID(ctx context.Context, repoID string) ([]*Worktree, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		LEFT JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE ter.repository_id = ?
		  AND ter.deleted_at IS NULL
		  AND ter.status <> ?
	`), repoID, StatusDeleted)
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
		UPDATE task_environment_repos SET
			worktree_path = ?, worktree_branch = ?,
			status = ?, updated_at = ?, merged_at = ?, deleted_at = ?
		WHERE worktree_id = ?
	`
	args := []interface{}{
		wt.Path,
		wt.Branch,
		wt.Status,
		wt.UpdatedAt,
		wt.MergedAt,
		wt.DeletedAt,
		wt.ID,
	}
	if wt.TaskEnvironmentID != "" {
		query += " AND task_environment_id = ?"
		args = append(args, wt.TaskEnvironmentID)
	}

	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrWorktreeNotFound, wt.ID)
	}
	return nil
}

// DeleteWorktree removes a worktree record.
func (s *SQLiteStore) DeleteWorktree(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM task_environment_repos WHERE worktree_id = ?`), id)
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
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		LEFT JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE ter.status = ? AND ter.deleted_at IS NULL
	`), StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// ListActiveWorktreePaths returns the worktree_path of every active,
// non-deleted environment-repository row that has a non-empty path. The
// office GC uses this set as the authoritative inventory of live worktrees;
// any directory under the worktree base that does not appear here (and is
// older than the GC grace period) is considered orphaned.
func (s *SQLiteStore) ListActiveWorktreePaths(ctx context.Context) ([]string, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT worktree_path
		FROM task_environment_repos
		WHERE status = ?
		  AND deleted_at IS NULL
		  AND worktree_path <> ''
	`), StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// CountActiveWorktreeReferences counts non-deleted session associations from
// OTHER tasks for the environment that owns the worktree. The worktree is
// task-owned: sessions of the owning task (which share the environment) never
// count against cleanup, while a borrower task's session protects the
// workspace until ownership transfers. A terminal session still protects its
// task workspace until that task's own cleanup releases the association.
func (s *SQLiteStore) CountActiveWorktreeReferences(
	ctx context.Context,
	worktreeID string,
	excludeSessionIDs []string,
) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM task_sessions s
		INNER JOIN task_environment_repos ter
			ON ter.task_environment_id = s.task_environment_id
		INNER JOIN task_environments te
			ON te.id = ter.task_environment_id
		WHERE ter.worktree_id = ?
		  AND ter.status <> ?
		  AND ter.deleted_at IS NULL
		  AND te.task_id != s.task_id
	`
	args := []interface{}{
		worktreeID,
		StatusDeleted,
	}
	if len(excludeSessionIDs) > 0 {
		query += ` AND s.id NOT IN (?)`
		args = append(args, excludeSessionIDs)
	}
	query, args, err := sqlx.In(query, args...)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.ro.QueryRowContext(ctx, s.ro.Rebind(query), args...).Scan(&count)
	return count, err
}

// scanWorktrees is a helper to scan multiple worktree rows. Task-keyed and
// repository-keyed queries LEFT JOIN sessions (a worktree belongs to the task
// environment, and a task may have zero or many sessions), so rows are
// deduplicated by worktree identity.
func (s *SQLiteStore) scanWorktrees(rows *sql.Rows) ([]*Worktree, error) {
	var result []*Worktree
	seen := make(map[string]bool)
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
			&wt.BranchSlug,
			&baseBranch,
			&wt.Status,
			&wt.CreatedAt,
			&wt.UpdatedAt,
			&mergedAt,
			&deletedAt,
			&wt.TaskEnvironmentID,
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

		if seen[wt.ID] {
			continue
		}
		seen[wt.ID] = true
		result = append(result, wt)
	}
	return result, rows.Err()
}

// GetWorktreesBySessionID returns all active worktrees for the session.
// Implements MultiRepoStore.
func (s *SQLiteStore) GetWorktreesBySessionID(ctx context.Context, sessionID string) ([]*Worktree, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		INNER JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE s.id = ? AND ter.status = ?
		  AND ter.deleted_at IS NULL
		ORDER BY ter.position ASC, ter.created_at ASC
	`), sessionID, StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return s.scanWorktrees(rows)
}

// GetWorktreeBySessionAndRepository returns the active worktree for the
// given (session, repository, branchSlug) triple, or nil if none exists.
// branchSlug scopes the lookup so multi-branch tasks (same repo, multiple
// branches) don't collapse — an empty slug matches the legacy
// single-branch persistence shape, so single-branch callers remain
// unchanged. Implements MultiRepoStore.
func (s *SQLiteStore) GetWorktreeBySessionAndRepository(ctx context.Context, sessionID, repositoryID, branchSlug string) (*Worktree, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+worktreeSelectCols+`
		FROM task_environment_repos ter
		INNER JOIN task_environments te ON ter.task_environment_id = te.id
		INNER JOIN task_sessions s ON s.task_environment_id = ter.task_environment_id
		LEFT JOIN repositories r ON ter.repository_id = r.id
		WHERE s.id = ? AND ter.repository_id = ?
		  AND COALESCE(ter.branch_slug, '') = ?
		  AND ter.status = ?
		  AND ter.deleted_at IS NULL
		LIMIT 1
	`), sessionID, repositoryID, branchSlug, StatusActive)
	return scanWorktreeRow(row)
}

// Ensure SQLiteStore implements both Store and MultiRepoStore.
var (
	_ Store          = (*SQLiteStore)(nil)
	_ MultiRepoStore = (*SQLiteStore)(nil)
)
