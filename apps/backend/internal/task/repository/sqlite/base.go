// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
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
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_archived ON tasks(workspace_id, archived_at)`); err != nil {
		return err
	}
	return nil
}

// ensureMessageMetadataIndexes creates indexes on JSON metadata fields for fast lookups.
func (r *Repository) ensureMessageMetadataIndexes() error {
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_metadata_tool_call_id ON task_session_messages(task_session_id, json_extract(metadata, '$.tool_call_id'))`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_metadata_pending_id ON task_session_messages(task_session_id, json_extract(metadata, '$.pending_id'))`); err != nil {
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
	if err := r.backfillTaskEnvironments(); err != nil {
		return err
	}
	if err := r.ensureWorkspaceIndexes(); err != nil {
		return err
	}
	return r.ensureMessageMetadataIndexes()
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
	// Add metadata column to executors_running (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE executors_running ADD COLUMN metadata TEXT DEFAULT '{}'`)
	// Add is_ephemeral column to tasks for quick chat (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN is_ephemeral INTEGER NOT NULL DEFAULT 0`)
	// Add checkout_branch column to task_repositories (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE task_repositories ADD COLUMN checkout_branch TEXT DEFAULT ''`)
	// Add base_commit_sha column to task_sessions for tracking session start commit (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE task_sessions ADD COLUMN base_commit_sha TEXT DEFAULT ''`)
	// Add default_config_agent_profile_id column to workspaces (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE workspaces ADD COLUMN default_config_agent_profile_id TEXT DEFAULT ''`)
	// Add task_environment_id column to task_sessions for shared environment reference (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE task_sessions ADD COLUMN task_environment_id TEXT DEFAULT ''`)
	// Add parent_id column to tasks for subtask support (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN parent_id TEXT DEFAULT ''`)
	// Remove FK constraint on workflow_id to allow ephemeral tasks without workflows
	if err := r.migrateTasksRemoveWorkflowFK(); err != nil {
		return err
	}
	// Remove deprecated workflow_step_id column from task_sessions
	if err := r.migrateSessionsRemoveWorkflowStepID(); err != nil {
		return err
	}
	// Add sort_order column to workflows for user-defined ordering (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE workflows ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	// Add agent_profile_id column to workflows for per-workflow agent profile override (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE workflows ADD COLUMN agent_profile_id TEXT DEFAULT ''`)
	// Add hidden flag to workflows for system-only flows excluded from management UI (ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE workflows ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`)
	return nil
}

// recreateTable checks whether tableName's DDL contains triggerPhrase and, if so,
// runs statements inside a transaction with FK enforcement disabled.
// This is the standard SQLite pattern for dropping columns or FK constraints,
// since SQLite has no ALTER TABLE DROP COLUMN / DROP CONSTRAINT.
// Note: PRAGMA statements cannot run inside a transaction in SQLite, so FK enforcement
// is toggled outside the transaction. The writer pool must have MaxOpenConns(1) so that
// the PRAGMA and the subsequent transaction use the same connection.
func (r *Repository) recreateTable(tableName, triggerPhrase string, statements []string) error {
	var tableSql string
	err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, tableName).Scan(&tableSql)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // Table doesn't exist yet; migration not applicable
	}
	if err != nil {
		return fmt.Errorf("query %s schema: %w", tableName, err)
	}
	if !strings.Contains(tableSql, triggerPhrase) {
		return nil // Trigger phrase absent; migration already applied or not needed
	}

	if _, err := r.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = r.db.Exec(`PRAGMA foreign_keys=ON`) }()

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration %s failed: %w", tableName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}
	return nil
}

// migrateTasksRemoveWorkflowFK removes the foreign key constraint on workflow_id
// to allow ephemeral tasks (quick chat) to have empty workflow_id.
func (r *Repository) migrateTasksRemoveWorkflowFK() error {
	return r.recreateTable("tasks", "FOREIGN KEY (workflow_id)", []string{
		`CREATE TABLE tasks_new (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`INSERT INTO tasks_new SELECT
			id, workspace_id, workflow_id, workflow_step_id, title, description,
			state, priority, position, metadata, is_ephemeral, parent_id, archived_at, created_at, updated_at
		FROM tasks`,
		`DROP TABLE tasks`,
		`ALTER TABLE tasks_new RENAME TO tasks`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_id ON tasks(workflow_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_step_id ON tasks(workflow_step_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_archived_at ON tasks(archived_at)`,
	})
}

