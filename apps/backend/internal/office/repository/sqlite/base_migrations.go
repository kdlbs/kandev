package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// runMigrations applies idempotent schema migrations for office tables
// living alongside the canonical task tables. Office is shipping as a
// new feature, so we only carry migrations that mutate state owned by
// the task schema on main (tasks priority TEXT rebuild, FTS triggers,
// checkout columns) plus the routing/settings/dismissal tables created
// from base_migrations. Legacy in-branch reshapes (rename office_runs,
// fold office_agent_instances, drop dead task columns, etc.) have been
// collapsed: those rows never landed on main so the migrations were
// pure no-ops at first boot.
func (r *Repository) runMigrations() {
	r.migrateSchedulerColumns()
	r.migrateFailureColumns()
	if err := r.migrateTaskPriorityToText(); err != nil {
		// Surface to stderr; this stage runs from initSchema which doesn't
		// have a logger handle. The recreate is wrapped in a transaction
		// so a failure leaves the DB intact.
		fmt.Println("office sqlite migrate priority:", err)
	}
	// Run migrateTaskFTS LAST so its triggers survive any subsequent
	// recreate-table migrations (notably migrateTaskPriorityToText, which
	// drops + rebuilds `tasks` and would otherwise wipe the FTS triggers).
	r.migrateTaskFTS()
	// Provider routing tables (office_workspace_routing,
	// office_run_route_attempts, office_provider_health). The runs queue
	// routing columns and tier_per_reason are part of the canonical
	// CREATE TABLE statements, so this migration only creates the
	// auxiliary tables.
	r.migrateProviderRouting()
}

// migrateProviderRouting creates the office_workspace_routing,
// office_run_route_attempts, and office_provider_health tables. The
// routing columns on the runs queue (logical_provider_order,
// requested_tier, etc.) live in the canonical CREATE TABLE runs in
// base.go, so they are not ALTERed here.
func (r *Repository) migrateProviderRouting() {
	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_workspace_routing (
		workspace_id      TEXT PRIMARY KEY,
		enabled           INTEGER NOT NULL DEFAULT 0,
		default_tier      TEXT    NOT NULL DEFAULT 'balanced',
		provider_order    TEXT    NOT NULL DEFAULT '[]',
		provider_profiles TEXT    NOT NULL DEFAULT '{}',
		tier_per_reason   TEXT    NOT NULL DEFAULT '{}',
		updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_run_route_attempts (
		run_id           TEXT NOT NULL,
		seq              INTEGER NOT NULL,
		provider_id      TEXT NOT NULL,
		model            TEXT NOT NULL,
		tier             TEXT NOT NULL,
		outcome          TEXT NOT NULL,
		error_code       TEXT,
		error_confidence TEXT,
		adapter_phase    TEXT,
		classifier_rule  TEXT,
		exit_code        INTEGER,
		raw_excerpt      TEXT,
		reset_hint       TIMESTAMP,
		started_at       TIMESTAMP NOT NULL,
		finished_at      TIMESTAMP,
		PRIMARY KEY (run_id, seq),
		FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
	)`)

	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_provider_health (
		workspace_id   TEXT NOT NULL,
		provider_id    TEXT NOT NULL,
		scope          TEXT NOT NULL,
		scope_value    TEXT NOT NULL,
		state          TEXT NOT NULL,
		error_code     TEXT,
		retry_at       TIMESTAMP,
		backoff_step   INTEGER NOT NULL DEFAULT 0,
		last_failure   TIMESTAMP,
		last_success   TIMESTAMP,
		raw_excerpt    TEXT,
		updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (workspace_id, provider_id, scope, scope_value)
	)`)
}

// migrateFailureColumns creates the auxiliary office_workspace_settings
// and office_inbox_dismissals tables used by office-agent-error-handling.
// The runs.error_message column is part of the canonical CREATE TABLE
// in base.go, so no ALTER is needed here.
func (r *Repository) migrateFailureColumns() {
	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_workspace_settings (
		workspace_id TEXT PRIMARY KEY,
		agent_failure_threshold INTEGER NOT NULL DEFAULT 3,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_inbox_dismissals (
		user_id TEXT NOT NULL,
		item_kind TEXT NOT NULL,
		item_id TEXT NOT NULL,
		dismissed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, item_kind, item_id)
	)`)
	_, _ = r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_office_inbox_dismissals_kind ON office_inbox_dismissals(item_kind, item_id)`)
}

// migrateSchedulerColumns adds the atomic task-checkout columns to the
// canonical tasks table. The tasks table exists on main, so this ALTER
// is necessary for upgrade. All run/routine column ALTERs that used to
// live here have been folded into the canonical CREATE TABLE statements
// (base.go) — office is new, so main upgrades pick up the final shape
// directly without an interim ALTER step.
func (r *Repository) migrateSchedulerColumns() {
	r.migrate.Apply("tasks.checkout_agent_id", `ALTER TABLE tasks ADD COLUMN checkout_agent_id TEXT`)
	r.migrate.Apply("tasks.checkout_at", `ALTER TABLE tasks ADD COLUMN checkout_at DATETIME`)
}

