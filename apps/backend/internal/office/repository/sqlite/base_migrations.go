package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
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
	r.migrateRunPayloadIndexes()
	r.migrateCostEventContract()
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
	// Provider routing tables and replayable column migrations. Fresh
	// schemas include the columns inline; ALTERs converge existing databases.
	r.migrateProviderRouting()
	r.migrateContinuationScope()
	r.migrateRunOutcome()
	r.migrateParentWakeIndexes()
	r.migrateParentWakeReceiptColumns()
}

// migrateContinuationScope adds runs.continuation_scope for databases
// created before WO-16's claim-time scope persistence. Existing rows receive
// a scope from their stored context snapshot so queued or claimed taskless
// runs keep their continuation chain after an upgrade.
func (r *Repository) migrateContinuationScope() {
	r.migrate.Apply("runs.continuation_scope",
		`ALTER TABLE runs ADD COLUMN continuation_scope TEXT NOT NULL DEFAULT ''`)
	r.backfillContinuationScopes()
	r.migrate.Apply("runs.failure_scope_status_index",
		`CREATE INDEX IF NOT EXISTS idx_run_failure_scope_status ON runs(agent_profile_id, continuation_scope, status)`)
}

func (r *Repository) backfillContinuationScopes() {
	var legacyRuns []struct {
		ID              string `db:"id"`
		AgentProfileID  string `db:"agent_profile_id"`
		ContextSnapshot string `db:"context_snapshot"`
	}
	if err := r.db.Select(&legacyRuns,
		`SELECT id, agent_profile_id, context_snapshot
		 FROM runs WHERE continuation_scope = ''`); err != nil {
		if r.log != nil {
			r.log.Warn("continuation scope backfill query failed", zap.Error(err))
		}
		return
	}
	for _, legacyRun := range legacyRuns {
		scope := models.ContinuationScopeForRun(
			&models.Run{ContextSnapshot: legacyRun.ContextSnapshot},
			legacyRun.AgentProfileID,
		)
		if _, err := r.db.Exec(r.db.Rebind(`
			UPDATE runs SET continuation_scope = ?
			WHERE id = ? AND continuation_scope = ''
		`), scope, legacyRun.ID); err != nil {
			if r.log != nil {
				r.log.Warn("continuation scope backfill update failed",
					zap.String("run_id", legacyRun.ID), zap.Error(err))
			}
		}
	}
}

