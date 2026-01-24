// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"fmt"
)

// Repository provides SQLite-based task storage operations.
type Repository struct {
	db     *sql.DB
	ownsDB bool
}

// NewWithDB creates a new SQLite repository with an existing database connection (shared ownership).
func NewWithDB(dbConn *sql.DB) (*Repository, error) {
	return newRepository(dbConn, false)
}



func newRepository(dbConn *sql.DB, ownsDB bool) (*Repository, error) {
	repo := &Repository{db: dbConn, ownsDB: ownsDB}
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

// Close closes the database connection
func (r *Repository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *Repository) DB() *sql.DB {
	return r.db
}

// ensureColumn adds a column to a table if it doesn't exist
func (r *Repository) ensureColumn(table, column, definition string) error {
	exists, err := r.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

// columnExists checks if a column exists in a table
func (r *Repository) columnExists(table, column string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// boolToInt converts a boolean to an integer (for SQLite)
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// ensureWorkspaceIndexes creates workspace-related indexes
func (r *Repository) ensureWorkspaceIndexes() error {
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_boards_workspace_id ON boards(workspace_id)`); err != nil {
		return err
	}
	return nil
}



// initSchema creates the database tables if they don't exist
func (r *Repository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		owner_id TEXT DEFAULT '',
		default_executor_id TEXT DEFAULT '',
		default_environment_id TEXT DEFAULT '',
		default_agent_profile_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS executors (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		is_system INTEGER NOT NULL DEFAULT 0,
		resumable INTEGER NOT NULL DEFAULT 1,
		config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS executors_running (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL UNIQUE,
		task_id TEXT NOT NULL,
		executor_id TEXT NOT NULL,
		runtime TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'starting',
		resumable INTEGER NOT NULL DEFAULT 0,
		resume_token TEXT DEFAULT '',
		agent_execution_id TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		agentctl_url TEXT DEFAULT '',
		agentctl_port INTEGER DEFAULT 0,
		pid INTEGER DEFAULT 0,
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		last_seen_at DATETIME,
		error_message TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS environments (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		is_system INTEGER NOT NULL DEFAULT 0,
		worktree_root TEXT DEFAULT '',
		image_tag TEXT DEFAULT '',
		dockerfile TEXT DEFAULT '',
		build_config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS boards (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS columns (
		id TEXT PRIMARY KEY,
		board_id TEXT NOT NULL,
		name TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		state TEXT DEFAULT 'TODO',
		color TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		board_id TEXT NOT NULL,
		column_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		state TEXT DEFAULT 'TODO',
		priority INTEGER DEFAULT 0,
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
		FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS task_repositories (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		base_branch TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
		UNIQUE(task_id, repository_id)
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'local',
		local_path TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		provider_repo_id TEXT DEFAULT '',
		provider_owner TEXT DEFAULT '',
		provider_name TEXT DEFAULT '',
		default_branch TEXT DEFAULT '',
		worktree_branch_prefix TEXT DEFAULT 'feature/',
		setup_script TEXT DEFAULT '',
		cleanup_script TEXT DEFAULT '',
		dev_script TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repository_scripts (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_board_id ON tasks(board_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_column_id ON tasks(column_id);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_task_id ON task_repositories(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_repository_id ON task_repositories(repository_id);
	CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_workspace_id ON repositories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_repository_scripts_repo_id ON repository_scripts(repository_id);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	// Session and message tables
	sessionSchema := `
	CREATE TABLE IF NOT EXISTS task_session_messages (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		task_id TEXT DEFAULT '',
		turn_id TEXT NOT NULL,
		author_type TEXT NOT NULL DEFAULT 'user',
		author_id TEXT DEFAULT '',
		content TEXT NOT NULL,
		requests_input INTEGER DEFAULT 0,
		type TEXT NOT NULL DEFAULT 'message',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON task_session_messages(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON task_session_messages(created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_session_messages(task_session_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_turn_id ON task_session_messages(turn_id);

	CREATE TABLE IF NOT EXISTS task_session_turns (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_turns_session_id ON task_session_turns(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_turns_session_started ON task_session_turns(task_session_id, started_at);
	CREATE INDEX IF NOT EXISTS idx_turns_task_id ON task_session_turns(task_id);

	CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_execution_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL,
		executor_id TEXT DEFAULT '',
		environment_id TEXT DEFAULT '',
		repository_id TEXT DEFAULT '',
		base_branch TEXT DEFAULT '',
		agent_profile_snapshot TEXT DEFAULT '{}',
		executor_snapshot TEXT DEFAULT '{}',
		environment_snapshot TEXT DEFAULT '{}',
		repository_snapshot TEXT DEFAULT '{}',
		state TEXT NOT NULL DEFAULT 'CREATED',
		error_message TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state);

	CREATE TABLE IF NOT EXISTS task_session_worktrees (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		worktree_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		merged_at DATETIME,
		deleted_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		UNIQUE(session_id, worktree_id)
	);

	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_session_id ON task_session_worktrees(session_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_worktree_id ON task_session_worktrees(worktree_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_repository_id ON task_session_worktrees(repository_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_status ON task_session_worktrees(status);
	`

	if _, err := r.db.Exec(sessionSchema); err != nil {
		return err
	}

	// Git tracking tables
	gitSchema := `
	CREATE TABLE IF NOT EXISTS task_session_git_snapshots (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		snapshot_type TEXT NOT NULL,
		branch TEXT NOT NULL,
		remote_branch TEXT DEFAULT '',
		head_commit TEXT DEFAULT '',
		base_commit TEXT DEFAULT '',
		ahead INTEGER DEFAULT 0,
		behind INTEGER DEFAULT 0,
		files TEXT DEFAULT '{}',
		triggered_by TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_git_snapshots_session ON task_session_git_snapshots(session_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_git_snapshots_type ON task_session_git_snapshots(session_id, snapshot_type);

	CREATE TABLE IF NOT EXISTS task_session_commits (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		commit_sha TEXT NOT NULL,
		parent_sha TEXT DEFAULT '',
		author_name TEXT DEFAULT '',
		author_email TEXT DEFAULT '',
		commit_message TEXT DEFAULT '',
		committed_at DATETIME NOT NULL,
		pre_commit_snapshot_id TEXT DEFAULT '',
		post_commit_snapshot_id TEXT DEFAULT '',
		files_changed INTEGER DEFAULT 0,
		insertions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_session_commits_session ON task_session_commits(session_id, committed_at DESC);
	CREATE INDEX IF NOT EXISTS idx_session_commits_sha ON task_session_commits(commit_sha);
	`

	if _, err := r.db.Exec(gitSchema); err != nil {
		return err
	}

	// Ensure columns exist for schema migrations
	if err := r.ensureColumn("boards", "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("tasks", "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_executor_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_environment_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_agent_profile_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("columns", "color", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("environments", "is_system", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureColumn("repositories", "deleted_at", "DATETIME"); err != nil {
		return err
	}
	if err := r.ensureColumn("repositories", "worktree_branch_prefix", "TEXT DEFAULT 'feature/'"); err != nil {
		return err
	}
	if err := r.ensureColumn("repositories", "dev_script", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("executors", "deleted_at", "DATETIME"); err != nil {
		return err
	}
	if err := r.ensureColumn("executors", "resumable", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := r.ensureColumn("environments", "deleted_at", "DATETIME"); err != nil {
		return err
	}

	// Ensure new message columns exist for existing databases
	if err := r.ensureColumn("task_session_messages", "type", "TEXT NOT NULL DEFAULT 'message'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_session_messages", "metadata", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_session_messages", "task_session_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Ensure container_id column exists for existing databases
	if err := r.ensureColumn("task_sessions", "container_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "executor_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "environment_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "state", "TEXT NOT NULL DEFAULT 'CREATED'"); err != nil {
		return err
	}

	if err := r.ensureDefaultWorkspace(); err != nil {
		return err
	}

	if err := r.ensureDefaultExecutorsAndEnvironments(); err != nil {
		return err
	}

	if err := r.ensureWorkspaceIndexes(); err != nil {
		return err
	}

	return nil
}
