// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Repository provides SQLite-based task storage operations.
type Repository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader (read-only pool)
	ownsDB bool
}

// NewWithDB creates a new SQLite repository with an existing database connection (shared ownership).
func NewWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	return newRepository(writer, reader, false)
}

func newRepository(writer, reader *sqlx.DB, ownsDB bool) (*Repository, error) {
	repo := &Repository{db: writer, ro: reader, ownsDB: ownsDB}
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

// Close closes the database connection
func (r *Repository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *Repository) DB() *sql.DB {
	return r.db.DB
}

// ensureWorkspaceIndexes creates workspace-related indexes
func (r *Repository) ensureWorkspaceIndexes() error {
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_workflows_workspace_id ON workflows(workspace_id)`); err != nil {
		return err
	}
	return nil
}

// initSchema creates the database tables if they don't exist
func (r *Repository) initSchema() error {
	if err := r.initCoreSchema(); err != nil {
		return err
	}
	if err := r.initPlansSchema(); err != nil {
		return err
	}
	if err := r.initSessionSchema(); err != nil {
		return err
	}
	if err := r.initGitSchema(); err != nil {
		return err
	}
	if err := r.initReviewSchema(); err != nil {
		return err
	}
	if err := r.migrateExecutorProfiles(); err != nil {
		return err
	}
	if err := r.migrateTaskSessions(); err != nil {
		return err
	}
	if err := r.ensureDefaultWorkspace(); err != nil {
		return err
	}
	if err := r.ensureDefaultExecutorsAndEnvironments(); err != nil {
		return err
	}
	if err := r.runMigrations(); err != nil {
		return err
	}
	return r.ensureWorkspaceIndexes()
}

// migrateExecutorProfiles adds mcp_policy column and drops is_default from executor_profiles.
func (r *Repository) migrateExecutorProfiles() error {
	// Add mcp_policy column if it doesn't exist
	_, _ = r.db.Exec(`ALTER TABLE executor_profiles ADD COLUMN mcp_policy TEXT DEFAULT ''`)
	// Drop is_default column - SQLite doesn't support DROP COLUMN before 3.35.0,
	// so we just ignore the old column if present. New schema omits it.
	return nil
}

// migrateTaskSessions adds new columns to task_sessions.
func (r *Repository) migrateTaskSessions() error {
	_, _ = r.db.Exec(`ALTER TABLE task_sessions ADD COLUMN executor_profile_id TEXT DEFAULT ''`)
	return nil
}

// runMigrations applies idempotent ALTER TABLE migrations for schema evolution.
func (r *Repository) runMigrations() error {
	// Add last_message_uuid column to executors_running (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE executors_running ADD COLUMN last_message_uuid TEXT DEFAULT ''`)
	return nil
}

func (r *Repository) initCoreSchema() error {
	if err := r.initInfraSchema(); err != nil {
		return err
	}
	if err := r.initTaskSchema(); err != nil {
		return err
	}
	return r.initCoreIndexes()
}

func (r *Repository) initInfraSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		owner_id TEXT DEFAULT '',
		default_executor_id TEXT DEFAULT '',
		default_environment_id TEXT DEFAULT '',
		default_agent_profile_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS executors (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		is_system INTEGER NOT NULL DEFAULT 0,
		resumable INTEGER NOT NULL DEFAULT 1,
		config TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP
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
		last_seen_at TIMESTAMP,
		error_message TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS executor_profiles (
		id TEXT PRIMARY KEY,
		executor_id TEXT NOT NULL,
		name TEXT NOT NULL,
		mcp_policy TEXT DEFAULT '',
		config TEXT DEFAULT '{}',
		prepare_script TEXT DEFAULT '',
		cleanup_script TEXT DEFAULT '',
		env_vars TEXT DEFAULT '[]',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (executor_id) REFERENCES executors(id)
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
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`)
	return err
}

func (r *Repository) initTaskSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		state TEXT DEFAULT 'TODO',
		priority INTEGER DEFAULT 0,
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		archived_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS task_repositories (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		base_branch TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
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
		pull_before_worktree INTEGER NOT NULL DEFAULT 1,
		setup_script TEXT DEFAULT '',
		cleanup_script TEXT DEFAULT '',
		dev_script TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repository_scripts (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);
	`)
	return err
}

func (r *Repository) initCoreIndexes() error {
	_, err := r.db.Exec(`
	CREATE INDEX IF NOT EXISTS idx_tasks_workflow_id ON tasks(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_workflow_step_id ON tasks(workflow_step_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_archived_at ON tasks(archived_at);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_task_id ON task_repositories(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_repository_id ON task_repositories(repository_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_workspace_id ON repositories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_repository_scripts_repo_id ON repository_scripts(repository_id);
	`)
	return err
}

func (r *Repository) initPlansSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_plans (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL DEFAULT 'Plan',
		content TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT 'agent',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_task_plans_task_id ON task_plans(task_id);
	`)
	return err
}

func (r *Repository) initSessionSchema() error {
	if err := r.initMessageTurnSchema(); err != nil {
		return err
	}
	return r.initSessionWorktreeSchema()
}

func (r *Repository) initMessageTurnSchema() error {
	_, err := r.db.Exec(`
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
		created_at TIMESTAMP NOT NULL,
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
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		metadata TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_turns_session_id ON task_session_turns(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_turns_session_started ON task_session_turns(task_session_id, started_at);
	CREATE INDEX IF NOT EXISTS idx_turns_task_id ON task_session_turns(task_id);
	`)
	return err
}

func (r *Repository) initSessionWorktreeSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_execution_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL,
		executor_id TEXT DEFAULT '',
		executor_profile_id TEXT DEFAULT '',
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
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		updated_at TIMESTAMP NOT NULL,
		is_primary INTEGER DEFAULT 0,
		workflow_step_id TEXT DEFAULT '',
		is_passthrough INTEGER DEFAULT 0,
		review_status TEXT DEFAULT '',
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
	`)
	return err
}

func (r *Repository) initGitSchema() error {
	_, err := r.db.Exec(`
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
		created_at TIMESTAMP NOT NULL,
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
		committed_at TIMESTAMP NOT NULL,
		pre_commit_snapshot_id TEXT DEFAULT '',
		post_commit_snapshot_id TEXT DEFAULT '',
		files_changed INTEGER DEFAULT 0,
		insertions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_session_commits_session ON task_session_commits(session_id, committed_at DESC);
	CREATE INDEX IF NOT EXISTS idx_session_commits_sha ON task_session_commits(commit_sha);
	`)
	return err
}

func (r *Repository) initReviewSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS session_file_reviews (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		reviewed INTEGER NOT NULL DEFAULT 0,
		diff_hash TEXT NOT NULL DEFAULT '',
		reviewed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		UNIQUE(session_id, file_path)
	);
	CREATE INDEX IF NOT EXISTS idx_session_file_reviews_session ON session_file_reviews(session_id);
	`)
	return err
}