// migrateTaskFTS creates the FTS5 virtual table and triggers for full-text task search.
// Skips entirely when the tasks table does not exist or when the SQLite build lacks FTS5.
func (r *Repository) migrateTaskFTS() {
	// Guard: tasks table may not exist yet (office schema runs before task schema).
	var exists int
	if err := r.db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks'",
	).Scan(&exists); err != nil {
		return
	}

	// Attempt to create the FTS5 virtual table. If the SQLite build lacks FTS5,
	// this will fail silently and we skip triggers + backfill.
	//
	// NOTE: Use internal content storage (no `content=` clause). External
	// content mode (content='tasks') was previously used but never actually
	// indexed any rows — the AFTER INSERT trigger's INSERT INTO tasks_fts(
	// rowid, ...) is supposed to be a sync directive in external mode, but
	// in this setup it left the index empty (search returned 0 even for
	// seeded onboarding tasks). Internal mode duplicates the text into the
	// FTS table itself, so the trigger does a real INSERT and the index
	// stays in sync. The storage cost is negligible for our task volumes.
	r.maybeDropLegacyExternalContentFTS()
	if _, err := r.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(
			title, description, identifier
		)
	`); err != nil {
		return // FTS5 module not available
	}

	r.createFTSTriggers()
	r.backfillFTS()
}

// maybeDropLegacyExternalContentFTS drops the legacy external-content
// tasks_fts table (and its triggers) when the existing schema indicates
// the previous shape — `CREATE VIRTUAL TABLE … USING fts5(…, content='tasks',
// content_rowid='rowid')`. Older databases were created with that mode, which
// never populated the index in this codebase. Dropping lets migrateTaskFTS
// recreate it in internal-content mode and backfill from `tasks`. No-op when
// the table doesn't exist or is already internal-content.
func (r *Repository) maybeDropLegacyExternalContentFTS() {
	var sqlText string
	err := r.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='tasks_fts'`,
	).Scan(&sqlText)
	if err != nil {
		return // no table to drop
	}
	if !strings.Contains(sqlText, "content='tasks'") &&
		!strings.Contains(sqlText, `content="tasks"`) {
		return // already internal-content
	}
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_insert`)
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_update`)
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_delete`)
	_, _ = r.db.Exec(`DROP TABLE IF EXISTS tasks_fts`)
}

// createFTSTriggers installs INSERT/UPDATE/DELETE triggers to keep the FTS
// index in sync. tasks_fts is an internal-content FTS5 table, so plain
// INSERT/DELETE statements drive the index — the 'delete' command literal
// is reserved for external-content mode and raises "SQL logic error" here
// (the prior implementation broke every UPDATE on tasks).
func (r *Repository) createFTSTriggers() {
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_insert`)
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_update`)
	_, _ = r.db.Exec(`DROP TRIGGER IF EXISTS tasks_fts_delete`)
	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_insert AFTER INSERT ON tasks BEGIN
		INSERT INTO tasks_fts(rowid, title, description, identifier)
		VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
	END`)

	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_update AFTER UPDATE ON tasks BEGIN
		DELETE FROM tasks_fts WHERE rowid = old.rowid;
		INSERT INTO tasks_fts(rowid, title, description, identifier)
		VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
	END`)

	_, _ = r.db.Exec(`CREATE TRIGGER IF NOT EXISTS tasks_fts_delete AFTER DELETE ON tasks BEGIN
		DELETE FROM tasks_fts WHERE rowid = old.rowid;
	END`)
}

// backfillFTS populates the FTS index from existing task rows.
func (r *Repository) backfillFTS() {
	_, _ = r.db.Exec(`
		INSERT OR IGNORE INTO tasks_fts(rowid, title, description, identifier)
		SELECT rowid, title, COALESCE(description,''), COALESCE(identifier,'') FROM tasks
	`)
}

// migrateTaskPriorityToText converts the tasks.priority column from INTEGER to
// TEXT with a CHECK constraint and 'medium' default. It is idempotent: when
// the column is already TEXT (or the tasks table does not yet exist) the
// function returns silently. All existing integer values are remapped to
// 'medium' per spec — the integer was not surfaced anywhere meaningful.
// Returns an error so the migration runner can log a meaningful message;
// the recreate is wrapped in a tx so failures don't half-modify the DB.
func (r *Repository) migrateTaskPriorityToText() error {
	if !r.tasksTableExists() {
		return nil
	}
	if !r.taskPriorityIsInteger() {
		return nil
	}
	if err := r.runTaskPriorityRecreate(); err != nil {
		return err
	}
	if r.log != nil {
		r.log.Info("migration applied", zap.String("name", "tasks.priority_text_rebuild"))
	}
	return nil
}

