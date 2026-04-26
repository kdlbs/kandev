// Package sqlite provides SQLite-based repository for orchestrate entities.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Repository provides SQLite-based orchestrate storage operations.
type Repository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewWithDB creates a new orchestrate repository with existing database connections.
func NewWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	repo := &Repository{db: writer, ro: reader}
	if err := repo.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize orchestrate schema: %w", err)
	}
	return repo, nil
}

// ExecRaw executes a raw SQL statement against the writer database.
// Intended for test setup; production code should use typed methods.
func (r *Repository) ExecRaw(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.ExecContext(ctx, r.db.Rebind(query), args...)
}

// initSchema creates all orchestrate tables if they don't exist.
func (r *Repository) initSchema() error {
	if err := r.createAgentTables(); err != nil {
		return err
	}
	if err := r.createProjectTables(); err != nil {
		return err
	}
	if err := r.createCostTables(); err != nil {
		return err
	}
	if err := r.createWakeupTables(); err != nil {
		return err
	}
	if err := r.createRoutineTables(); err != nil {
		return err
	}
	if err := r.createApprovalTables(); err != nil {
		return err
	}
	if err := r.createActivityTables(); err != nil {
		return err
	}
	if err := r.createMemoryTables(); err != nil {
		return err
	}
	if err := r.createChannelTables(); err != nil {
		return err
	}
	if err := r.createTaskExtensionTables(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) createAgentTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_agent_instances (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_profile_id TEXT DEFAULT '',
		role TEXT NOT NULL DEFAULT 'worker',
		icon TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle',
		reports_to TEXT DEFAULT '',
		permissions TEXT DEFAULT '{}',
		budget_monthly_cents INTEGER DEFAULT 0,
		max_concurrent_sessions INTEGER DEFAULT 1,
		desired_skills TEXT DEFAULT '[]',
		executor_preference TEXT DEFAULT '{}',
		pause_reason TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(workspace_id, name)
	);

	CREATE TABLE IF NOT EXISTS orchestrate_skills (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		description TEXT DEFAULT '',
		source_type TEXT NOT NULL DEFAULT 'inline',
		source_locator TEXT DEFAULT '',
		content TEXT DEFAULT '',
		file_inventory TEXT DEFAULT '[]',
		created_by_agent_instance_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(workspace_id, slug)
	);
	`)
	return err
}

func (r *Repository) createProjectTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_projects (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		lead_agent_instance_id TEXT DEFAULT '',
		color TEXT DEFAULT '',
		budget_cents INTEGER DEFAULT 0,
		repositories TEXT DEFAULT '[]',
		executor_config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createCostTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_cost_events (
		id TEXT PRIMARY KEY,
		session_id TEXT DEFAULT '',
		task_id TEXT DEFAULT '',
		agent_instance_id TEXT DEFAULT '',
		project_id TEXT DEFAULT '',
		model TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		tokens_in INTEGER DEFAULT 0,
		tokens_cached_in INTEGER DEFAULT 0,
		tokens_out INTEGER DEFAULT 0,
		cost_cents INTEGER DEFAULT 0,
		occurred_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS orchestrate_budget_policies (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		scope_type TEXT NOT NULL,
		scope_id TEXT NOT NULL,
		limit_cents INTEGER NOT NULL,
		period TEXT NOT NULL,
		alert_threshold_pct INTEGER DEFAULT 80,
		action_on_exceed TEXT DEFAULT 'notify_only',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createWakeupTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_wakeup_queue (
		id TEXT PRIMARY KEY,
		agent_instance_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		payload TEXT DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'queued',
		coalesced_count INTEGER DEFAULT 1,
		idempotency_key TEXT,
		context_snapshot TEXT DEFAULT '{}',
		requested_at DATETIME NOT NULL,
		claimed_at DATETIME,
		finished_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_wakeup_status_requested ON orchestrate_wakeup_queue(status, requested_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_wakeup_idempotency ON orchestrate_wakeup_queue(idempotency_key) WHERE idempotency_key IS NOT NULL;
	`)
	return err
}

func (r *Repository) createRoutineTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_routines (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		task_template TEXT NOT NULL DEFAULT '{}',
		assignee_agent_instance_id TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		concurrency_policy TEXT DEFAULT 'skip_if_active',
		variables TEXT DEFAULT '{}',
		last_run_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS orchestrate_routine_triggers (
		id TEXT PRIMARY KEY,
		routine_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		cron_expression TEXT DEFAULT '',
		timezone TEXT DEFAULT '',
		public_id TEXT DEFAULT '',
		signing_mode TEXT DEFAULT '',
		secret TEXT DEFAULT '',
		next_run_at DATETIME,
		last_fired_at DATETIME,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (routine_id) REFERENCES orchestrate_routines(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS orchestrate_routine_runs (
		id TEXT PRIMARY KEY,
		routine_id TEXT NOT NULL,
		trigger_id TEXT DEFAULT '',
		source TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'received',
		trigger_payload TEXT DEFAULT '{}',
		linked_task_id TEXT DEFAULT '',
		coalesced_into_run_id TEXT DEFAULT '',
		dispatch_fingerprint TEXT DEFAULT '',
		started_at DATETIME,
		completed_at DATETIME,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (routine_id) REFERENCES orchestrate_routines(id) ON DELETE CASCADE
	);
	`)
	return err
}

func (r *Repository) createApprovalTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_approvals (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		type TEXT NOT NULL,
		requested_by_agent_instance_id TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		payload TEXT DEFAULT '{}',
		decision_note TEXT DEFAULT '',
		decided_by TEXT DEFAULT '',
		decided_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createActivityTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_activity_log (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		actor_type TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		action TEXT NOT NULL,
		target_type TEXT DEFAULT '',
		target_id TEXT DEFAULT '',
		details TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_activity_workspace_created ON orchestrate_activity_log(workspace_id, created_at DESC);
	`)
	return err
}

func (r *Repository) createMemoryTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_agent_memory (
		id TEXT PRIMARY KEY,
		agent_instance_id TEXT NOT NULL,
		layer TEXT NOT NULL,
		key TEXT NOT NULL,
		content TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(agent_instance_id, layer, key)
	);
	`)
	return err
}

func (r *Repository) createChannelTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_channels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		agent_instance_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		status TEXT DEFAULT 'active',
		task_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createTaskExtensionTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_blockers (
		task_id TEXT NOT NULL,
		blocker_task_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (task_id, blocker_task_id),
		CHECK (task_id != blocker_task_id)
	);

	CREATE TABLE IF NOT EXISTS task_comments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		author_type TEXT NOT NULL,
		author_id TEXT NOT NULL,
		body TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'user',
		reply_channel_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_task_comments_task_created ON task_comments(task_id, created_at);
	`)
	return err
}
