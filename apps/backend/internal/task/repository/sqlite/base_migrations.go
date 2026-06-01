// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// migrateExecutorProfiles adds mcp_policy column and drops is_default from executor_profiles.
func (r *Repository) migrateExecutorProfiles() error {
	r.migrate.Apply("executor_profiles.mcp_policy", `ALTER TABLE executor_profiles ADD COLUMN mcp_policy TEXT DEFAULT ''`)
	// Drop is_default column - SQLite doesn't support DROP COLUMN before 3.35.0,
	// so we just ignore the old column if present. New schema omits it.
	return nil
}

// migrateTaskSessions adds new columns to task_sessions.
func (r *Repository) migrateTaskSessions() error {
	r.migrate.Apply("task_sessions.executor_profile_id", `ALTER TABLE task_sessions ADD COLUMN executor_profile_id TEXT DEFAULT ''`)
	return nil
}

// migrateSessionsAddCostColumns backfills the per-session cost/token columns on
// task_sessions. These are otherwise only introduced inside two gated
// CREATE-TABLE statements — the fresh create (a no-op once the table exists) and
// the agent_execution_id-drop rebuild (gated on that column still being present).
// A DB that no longer contains the rebuild trigger columns would never gain the
// cost columns, breaking the office cost subscriber's IncrementTaskSessionUsage
// with "no such column: tokens_in". These additive ALTERs are idempotent — the
// MigrateLogger swallows "duplicate column name" on DBs that already have them.
func (r *Repository) migrateSessionsAddCostColumns() {
	r.migrate.Apply("task_sessions.cost_subcents", `ALTER TABLE task_sessions ADD COLUMN cost_subcents INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_sessions.tokens_in", `ALTER TABLE task_sessions ADD COLUMN tokens_in INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_sessions.tokens_out", `ALTER TABLE task_sessions ADD COLUMN tokens_out INTEGER NOT NULL DEFAULT 0`)
}

// runMigrations applies idempotent ALTER TABLE migrations for schema evolution.
func (r *Repository) runMigrations() error {
	r.migrate.Apply("executors_running.last_message_uuid", `ALTER TABLE executors_running ADD COLUMN last_message_uuid TEXT DEFAULT ''`)
	r.migrate.Apply("executors_running.metadata", `ALTER TABLE executors_running ADD COLUMN metadata TEXT DEFAULT '{}'`)
	r.migrate.Apply("tasks.is_ephemeral", `ALTER TABLE tasks ADD COLUMN is_ephemeral INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_repositories.checkout_branch", `ALTER TABLE task_repositories ADD COLUMN checkout_branch TEXT DEFAULT ''`)
	// Multi-branch support: drop the old UNIQUE(task_id, repository_id) and
	// replace it with UNIQUE(task_id, repository_id, checkout_branch) so the
	// same repo can appear multiple times in a task on different branches.
	if err := r.migrateTaskRepositoriesAllowMultiBranch(); err != nil {
		return err
	}
	r.migrate.Apply("task_sessions.base_commit_sha", `ALTER TABLE task_sessions ADD COLUMN base_commit_sha TEXT DEFAULT ''`)
	r.migrate.Apply("workspaces.default_config_agent_profile_id", `ALTER TABLE workspaces ADD COLUMN default_config_agent_profile_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_sessions.task_environment_id", `ALTER TABLE task_sessions ADD COLUMN task_environment_id TEXT DEFAULT ''`)
	r.migrate.Apply("tasks.parent_id", `ALTER TABLE tasks ADD COLUMN parent_id TEXT DEFAULT ''`)
	// Remove FK constraint on workflow_id to allow ephemeral tasks without workflows
	if err := r.migrateTasksRemoveWorkflowFK(); err != nil {
		return err
	}
	// Remove deprecated workflow_step_id column from task_sessions
	if err := r.migrateSessionsRemoveWorkflowStepID(); err != nil {
		return err
	}
	// Backfill executors_running from task_sessions and drop the denormalized
	// agent_execution_id / container_id columns. After this migration,
	// executors_running is the single source of truth for "active execution per
	// session" - see persistence.go in the lifecycle package for the new ownership
	// model. Order matters: backfill must run BEFORE the column drop.
	if err := r.backfillExecutorsRunningFromTaskSessions(); err != nil {
		return err
	}
	if err := r.migrateSessionsRemoveAgentExecutionID(); err != nil {
		return err
	}
	// Must run BEFORE migrateTaskEnvironmentsRemoveAgentExecutionID, which copies task_dir_name into the recreated table.
	r.migrate.Apply("task_environments.task_dir_name", `ALTER TABLE task_environments ADD COLUMN task_dir_name TEXT DEFAULT ''`)
	if err := r.migrateTaskEnvironmentsRemoveAgentExecutionID(); err != nil {
		return err
	}
	r.migrate.Apply("workflows.sort_order", `ALTER TABLE workflows ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("workflows.agent_profile_id", `ALTER TABLE workflows ADD COLUMN agent_profile_id TEXT DEFAULT ''`)
	r.migrate.Apply("workflows.hidden", `ALTER TABLE workflows ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_sessions.workspace_path", `ALTER TABLE task_sessions ADD COLUMN workspace_path TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.copy_files", `ALTER TABLE repositories ADD COLUMN copy_files TEXT DEFAULT ''`)

	// Backfill the per-session cost/token columns. Runs after the gated
	// task_sessions rebuilds above so it repairs legacy DBs whose schema can no
	// longer trigger a rebuild (see migrateSessionsAddCostColumns).
	r.migrateSessionsAddCostColumns()

	// Office task extensions - net-new columns on existing main tables.
	// Idempotent ALTERs; main upgrades pick them up at first boot.
	// The transient in-branch columns (requires_approval,
	// execution_policy, execution_state, assignee_agent_profile_id,
	// task_sessions.agent_instance_id) were never on main and are
	// therefore not added or dropped here.
	r.migrate.Apply("tasks.origin", `ALTER TABLE tasks ADD COLUMN origin TEXT DEFAULT 'manual'`)
	r.migrate.Apply("tasks.project_id", `ALTER TABLE tasks ADD COLUMN project_id TEXT DEFAULT ''`)
	r.migrate.Apply("tasks.labels", `ALTER TABLE tasks ADD COLUMN labels TEXT DEFAULT '[]'`)
	r.migrate.Apply("tasks.identifier", `ALTER TABLE tasks ADD COLUMN identifier TEXT`)
	// Office task-handoffs phase 6 - tag tasks archived as part of a cascade so
	// unarchive can restore exactly the descendants that cascade archived.
	r.migrate.Apply("tasks.archived_by_cascade_id", `ALTER TABLE tasks ADD COLUMN archived_by_cascade_id TEXT DEFAULT ''`)

	// Office workspace extensions
	r.migrate.Apply("workspaces.task_prefix", `ALTER TABLE workspaces ADD COLUMN task_prefix TEXT DEFAULT 'KAN'`)
	r.migrate.Apply("workspaces.task_sequence", `ALTER TABLE workspaces ADD COLUMN task_sequence INTEGER DEFAULT 0`)
	r.migrate.Apply("workspaces.office_workflow_id", `ALTER TABLE workspaces ADD COLUMN office_workflow_id TEXT DEFAULT ''`)

	// Office session cost tracking extensions are declared in
	// initSessionWorktreeSchema's CREATE TABLE (cost_subcents, tokens_in,
	// tokens_out). task_sessions.agent_profile_id existed on main as
	// NOT NULL; migrateSessionsRemoveAgentExecutionID rebuilds the table
	// with the column nullable and the cost columns added.

	r.migrate.Apply("workflows.is_system", `ALTER TABLE workflows ADD COLUMN is_system INTEGER DEFAULT 0`)

	// Phase 2 (ADR-0004) - workflows.style is a UX hint for the frontend
	// ("kanban" | "office" | "custom"). Backend code MUST NOT branch on
	// this value. Idempotent ALTER; default "kanban" preserves the current
	// presentation for existing workflows.
	r.migrate.Apply("workflows.style", `ALTER TABLE workflows ADD COLUMN style TEXT NOT NULL DEFAULT 'kanban'`)

	// ADR 0005 Wave F — ensure the runner-projection tables exist so
	// task SELECTs that reference them via correlated subquery don't
	// fail. Required for tests and any environment where the workflow
	// repo hasn't run yet.
	r.ensureRunnerProjectionTables()

	return nil
}

// recreateTable checks whether tableName's DDL contains triggerPhrase and, if so,
// runs statements inside a transaction with FK enforcement disabled.
// This is the standard SQLite pattern for dropping columns or FK constraints,
// since SQLite has no ALTER TABLE DROP COLUMN / DROP CONSTRAINT.
// Note: PRAGMA statements cannot run inside a transaction in SQLite, so FK enforcement
// is toggled outside the transaction. The writer pool must have MaxOpenConns(1) so that
// the PRAGMA and the subsequent transaction use the same connection.
// Returns true if the migration actually ran (gate fired), false if it was a no-op.
func (r *Repository) recreateTable(tableName, triggerPhrase string, statements []string) (bool, error) {
	var tableSql string
	err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, tableName).Scan(&tableSql)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // Table doesn't exist yet; migration not applicable
	}
	if err != nil {
		return false, fmt.Errorf("query %s schema: %w", tableName, err)
	}
	if !strings.Contains(tableSql, triggerPhrase) {
		return false, nil // Trigger phrase absent; migration already applied or not needed
	}

	if _, err := r.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return false, fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = r.db.Exec(`PRAGMA foreign_keys=ON`) }()

	tx, err := r.db.Beginx()
	if err != nil {
		return false, fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return false, fmt.Errorf("migration %s failed: %w", tableName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit migration transaction: %w", err)
	}
	return true, nil
}

