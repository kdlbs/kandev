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
	if err := r.createAgentRuntimeTable(); err != nil {
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
	r.migrateSchedulerColumns()
	r.migrateTaskFTS()
	return nil
}

// migrateSchedulerColumns adds scheduler-related columns to existing tables.
// Each ALTER is idempotent: errors are ignored if the column already exists.
func (r *Repository) migrateSchedulerColumns() {
	// Retry fields on wakeup queue.
	_, _ = r.db.Exec(`ALTER TABLE orchestrate_wakeup_queue ADD COLUMN retry_count INTEGER DEFAULT 0`)
	_, _ = r.db.Exec(`ALTER TABLE orchestrate_wakeup_queue ADD COLUMN scheduled_retry_at DATETIME`)

	// Atomic task checkout fields.
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN checkout_agent_id TEXT`)
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN checkout_at DATETIME`)
}

// migrateTaskFTS creates the FTS5 virtual table and triggers for full-text task search.
// Skips entirely when the tasks table does not exist or when the SQLite build lacks FTS5.
func (r *Repository) migrateTaskFTS() {
	// Guard: tasks table may not exist yet (orchestrate schema runs before task schema).
	var exists int
	if err := r.db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks'",
	).Scan(&exists); err != nil {
		return
	}

	// Attempt to create the FTS5 virtual table. If the SQLite build lacks FTS5,
	// this will fail silently and we skip triggers + backfill.
	if _, err := r.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(
			title, description, identifier,
			content='tasks',
			content_rowid='rowid'
		)
	`); err != nil {
		return // FTS5 module not available
	}

	r.createFTSTriggers()
	r.backfillFTS()
}

// createFTSTriggers installs INSERT/UPDATE/DELETE triggers to keep the FTS index in sync.
func (r *Repository) createFTSTriggers() {
	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_insert AFTER INSERT ON tasks BEGIN
		INSERT INTO tasks_fts(rowid, title, description, identifier)
		VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
	END`)

	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_update AFTER UPDATE ON tasks BEGIN
		INSERT INTO tasks_fts(tasks_fts, rowid, title, description, identifier)
		VALUES('delete', old.rowid, old.title, COALESCE(old.description,''), COALESCE(old.identifier,''));
		INSERT INTO tasks_fts(rowid, title, description, identifier)
		VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
	END`)

	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_delete AFTER DELETE ON tasks BEGIN
		INSERT INTO tasks_fts(tasks_fts, rowid, title, description, identifier)
		VALUES('delete', old.rowid, old.title, COALESCE(old.description,''), COALESCE(old.identifier,''));
	END`)
}

// backfillFTS populates the FTS index from existing task rows.
func (r *Repository) backfillFTS() {
	_, _ = r.db.Exec(`
		INSERT OR IGNORE INTO tasks_fts(rowid, title, description, identifier)
		SELECT rowid, title, COALESCE(description,''), COALESCE(identifier,'') FROM tasks
	`)
}

func (r *Repository) createAgentRuntimeTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS orchestrate_agent_runtime (
		agent_id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'idle',
		pause_reason TEXT DEFAULT '',
		last_wakeup_finished_at DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
		retry_count INTEGER DEFAULT 0,
		scheduled_retry_at DATETIME,
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