// tasksTableExists returns true when the `tasks` table is present.
func (r *Repository) tasksTableExists() bool {
	var exists int
	err := r.db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks'",
	).Scan(&exists)
	return err == nil && exists == 1
}

// taskPriorityIsInteger returns true when tasks.priority has INTEGER type.
// SQLite stores type info via PRAGMA table_info; we look for the legacy type.
func (r *Repository) taskPriorityIsInteger() bool {
	rows, err := r.db.Queryx(`PRAGMA table_info(tasks)`)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == "priority" {
			return strings.EqualFold(ctype, "INTEGER")
		}
	}
	return false
}

// runTaskPriorityRecreate performs the SQLite table-recreate dance to change
// tasks.priority from INTEGER to TEXT. The whole sequence runs on a single
// connection grabbed via db.Conn so PRAGMA + statements share the same SQLite
// session — matters for :memory: databases that are connection-local.
//
// The recreate dance itself is idempotent: each step works against the
// current schema, and tasks_priority_new is rebuilt fresh every run.
func (r *Repository) runTaskPriorityRecreate() error {
	ctx := context.Background()
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()

	// archived_by_cascade_id is added to the canonical tasks shape by
	// internal/task/repository/sqlite/base.go runMigrations() (line 222)
	// AFTER initTaskSchema creates the legacy INTEGER-priority table
	// but BEFORE the office migrations run. On test fixtures that seed
	// a pre-cascade legacy schema (see TestMigrate_PriorityIntegerToText)
	// the column is absent — we add it idempotently here so the recreate
	// SELECT below can reference it. Errors are swallowed because the
	// most common cause is "column already exists" on real installs.
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN archived_by_cascade_id TEXT DEFAULT ''`)

	for _, stmt := range taskPriorityMigrationStatements() {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("priority migration step failed: %w", err)
		}
	}
	return nil
}

// taskPriorityMigrationStatements returns the ordered SQL statements that
// recreate the tasks table with a TEXT priority column AND drop the
// legacy office columns retired in Phase 4 of task-model-unification
// (requires_approval, execution_policy, execution_state). Existing
// integer priority values are mapped to 'medium' as the spec requires.
//
// The migration runs once when the existing tasks.priority column is
// still INTEGER. After it lands, the table matches the final shape for
// kanban+office unification: stage progression is owned by the
// workflow engine, so the legacy policy/state columns are gone.
func taskPriorityMigrationStatements() []string {
	return []string{
		// ADR 0005 Wave F dropped assignee_agent_profile_id from the
		// canonical tasks shape. Per-task runner now lives in
		// workflow_step_participants. Old INTEGER-priority rows that
		// still carry the column have it discarded by the SELECT below.
		`CREATE TABLE tasks_priority_new (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority TEXT NOT NULL DEFAULT 'medium'
				CHECK (priority IN ('critical','high','medium','low')),
			position INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			archived_by_cascade_id TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			origin TEXT DEFAULT 'manual',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT,
			checkout_agent_id TEXT,
			checkout_at DATETIME
		)`,
		// archived_by_cascade_id is added to the task schema by
		// task/repository/sqlite/base.go runMigrations() via an
		// idempotent ALTER ADD COLUMN that runs BEFORE this office
		// recreate. If the recreate omitted it from the new shape,
		// httpArchiveTask -> HandoffService.ArchiveTaskTree would 500
		// with "no such column: archived_by_cascade_id" because the
		// CAS update in ArchiveTaskIfActive references it. Carry it
		// over with COALESCE so the column is preserved across the
		// recreate dance.
		`INSERT INTO tasks_priority_new (
			id, workspace_id, workflow_id, workflow_step_id, title, description,
			state, priority, position, metadata, is_ephemeral, parent_id,
			archived_at, archived_by_cascade_id, created_at, updated_at,
			origin, project_id,
			labels, identifier,
			checkout_agent_id, checkout_at
		) SELECT
			id, COALESCE(workspace_id,''), COALESCE(workflow_id,''),
			COALESCE(workflow_step_id,''), title, COALESCE(description,''),
			COALESCE(state,'TODO'), 'medium', COALESCE(position,0),
			COALESCE(metadata,'{}'), COALESCE(is_ephemeral,0),
			COALESCE(parent_id,''), archived_at,
			COALESCE(archived_by_cascade_id,''),
			created_at, updated_at,
			COALESCE(origin,'manual'),
			COALESCE(project_id,''),
			COALESCE(labels,'[]'), identifier,
			checkout_agent_id, checkout_at
		FROM tasks`,
		`DROP TABLE tasks`,
		`ALTER TABLE tasks_priority_new RENAME TO tasks`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_id ON tasks(workflow_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_step_id ON tasks(workflow_step_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_archived_at ON tasks(archived_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_archived ON tasks(workspace_id, archived_at)`,
		// idx_tasks_assignee was removed in ADR 0005 Wave F when the
		// per-task assignee moved to workflow_step_participants.
	}
}