// migrateCostEventContract adds the cache read/write split, turn
// attribution, and cost-provenance columns to office_cost_events for
// databases created before this contract (docs/specs/office/requirements/costs.md). Every
// ALTER is nullable with no DEFAULT: a legacy row must read NULL, never 0,
// because a merged tokens_cached_in cannot be decomposed and an unversioned
// "0" would be indistinguishable from "no cache activity". Keep this column
// set byte-identical to the inline CREATE TABLE in createCostTables so a
// fresh database and a migrated one converge.
func (r *Repository) migrateCostEventContract() {
	r.migrate.Apply("office_cost_events.tokens_cached_read",
		`ALTER TABLE office_cost_events ADD COLUMN tokens_cached_read INTEGER`)
	r.migrate.Apply("office_cost_events.tokens_cached_write",
		`ALTER TABLE office_cost_events ADD COLUMN tokens_cached_write INTEGER`)
	r.migrate.Apply("office_cost_events.turn_id",
		`ALTER TABLE office_cost_events ADD COLUMN turn_id TEXT`)
	r.migrate.Apply("office_cost_events.usage_event_id",
		`ALTER TABLE office_cost_events ADD COLUMN usage_event_id TEXT`)
	r.migrate.Apply("office_cost_events.cost_source",
		`ALTER TABLE office_cost_events ADD COLUMN cost_source TEXT`)
	r.migrate.Apply("office_cost_events.rate_input_per_million",
		`ALTER TABLE office_cost_events ADD COLUMN rate_input_per_million INTEGER`)
	r.migrate.Apply("office_cost_events.rate_cached_read_per_million",
		`ALTER TABLE office_cost_events ADD COLUMN rate_cached_read_per_million INTEGER`)
	r.migrate.Apply("office_cost_events.rate_cached_write_per_million",
		`ALTER TABLE office_cost_events ADD COLUMN rate_cached_write_per_million INTEGER`)
	r.migrate.Apply("office_cost_events.rate_output_per_million",
		`ALTER TABLE office_cost_events ADD COLUMN rate_output_per_million INTEGER`)
	r.migrate.Apply("office_cost_events.pricing_catalog_version",
		`ALTER TABLE office_cost_events ADD COLUMN pricing_catalog_version TEXT`)
	r.migrate.Apply("office_cost_events.cost_contract_version",
		`ALTER TABLE office_cost_events ADD COLUMN cost_contract_version INTEGER`)
	r.migrate.Apply("uniq_office_cost_usage_event",
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_office_cost_usage_event
			ON office_cost_events(usage_event_id) WHERE usage_event_id IS NOT NULL`)
}

func (r *Repository) migrateRunPayloadIndexes() {
	r.migrate.Apply(
		"runs.idx_run_payload_comment_id",
		runPayloadCommentIDIndexSQL(r.db.DriverName()),
	)
}

// migrateParentWakeIndexes adds the tasks(parent_id) index
// ListStuckParents' three parent_id-correlated subqueries (has-a-live-
// child, no-non-terminal-child, the child-set-key GROUP_CONCAT) need to
// avoid a full tasks scan on every 30-second reconciler tick. Also
// benefits AreAllChildrenTerminal and GetChildSummaries (blockers.go),
// which query the same shape. Applied via r.migrate (non-fatal) rather
// than createParentChildWakeReceiptsTable's r.db.Exec: tasks belongs to
// a different package's schema, so a plain Exec against it is fatal in
// the minimal single-domain test repos that don't create tasks until
// after NewWithDB returns.
func (r *Repository) migrateParentWakeIndexes() {
	r.migrate.Apply(
		"idx_tasks_parent_id",
		`CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id)`,
	)
}

// migrateParentWakeReceiptColumns adds operation identity for receipts
// created by workflow-engine dispatch. Existing direct-run receipts keep
// their delivered_run_id and receive the empty operation id default.
func (r *Repository) migrateParentWakeReceiptColumns() {
	r.migrate.Apply(
		"parent_child_wake_receipts.delivery_operation_id",
		`ALTER TABLE parent_child_wake_receipts
		 ADD COLUMN delivery_operation_id TEXT NOT NULL DEFAULT ''`,
	)
}

// migrateProviderRouting creates the office_workspace_routing,
// office_run_route_attempts, and office_provider_health tables. The
// routing columns are also added with replayable ALTER statements so
// databases created before a column shipped converge to the fresh schema.
func (r *Repository) migrateProviderRouting() {
	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_workspace_routing (
		workspace_id      TEXT PRIMARY KEY,
		enabled           INTEGER NOT NULL DEFAULT 0,
		default_tier      TEXT    NOT NULL DEFAULT 'balanced',
		provider_order    TEXT    NOT NULL DEFAULT '[]',
		provider_profiles TEXT    NOT NULL DEFAULT '{}',
		tier_per_reason   TEXT    NOT NULL DEFAULT '{}',
		role_tiers        TEXT    NOT NULL DEFAULT '{}',
		updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_run_route_attempts (
		run_id           TEXT NOT NULL,
		seq              INTEGER NOT NULL,
		execution_profile_id TEXT NOT NULL DEFAULT '',
		provider_id      TEXT NOT NULL,
		model            TEXT NOT NULL,
		tier             TEXT NOT NULL,
		tier_source      TEXT NOT NULL DEFAULT '',
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

	r.migrate.Apply("runs.resolved_execution_profile_id",
		`ALTER TABLE runs ADD COLUMN resolved_execution_profile_id TEXT`)
	r.migrate.Apply("office_run_route_attempts.execution_profile_id",
		`ALTER TABLE office_run_route_attempts ADD COLUMN execution_profile_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("office_workspace_routing.role_tiers",
		`ALTER TABLE office_workspace_routing ADD COLUMN role_tiers TEXT NOT NULL DEFAULT '{}'`)
	r.migrate.Apply("office_run_route_attempts.tier_source",
		`ALTER TABLE office_run_route_attempts ADD COLUMN tier_source TEXT NOT NULL DEFAULT ''`)

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
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	_, _ = r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_inbox_dismissals (
		user_id TEXT NOT NULL,
		item_kind TEXT NOT NULL,
		item_id TEXT NOT NULL,
		dismissed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
	r.migrate.Apply("tasks.checkout_at", `ALTER TABLE tasks ADD COLUMN checkout_at TIMESTAMP`)
	r.migrate.Apply("tasks.checkout_run_id", `ALTER TABLE tasks ADD COLUMN checkout_run_id TEXT`)
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
	// Older priority-migration fixtures predate the WIP queue columns. Add
	// them before the recreate SELECT so the migration remains compatible with
	// those schemas as well as with real pre-queue installations.
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN wip_admitted INTEGER NOT NULL DEFAULT 1`)
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN queued_for_step_id TEXT NOT NULL DEFAULT ''`)
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN queued_at TIMESTAMP`)
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN autopilot_enabled INTEGER NOT NULL DEFAULT 0`)
	// Same defensive add for external_id/external_id_settled_at
	// (docs/specs/tasks/system-design/external-id-idempotency.md): real installs already have
	// these from task/repository/sqlite/base.go runMigrations(), but older
	// priority-migration fixtures predate them too.
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN external_id TEXT COLLATE BINARY`)
	_, _ = conn.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN external_id_settled_at TIMESTAMP`)

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
			wip_admitted INTEGER NOT NULL DEFAULT 1,
			queued_for_step_id TEXT NOT NULL DEFAULT '',
			queued_at TIMESTAMP,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			autopilot_enabled INTEGER NOT NULL DEFAULT 0,
			archived_at TIMESTAMP,
			archived_by_cascade_id TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			origin TEXT DEFAULT 'manual',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT,
			checkout_agent_id TEXT,
			checkout_at TIMESTAMP,
			checkout_run_id TEXT,
			external_id TEXT COLLATE BINARY,
			external_id_settled_at TIMESTAMP
		)`,
		// archived_by_cascade_id and external_id/external_id_settled_at are
		// added to the task schema by task/repository/sqlite/base.go
		// runMigrations() via idempotent ALTER ADD COLUMN calls that run
		// BEFORE this office recreate. If the recreate omitted them from the
		// new shape: archived_by_cascade_id missing would 500
		// httpArchiveTask -> HandoffService.ArchiveTaskTree (the CAS update
		// in ArchiveTaskIfActive references it); external_id missing would
		// silently drop task create-idempotency on any install that still
		// needed this priority rebuild. Carry both over so they survive the
		// recreate dance. external_id is not COALESCEd — NULL is its
		// meaningful "no identity" state.
		`INSERT INTO tasks_priority_new (
			id, workspace_id, workflow_id, workflow_step_id, title, description,
			state, priority, position, wip_admitted, queued_for_step_id, queued_at, metadata, is_ephemeral, parent_id, autopilot_enabled,
			archived_at, archived_by_cascade_id, created_at, updated_at,
			origin, project_id,
			labels, identifier,
			checkout_agent_id, checkout_at, checkout_run_id,
			external_id, external_id_settled_at
		) SELECT
			id, COALESCE(workspace_id,''), COALESCE(workflow_id,''),
			COALESCE(workflow_step_id,''), title, COALESCE(description,''),
			COALESCE(state,'TODO'), 'medium', COALESCE(position,0),
			COALESCE(wip_admitted,1), COALESCE(queued_for_step_id,''), queued_at,
			COALESCE(metadata,'{}'), COALESCE(is_ephemeral,0),
			COALESCE(parent_id,''), COALESCE(autopilot_enabled,0), archived_at,
			COALESCE(archived_by_cascade_id,''),
			created_at, updated_at,
			COALESCE(origin,'manual'),
			COALESCE(project_id,''),
			COALESCE(labels,'[]'), identifier,
			checkout_agent_id, checkout_at, checkout_run_id,
			external_id, external_id_settled_at
		FROM tasks`,
		`DROP TABLE tasks`,
		`ALTER TABLE tasks_priority_new RENAME TO tasks`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_id ON tasks(workflow_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workflow_step_id ON tasks(workflow_step_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_archived_at ON tasks(archived_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_archived ON tasks(workspace_id, archived_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id)`,
		// idx_tasks_assignee was removed in ADR 0005 Wave F when the
		// per-task assignee moved to workflow_step_participants.
		//
		// uniq_tasks_external_id enforces task create-idempotency
		// (docs/specs/tasks/system-design/external-id-idempotency.md). DROP TABLE tasks above
		// silently drops every index on the old table, this one included —
		// unlike a plain ALTER TABLE ADD COLUMN, recreating the table means
		// every index must be explicitly relisted here or it is gone from
		// any install that still needs this historical rebuild.
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_tasks_external_id ON tasks(workspace_id, external_id) WHERE external_id IS NOT NULL`,
	}
}