// migrateSessionsRemoveWorkflowStepID removes the deprecated workflow_step_id column
// from task_sessions. Workflow step is now tracked on the task, not the session.
func (r *Repository) migrateSessionsRemoveWorkflowStepID() error {
	return r.recreateTable("task_sessions", "workflow_step_id", []string{
		`CREATE TABLE task_sessions_new (
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
			is_passthrough INTEGER DEFAULT 0,
			review_status TEXT DEFAULT '',
			base_commit_sha TEXT DEFAULT '',
			task_environment_id TEXT DEFAULT '',
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`INSERT INTO task_sessions_new SELECT
			id, task_id, agent_execution_id, container_id, agent_profile_id,
			executor_id, executor_profile_id, environment_id, repository_id, base_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, is_passthrough, review_status,
			COALESCE(base_commit_sha, ''), COALESCE(task_environment_id, '')
		FROM task_sessions`,
		`DROP TABLE task_sessions`,
		`ALTER TABLE task_sessions_new RENAME TO task_sessions`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state)`,
	})
}

type backfillRow struct {
	taskID, executorID, executorProfileID string
	repositoryID, containerID             string
	startedAt                             string
}

// backfillTaskEnvironments creates TaskEnvironment records for historical tasks
// that have sessions but no environment, and links orphaned sessions.
// Idempotent: tasks with existing environments are skipped.
func (r *Repository) backfillTaskEnvironments() error {
	orphaned, err := r.findOrphanedTasks()
	if err != nil {
		return err
	}
	if len(orphaned) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("backfill: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, row := range orphaned {
		if err := r.backfillSingleTask(tx, row); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// findOrphanedTasks returns tasks that have sessions but no task_environments row.
func (r *Repository) findOrphanedTasks() ([]backfillRow, error) {
	rows, err := r.db.Query(`
		SELECT ts.task_id,
		       COALESCE(ts.executor_id, ''),
		       COALESCE(ts.executor_profile_id, ''),
		       COALESCE(ts.repository_id, ''),
		       COALESCE(ts.container_id, ''),
		       ts.started_at
		FROM task_sessions ts
		LEFT JOIN task_environments te ON te.task_id = ts.task_id
		WHERE te.id IS NULL
		GROUP BY ts.task_id
	`)
	if err != nil {
		return nil, fmt.Errorf("backfill: query orphaned tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orphaned []backfillRow
	for rows.Next() {
		var row backfillRow
		if err := rows.Scan(&row.taskID, &row.executorID, &row.executorProfileID,
			&row.repositoryID, &row.containerID, &row.startedAt); err != nil {
			return nil, fmt.Errorf("backfill: scan: %w", err)
		}
		orphaned = append(orphaned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("backfill: rows: %w", err)
	}
	return orphaned, nil
}

// backfillSingleTask creates a task_environment and links sessions for one orphaned task.
func (r *Repository) backfillSingleTask(tx *sql.Tx, row backfillRow) error {
	envID := uuid.New().String()

	// Look up executor type from executors table, default to "local_pc"
	var executorType string
	if err := tx.QueryRow(`SELECT type FROM executors WHERE id = ?`, row.executorID).Scan(&executorType); err != nil {
		executorType = "local_pc"
	}

	// Look up worktree info from task_session_worktrees (best effort)
	var wtID, wtPath, wtBranch string
	_ = tx.QueryRow(`
		SELECT w.worktree_id, w.worktree_path, w.worktree_branch
		FROM task_session_worktrees w
		JOIN task_sessions ts ON ts.id = w.session_id
		WHERE ts.task_id = ?
		LIMIT 1
	`, row.taskID).Scan(&wtID, &wtPath, &wtBranch)

	// Insert task_environment with status "stopped" (historical, agentctl not running)
	if _, err := tx.Exec(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id,
			executor_profile_id, agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, '', 0, 'stopped', ?, ?, ?, '', ?, '', ?, datetime('now'))
	`, envID, row.taskID, row.repositoryID, executorType, row.executorID,
		row.executorProfileID, wtID, wtPath, wtBranch, row.containerID, row.startedAt); err != nil {
		return fmt.Errorf("backfill: insert env for task %s: %w", row.taskID, err)
	}

	// Link all sessions for this task that lack task_environment_id
	if _, err := tx.Exec(`
		UPDATE task_sessions
		SET task_environment_id = ?
		WHERE task_id = ? AND (task_environment_id = '' OR task_environment_id IS NULL)
	`, envID, row.taskID); err != nil {
		return fmt.Errorf("backfill: link sessions for task %s: %w", row.taskID, err)
	}
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
		default_config_agent_profile_id TEXT DEFAULT '',
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
		metadata TEXT DEFAULT '{}',
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
		hidden INTEGER NOT NULL DEFAULT 0,
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
		workflow_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		state TEXT DEFAULT 'TODO',
		priority INTEGER DEFAULT 0,
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		archived_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
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
	if _, err := r.db.Exec(`
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
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_plan_revisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		revision_number INTEGER NOT NULL,
		title TEXT NOT NULL DEFAULT 'Plan',
		content TEXT NOT NULL DEFAULT '',
		author_kind TEXT NOT NULL DEFAULT 'agent',
		author_name TEXT NOT NULL DEFAULT '',
		revert_of_revision_id TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		UNIQUE (task_id, revision_number)
	);
	CREATE INDEX IF NOT EXISTS idx_task_plan_revisions_task_created
		ON task_plan_revisions(task_id, created_at DESC);
	-- Hot-path index: GetLatestTaskPlanRevision (called on every plan write
	-- as part of the coalesce check), ListTaskPlanRevisions, and the
	-- MAX(revision_number) lookup in WritePlanRevision all order/scan by
	-- (task_id, revision_number DESC). With this index the latest-row lookup
	-- is O(1) instead of an O(N) scan + sort per task.
	CREATE INDEX IF NOT EXISTS idx_task_plan_revisions_task_number
		ON task_plan_revisions(task_id, revision_number DESC);
	`); err != nil {
		return err
	}
	return r.backfillInitialPlanRevisions()
}

// backfillInitialPlanRevisions ensures every existing task_plans row has at least
// one corresponding revision. Runs once at startup and is idempotent.
func (r *Repository) backfillInitialPlanRevisions() error {
	rows, err := r.db.Query(`
	SELECT p.id, p.task_id, p.title, p.content, p.created_by, p.created_at, p.updated_at
	FROM task_plans p
	WHERE NOT EXISTS (
		SELECT 1 FROM task_plan_revisions r WHERE r.task_id = p.task_id
	)`)
	if err != nil {
		return fmt.Errorf("query plans missing revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		id, taskID, title, content, createdBy string
		createdAt, updatedAt                  interface{}
	}
	var pending []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.taskID, &x.title, &x.content, &x.createdBy, &x.createdAt, &x.updatedAt); err != nil {
			return fmt.Errorf("scan plan for backfill: %w", err)
		}
		pending = append(pending, x)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate plans for backfill: %w", err)
	}

	for _, x := range pending {
		authorKind := x.createdBy
		// Match CreateTaskPlan (plan.go) and the task_plan_revisions column DEFAULT 'agent'.
		if authorKind != "user" && authorKind != authorKindAgent {
			authorKind = authorKindAgent
		}
		_, err := r.db.Exec(r.db.Rebind(`
			INSERT INTO task_plan_revisions
			  (id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
			VALUES (?, ?, 1, ?, ?, ?, 'legacy', NULL, ?, ?)
		`), uuid.New().String(), x.taskID, x.title, x.content, authorKind, x.createdAt, x.updatedAt)
		if err != nil {
			return fmt.Errorf("backfill revision for task %s: %w", x.taskID, err)
		}
	}
	return nil
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
		is_passthrough INTEGER DEFAULT 0,
		review_status TEXT DEFAULT '',
		base_commit_sha TEXT DEFAULT '',
		task_environment_id TEXT DEFAULT '',
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state);

	CREATE TABLE IF NOT EXISTS task_environments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT DEFAULT '',
		executor_type TEXT NOT NULL DEFAULT '',
		executor_id TEXT DEFAULT '',
		executor_profile_id TEXT DEFAULT '',
		agent_execution_id TEXT DEFAULT '',
		control_port INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'creating',
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		workspace_path TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		sandbox_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_environments_task_id ON task_environments(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_environments_status ON task_environments(status);

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
