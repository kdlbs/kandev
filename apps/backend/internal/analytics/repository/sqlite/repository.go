// Package sqlite provides SQLite-based analytics repository implementations.
package sqlite

import "database/sql"

// Repository provides SQLite-based analytics operations.
type Repository struct {
	db *sql.DB
}

// NewWithDB creates a new analytics repository with an existing database connection.
// It automatically creates performance indexes for stats queries.
func NewWithDB(dbConn *sql.DB) (*Repository, error) {
	repo := &Repository{db: dbConn}
	if err := repo.ensureStatsIndexes(); err != nil {
		return nil, err
	}
	return repo, nil
}

// Close is a no-op because this repository does not own the database connection.
func (r *Repository) Close() error {
	return nil
}

// ensureStatsIndexes creates performance indexes for stats queries if they don't already exist.
// Query planner statistics are maintained by PRAGMA optimize on connection close (see persistence/provider.go).
func (r *Repository) ensureStatsIndexes() error {
	indexes := []string{
		// Tasks table indexes
		`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_created
			ON tasks(workspace_id, created_at)`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_state
			ON tasks(workflow_step_id, state)`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_updated
			ON tasks(workspace_id, updated_at)`,

		// Task sessions indexes
		`CREATE INDEX IF NOT EXISTS idx_sessions_task_started
			ON task_sessions(task_id, started_at)`,

		`CREATE INDEX IF NOT EXISTS idx_sessions_agent_started
			ON task_sessions(agent_profile_id, started_at)`,

		`CREATE INDEX IF NOT EXISTS idx_sessions_repo
			ON task_sessions(repository_id)`,

		// Task session turns indexes
		`CREATE INDEX IF NOT EXISTS idx_turns_session_times
			ON task_session_turns(task_session_id, started_at, completed_at)`,

		`CREATE INDEX IF NOT EXISTS idx_turns_task
			ON task_session_turns(task_id)`,

		// Task session messages indexes
		`CREATE INDEX IF NOT EXISTS idx_messages_session_created
			ON task_session_messages(task_session_id, created_at)`,

		`CREATE INDEX IF NOT EXISTS idx_messages_author_type
			ON task_session_messages(task_session_id, author_type, type)`,

		`CREATE INDEX IF NOT EXISTS idx_messages_turn
			ON task_session_messages(turn_id)`,

		// Task session commits indexes
		`CREATE INDEX IF NOT EXISTS idx_commits_session_time
			ON task_session_commits(session_id, committed_at)`,

		// Task repositories indexes
		`CREATE INDEX IF NOT EXISTS idx_task_repos_task
			ON task_repositories(task_id, repository_id)`,

		`CREATE INDEX IF NOT EXISTS idx_task_repos_repo
			ON task_repositories(repository_id, task_id)`,

		// Repositories indexes (with soft delete filter)
		`CREATE INDEX IF NOT EXISTS idx_repos_workspace
			ON repositories(workspace_id)
			WHERE deleted_at IS NULL`,

	}

	for _, idx := range indexes {
		if _, err := r.db.Exec(idx); err != nil {
			return err
		}
	}

	return nil
}