// recreateTableNamed wraps recreateTable and logs "migration applied" when the
// gate fires (trigger phrase found and statements ran).
func (r *Repository) recreateTableNamed(name, tableName, triggerPhrase string, statements []string) error {
	fired, err := r.recreateTable(tableName, triggerPhrase, statements)
	if err != nil {
		return err
	}
	if fired && r.log != nil {
		r.log.Info("migration applied", zap.String("name", name))
	}
	return nil
}

// migrateTasksRemoveWorkflowFK removes the foreign key constraint on workflow_id
// to allow ephemeral tasks (quick chat) to have empty workflow_id.
func (r *Repository) migrateTasksRemoveWorkflowFK() error {
	return r.recreateTableNamed("tasks.recreate_drop_workflow_fk", "tasks", "FOREIGN KEY (workflow_id)", []string{
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

// backfillExecutorsRunningFromTaskSessions creates an executors_running row for
// any session that has a non-empty task_sessions.agent_execution_id but no matching
// executors_running row. This preserves the data we're about to drop from
// task_sessions in the canonical executors_running table.
//
// Sessions with empty agent_execution_id are skipped intentionally — they were
// never launched (e.g. CREATED state, PR-watcher review tasks), and the new
// invariant says "executors_running row exists iff session was launched".
//
// Idempotent: rows that already exist on either side are left untouched.
func (r *Repository) backfillExecutorsRunningFromTaskSessions() error {
	// Check whether task_sessions still has the column. If migration already ran,
	// the column is gone and there's nothing to backfill.
	var tableSql string
	if err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='task_sessions'`).Scan(&tableSql); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("backfill executors_running: read schema: %w", err)
	}
	if !strings.Contains(tableSql, "agent_execution_id") {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	// SELECT … LEFT JOIN to find sessions with execution data but no executors_running row.
	// Insert with the minimum field set; runtime/status are best-effort defaults
	// (subsequent Launch / Resume will overwrite via the lifecycle manager's persistence).
	if _, err := r.db.Exec(`
		INSERT INTO executors_running (
			id, session_id, task_id, executor_id, runtime, status, resumable,
			resume_token, last_message_uuid, agent_execution_id, container_id,
			agentctl_url, agentctl_port, pid, worktree_id, worktree_path, worktree_branch,
			error_message, metadata, created_at, updated_at
		)
		-- executors_running.id mirrors session_id (both columns must hold the same UUID
		-- so the row is self-referential by design — the dupword linter complaint is
		-- a false positive on the SQL projection list).
		SELECT
			ts.id AS er_id,
			ts.id AS er_session_id,
			ts.task_id, ts.executor_id, '', 'unknown', 1,
			'', '', ts.agent_execution_id, ts.container_id,
			'', 0, 0, '', '', '',
			'', '{}', ts.started_at, ?
		FROM task_sessions ts
		LEFT JOIN executors_running er ON er.session_id = ts.id
		WHERE COALESCE(ts.agent_execution_id, '') != '' AND er.id IS NULL
	`, now); err != nil {
		return fmt.Errorf("backfill executors_running: %w", err)
	}
	return nil
}

// migrateSessionsRemoveAgentExecutionID drops the agent_execution_id and
// container_id columns from task_sessions. After this migration, executors_running
// is the single source of truth for both fields — no more denormalization.
//
// Must run after backfillExecutorsRunningFromTaskSessions so any data we're about
// to drop is preserved on the executors_running side.
//
// The trigger phrase "agent_execution_id" detects when the migration hasn't yet
// run (column still present); recreateTable is a no-op once the column is gone.
func (r *Repository) migrateSessionsRemoveAgentExecutionID() error {
	return r.recreateTableNamed("task_sessions.recreate_drop_agent_execution_id", "task_sessions", "agent_execution_id", []string{
		`CREATE TABLE task_sessions_new (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_profile_id TEXT,
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
			cost_subcents INTEGER NOT NULL DEFAULT 0,
			tokens_in INTEGER NOT NULL DEFAULT 0,
			tokens_out INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`INSERT INTO task_sessions_new SELECT
			id, task_id, agent_profile_id,
			executor_id, executor_profile_id, environment_id, repository_id, base_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, is_passthrough, review_status,
			COALESCE(base_commit_sha, ''), COALESCE(task_environment_id, ''),
			0, 0, 0
		FROM task_sessions`,
		`DROP TABLE task_sessions`,
		`ALTER TABLE task_sessions_new RENAME TO task_sessions`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state)`,
	})
}

// migrateTaskEnvironmentsRemoveAgentExecutionID drops the agent_execution_id
// column from task_environments. Like task_sessions, this column was a stale
// denormalized copy that drifted from the in-memory store. The orchestrator
// now reads execution state from executors_running only.
func (r *Repository) migrateTaskEnvironmentsRemoveAgentExecutionID() error {
	return r.recreateTableNamed("task_environments.recreate_drop_agent_execution_id", "task_environments", "agent_execution_id", []string{
		`CREATE TABLE task_environments_new (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT DEFAULT '',
			executor_type TEXT NOT NULL DEFAULT '',
			executor_id TEXT DEFAULT '',
			executor_profile_id TEXT DEFAULT '',
			control_port INTEGER DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'creating',
			worktree_id TEXT DEFAULT '',
			worktree_path TEXT DEFAULT '',
			worktree_branch TEXT DEFAULT '',
			workspace_path TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			sandbox_id TEXT DEFAULT '',
			task_dir_name TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`INSERT INTO task_environments_new SELECT
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			control_port, status, worktree_id, worktree_path, worktree_branch,
			workspace_path, container_id, sandbox_id,
			COALESCE(task_dir_name, ''), created_at, updated_at
		FROM task_environments`,
		`DROP TABLE task_environments`,
		`ALTER TABLE task_environments_new RENAME TO task_environments`,
		`CREATE INDEX IF NOT EXISTS idx_task_environments_task_id ON task_environments(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_environments_status ON task_environments(status)`,
		// uniq_task_environments_task_id is created by ensureTaskEnvironmentTaskUniqueIndex
		// AFTER healDuplicateTaskEnvironments collapses any pre-existing duplicates.
		// Creating it here would fail on databases that still have duplicate task_id rows.
	})
}

// migrateTaskRepositoriesAllowMultiBranch swaps the legacy
// UNIQUE(task_id, repository_id) constraint on task_repositories for
// UNIQUE(task_id, repository_id, base_branch, checkout_branch). The wider
// key lets the same repo coexist on N branches per task, including the
// worktree-executor case where the branch lives in base_branch and
// checkout_branch is empty. The trigger phrase matches both the legacy
// two-column constraint and the intermediate three-column variant added in
// the first multi-branch landing; the recreate becomes a no-op once the
// four-column constraint is in place.
func (r *Repository) migrateTaskRepositoriesAllowMultiBranch() error {
	if err := r.recreateTaskRepositoriesForMultiBranch("UNIQUE(task_id, repository_id)\n"); err != nil {
		return err
	}
	return r.recreateTaskRepositoriesForMultiBranch("UNIQUE(task_id, repository_id, checkout_branch)")
}

func (r *Repository) recreateTaskRepositoriesForMultiBranch(trigger string) error {
	return r.recreateTableNamed(
		"task_repositories.recreate_allow_multi_branch",
		"task_repositories",
		trigger,
		[]string{
			`CREATE TABLE task_repositories_new (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				repository_id TEXT NOT NULL,
				base_branch TEXT DEFAULT '',
				checkout_branch TEXT DEFAULT '',
				position INTEGER DEFAULT 0,
				metadata TEXT DEFAULT '{}',
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
				FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
				UNIQUE(task_id, repository_id, base_branch, checkout_branch)
			)`,
			`INSERT INTO task_repositories_new SELECT
				id, task_id, repository_id, base_branch,
				COALESCE(checkout_branch, ''),
				position, metadata, created_at, updated_at
			FROM task_repositories`,
			`DROP TABLE task_repositories`,
			`ALTER TABLE task_repositories_new RENAME TO task_repositories`,
			`CREATE INDEX IF NOT EXISTS idx_task_repositories_task_id ON task_repositories(task_id)`,
			`CREATE INDEX IF NOT EXISTS idx_task_repositories_repository_id ON task_repositories(repository_id)`,
		},
	)
}

// migrateSessionsRemoveWorkflowStepID removes the deprecated workflow_step_id column
// from task_sessions. Workflow step is now tracked on the task, not the session.
func (r *Repository) migrateSessionsRemoveWorkflowStepID() error {
	return r.recreateTableNamed("task_sessions.recreate_drop_workflow_step_id", "task_sessions", "workflow_step_id", []string{
		`CREATE TABLE task_sessions_new (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_execution_id TEXT NOT NULL DEFAULT '',
			container_id TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT,
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
//
// Pre-refactor this also read ts.container_id; that column was dropped from
// task_sessions when executors_running became the source of truth. Historical
// orphaned envs for already-launched sessions get container_id from the
// executors_running row via the LEFT JOIN; sessions without a row have empty
// container_id (they were never launched, so no container to track).
func (r *Repository) findOrphanedTasks() ([]backfillRow, error) {
	rows, err := r.db.Query(`
		SELECT ts.task_id,
		       COALESCE(ts.executor_id, ''),
		       COALESCE(ts.executor_profile_id, ''),
		       COALESCE(ts.repository_id, ''),
		       COALESCE(er.container_id, ''),
		       ts.started_at
		FROM task_sessions ts
		LEFT JOIN task_environments te ON te.task_id = ts.task_id
		LEFT JOIN executors_running er ON er.session_id = ts.id
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

// healTaskEnvironmentWorkspacePaths backfills workspace_path on worktree-mode
// envs that have a worktree_path set but an empty workspace_path. Such rows
// trigger ErrSessionWorkspaceNotReady forever in GetOrEnsureExecutionForEnvironment
// and leave shell terminals stuck on "Connecting terminal...".
//
// It also repairs rows where workspace_path was the task-root parent of
// worktree_path — a pre-fix value left by the legacy computeWorkspacePath
// that collapsed single-repo worktree paths via filepath.Dir. After the fix,
// workspace_path must equal worktree_path (the agent process cwd) so ACP
// session/load on cold start hits the same sanitized-cwd jsonl folder the
// agent wrote on hot start. Without this repair, existing single-repo
// Worktree tasks keep failing with -32002 after upgrade.
//
// Idempotent — once workspace_path == worktree_path nothing more is changed.
func (r *Repository) healTaskEnvironmentWorkspacePaths() error {
	// substr(...) prefix match is safer than LIKE here: paths may contain
	// "_" or "%", both of which are LIKE wildcards in SQLite.
	rows, err := r.db.Query(`
		SELECT id, worktree_path
		  FROM task_environments
		 WHERE executor_type = 'worktree'
		   AND COALESCE(worktree_path, '') != ''
		   AND COALESCE(workspace_path, '') != worktree_path
		   AND (
		         COALESCE(workspace_path, '') = ''
		         OR (length(workspace_path) < length(worktree_path)
		             AND substr(worktree_path, 1, length(workspace_path)) = workspace_path)
		       )
	`)
	if err != nil {
		return fmt.Errorf("heal workspace_path: query: %w", err)
	}
	type healRow struct{ id, worktreePath string }
	var pending []healRow
	for rows.Next() {
		var hr healRow
		if err := rows.Scan(&hr.id, &hr.worktreePath); err != nil {
			_ = rows.Close()
			return fmt.Errorf("heal workspace_path: scan: %w", err)
		}
		pending = append(pending, hr)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("heal workspace_path: rows: %w", err)
	}
	_ = rows.Close()
	if len(pending) == 0 {
		return nil
	}

	for _, hr := range pending {
		if _, err := r.db.Exec(
			`UPDATE task_environments SET workspace_path = ?, updated_at = datetime('now') WHERE id = ?`,
			hr.worktreePath, hr.id,
		); err != nil {
			return fmt.Errorf("heal workspace_path: update %s: %w", hr.id, err)
		}
	}
	return nil
}

// healDuplicateTaskEnvironments collapses rows where a single task has more
// than one task_environments row (race in lazy create). Keeps the most recently
// updated row and re-points any sessions still referring to the loser.
//
// Runs before ensureTaskEnvironmentTaskUniqueIndex so the unique constraint
// can be added cleanly. Idempotent — a no-op once the data is healed.
func (r *Repository) healDuplicateTaskEnvironments() error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("heal duplicate envs: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.Query(`
		SELECT task_id
		  FROM task_environments
		 GROUP BY task_id
		HAVING COUNT(*) > 1
	`)
	if err != nil {
		return fmt.Errorf("heal duplicate envs: list duplicates: %w", err)
	}
	var taskIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("heal duplicate envs: scan: %w", err)
		}
		taskIDs = append(taskIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("heal duplicate envs: rows: %w", err)
	}
	_ = rows.Close()

	for _, taskID := range taskIDs {
		if err := healDuplicateTaskEnvForTask(tx, taskID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// healDuplicateTaskEnvForTask keeps the most recently updated env for a task,
// re-points sessions on the loser rows to the winner, then deletes losers.
func healDuplicateTaskEnvForTask(tx *sql.Tx, taskID string) error {
	var winnerID string
	if err := tx.QueryRow(`
		SELECT id FROM task_environments
		 WHERE task_id = ?
		 ORDER BY updated_at DESC, created_at DESC
		 LIMIT 1
	`, taskID).Scan(&winnerID); err != nil {
		return fmt.Errorf("heal duplicate envs: find winner for task %s: %w", taskID, err)
	}

	if _, err := tx.Exec(`
		UPDATE task_sessions
		   SET task_environment_id = ?
		 WHERE task_id = ?
		   AND task_environment_id != ?
	`, winnerID, taskID, winnerID); err != nil {
		return fmt.Errorf("heal duplicate envs: relink sessions for task %s: %w", taskID, err)
	}

	if _, err := tx.Exec(`
		DELETE FROM task_environments
		 WHERE task_id = ?
		   AND id != ?
	`, taskID, winnerID); err != nil {
		return fmt.Errorf("heal duplicate envs: delete losers for task %s: %w", taskID, err)
	}
	return nil
}

// ensureTaskEnvironmentTaskUniqueIndex adds a UNIQUE index on
// task_environments(task_id) so that a future race in env creation fails loud
// instead of silently producing two rows for the same task. Must run AFTER
// healDuplicateTaskEnvironments, which collapses any pre-existing duplicates.
func (r *Repository) ensureTaskEnvironmentTaskUniqueIndex() error {
	_, err := r.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_task_environments_task_id
		    ON task_environments(task_id)
	`)
	return err
}

// healSessionTaskEnvironmentIDs backfills task_sessions.task_environment_id
// for any session whose task already has a task_environments row. Sessions
// created via paths that don't write the FK leave shell ops broken because
// every user-shell RPC is env-keyed and the frontend can't resolve session→env
// without this column. Idempotent: rows that already point at the env are
// untouched.
//
// Must run AFTER backfillTaskEnvironments + healDuplicateTaskEnvironments +
// ensureTaskEnvironmentTaskUniqueIndex so each task has exactly one env to
// link to.
func (r *Repository) healSessionTaskEnvironmentIDs() error {
	// LIMIT 1 is defensive — the unique index added by
	// ensureTaskEnvironmentTaskUniqueIndex guarantees ≤1 row per task at
	// runtime, but the SQL reads as non-deterministic in isolation. Belt
	// and suspenders.
	if _, err := r.db.Exec(`
		UPDATE task_sessions
		   SET task_environment_id = (
		         SELECT te.id FROM task_environments te WHERE te.task_id = task_sessions.task_id LIMIT 1
		       )
		 WHERE (task_environment_id = '' OR task_environment_id IS NULL)
		   AND EXISTS (
		         SELECT 1 FROM task_environments te WHERE te.task_id = task_sessions.task_id
		       )
	`); err != nil {
		return fmt.Errorf("heal session env id: update: %w", err)
	}
	return nil
}

// backfillSingleTask creates a task_environment and links sessions for one orphaned task.
func (r *Repository) backfillSingleTask(tx *sql.Tx, row backfillRow) error {
	envID := uuid.New().String()

	// Look up executor type from executors table. Default to "local_pc" ONLY
	// when the executor row genuinely doesn't exist (e.g. legacy session whose
	// executor was deleted). Any other scan error — driver failure, schema
	// mismatch, type assertion bug — must abort the migration so the operator
	// sees the underlying problem instead of every backfilled environment
	// silently getting the wrong executor_type.
	var executorType string
	if err := tx.QueryRow(`SELECT type FROM executors WHERE id = ?`, row.executorID).Scan(&executorType); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("backfill: lookup executor type for task %s: %w", row.taskID, err)
		}
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

	// Insert task_environment with status "stopped" (historical, agentctl not running).
	// Pre-refactor this also wrote agent_execution_id; that column is gone from
	// task_environments (executors_running is the only carrier of execution state now).
	if _, err := tx.Exec(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id,
			executor_profile_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 'stopped', ?, ?, ?, '', ?, '', ?, datetime('now'))
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

// backfillTaskEnvironmentRepos populates task_environment_repos from the legacy
// single-repo fields on task_environments. One row per environment that has a
// non-empty repository_id and no existing task_environment_repos row.
// Idempotent.
func (r *Repository) backfillTaskEnvironmentRepos() error {
	rows, err := r.db.Query(`
		SELECT te.id,
		       te.repository_id,
		       COALESCE(te.worktree_id, ''),
		       COALESCE(te.worktree_path, ''),
		       COALESCE(te.worktree_branch, ''),
		       te.created_at
		FROM task_environments te
		LEFT JOIN task_environment_repos ter ON ter.task_environment_id = te.id
		WHERE te.repository_id != '' AND ter.id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("backfill repos: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type envRepoRow struct {
		envID, repoID, wtID, wtPath, wtBranch, createdAt string
	}
	var pending []envRepoRow
	for rows.Next() {
		var row envRepoRow
		if err := rows.Scan(&row.envID, &row.repoID, &row.wtID, &row.wtPath, &row.wtBranch, &row.createdAt); err != nil {
			return fmt.Errorf("backfill repos: scan: %w", err)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill repos: rows: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("backfill repos: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, row := range pending {
		if _, err := tx.Exec(`
			INSERT INTO task_environment_repos (
				id, task_environment_id, repository_id,
				worktree_id, worktree_path, worktree_branch,
				position, error_message, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 0, '', ?, ?)
		`, uuid.New().String(), row.envID, row.repoID,
			row.wtID, row.wtPath, row.wtBranch,
			row.createdAt, row.createdAt); err != nil {
			return fmt.Errorf("backfill repos: insert env %s: %w", row.envID, err)
		}
	}
	return tx.Commit()
}
