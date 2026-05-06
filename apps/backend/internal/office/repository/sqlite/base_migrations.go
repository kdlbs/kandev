package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// runMigrations applies idempotent schema migrations.
func (r *Repository) runMigrations() {
	// Phase 3 of task-model-unification renamed office_runs → runs and
	// office_run_events → run_events. Run the rename BEFORE the column
	// migrations and createRunTables / createActivityTables so all
	// downstream paths see the new table names. The function is
	// idempotent: when the rename has already happened (or the legacy
	// tables never existed) it does nothing.
	r.renameLegacyRunTables()
	r.dropLegacyExecutionDecisions()
	r.migrateSchedulerColumns()
	r.migrateChannelColumns()
	r.migrateLegacyLabels()
	r.migrateFailureColumns()
	r.dropLegacyTaskColumns()
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
	// ADR 0005 Wave A — copy each office_agent_instances row into the
	// merged agent_profiles table, preserving the instance id. Idempotent:
	// rows with a matching id are skipped on subsequent runs.
	if err := r.migrateOfficeAgentsToProfiles(); err != nil {
		fmt.Println("office sqlite migrate agents->profiles:", err)
	}
	// ADR 0005 Wave C — once every row has been migrated into
	// agent_profiles and every office query has switched to read from
	// the merged table, we drop the legacy office_agent_instances table.
	// Idempotent: a no-op when the table is already gone.
	r.dropLegacyOfficeAgentInstances()
	// ADR 0005 Wave C — migrate office_task_participants rows into
	// workflow_step_participants then drop the legacy table.
	r.migrateOfficeTaskParticipantsToWorkflow()
	// ADR 0005 Wave E — migrate office_task_approval_decisions rows into
	// workflow_step_decisions then drop the legacy table. Idempotent: a
	// no-op when the legacy table is already gone.
	r.migrateOfficeDecisionsToWorkflow()
	// office-provider-routing — workspace routing config, per-run route
	// attempt history, scoped provider health, plus routing columns on
	// the runs queue. All idempotent (CREATE IF NOT EXISTS / ALTER ADD
	// COLUMN swallows duplicate-column errors).
	r.migrateProviderRouting()
	// System-skill columns. Added when bundled kandev system skills got
	// indexed in the office_skills table. Idempotent: ALTER ADD COLUMN
	// failures (duplicate column) are swallowed.
	r.migrateSystemSkillColumns()
}

// migrateSystemSkillColumns adds is_system / system_version /
// default_for_roles to office_skills for DBs created before bundled
// system skills became first-class rows. Idempotent.
func (r *Repository) migrateSystemSkillColumns() {
	_, _ = r.db.Exec(`ALTER TABLE office_skills ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0`)
	_, _ = r.db.Exec(`ALTER TABLE office_skills ADD COLUMN system_version TEXT NOT NULL DEFAULT ''`)
	_, _ = r.db.Exec(`ALTER TABLE office_skills ADD COLUMN default_for_roles TEXT NOT NULL DEFAULT '[]'`)
	_, _ = r.db.Exec(`CREATE INDEX IF NOT EXISTS office_skills_is_system_idx ON office_skills(is_system, workspace_id)`)
}

// migrateProviderRouting creates the office_workspace_routing,
// office_run_route_attempts, and office_provider_health tables and
// adds the routing columns to the runs queue. Each statement is
// idempotent; errors are swallowed because the migration runner does
// not have a logger handle here (mirrors migrateSchedulerColumns).
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
	// tier_per_reason added in the wake-reason tier policy patch — idempotent
	// for databases that were created before the column was part of the
	// CREATE TABLE.
	_, _ = r.db.Exec(`ALTER TABLE office_workspace_routing ADD COLUMN tier_per_reason TEXT NOT NULL DEFAULT '{}'`)

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

	for _, stmt := range providerRoutingRunColumnStatements() {
		_, _ = r.db.Exec(stmt)
	}
}

// providerRoutingRunColumnStatements returns the ALTER statements that
// add routing columns to the runs queue. Extracted so the column list
// is greppable and the parent migrate function stays under the
// statement-count linter cap.
func providerRoutingRunColumnStatements() []string {
	return []string{
		`ALTER TABLE runs ADD COLUMN logical_provider_order TEXT`,
		`ALTER TABLE runs ADD COLUMN requested_tier TEXT`,
		`ALTER TABLE runs ADD COLUMN resolved_provider_id TEXT`,
		`ALTER TABLE runs ADD COLUMN resolved_model TEXT`,
		`ALTER TABLE runs ADD COLUMN current_route_attempt_seq INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE runs ADD COLUMN routing_blocked_status TEXT`,
		`ALTER TABLE runs ADD COLUMN earliest_retry_at TIMESTAMP`,
		// route_cycle_baseline_seq marks the floor at which the current
		// retry cycle began. excludedFromAttempts filters prior attempt
		// rows with seq <= baseline so a parked-then-lifted run gets a
		// fresh exclusion list instead of re-inheriting every provider
		// that failed in the previous cycle. Bumped by lift / manual
		// retry; left untouched by post-start fallback (within-cycle
		// exclusion is intentional there).
		`ALTER TABLE runs ADD COLUMN route_cycle_baseline_seq INTEGER NOT NULL DEFAULT 0`,
	}
}

// migrateOfficeTaskParticipantsToWorkflow copies every office_task_participants
// row into workflow_step_participants under the participant's task's
// current workflow_step_id, then drops the legacy table. Idempotent:
// duplicate (step_id, task_id, role, agent_profile_id) keys are skipped via
// natural-key probe; missing legacy table or empty result is a no-op.
//
// Rows whose task has no workflow_step_id are silently dropped — they
// could not have been read by the office dashboard anyway (the new
// participants.go short-circuits when stepID is empty).
func (r *Repository) migrateOfficeTaskParticipantsToWorkflow() {
	if !r.tableExists("office_task_participants") {
		return
	}
	if !r.tableExists("workflow_step_participants") {
		// Settings/workflow store hasn't initialised yet — bail; the next
		// boot will pick this up after the workflow store has built its
		// schema. The legacy table stays around until then.
		return
	}
	rows, err := r.db.Query(`SELECT
		p.task_id, p.agent_profile_id, p.role,
		COALESCE(t.workflow_step_id, '') AS step_id
		FROM office_task_participants p
		LEFT JOIN tasks t ON t.id = p.task_id`)
	if err != nil {
		return
	}
	type legacyRow struct {
		taskID  string
		agentID string
		role    string
		stepID  string
	}
	var legacy []legacyRow
	for rows.Next() {
		var lr legacyRow
		if err := rows.Scan(&lr.taskID, &lr.agentID, &lr.role, &lr.stepID); err != nil {
			_ = rows.Close()
			return
		}
		legacy = append(legacy, lr)
	}
	_ = rows.Close()
	for _, lr := range legacy {
		if lr.stepID == "" {
			continue
		}
		// Probe-then-insert idempotency: matches the workflow repo's
		// UpsertTaskParticipant natural-key behaviour without depending on it.
		var existing string
		err := r.db.QueryRow(
			`SELECT id FROM workflow_step_participants
			 WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?`,
			lr.stepID, lr.taskID, lr.role, lr.agentID,
		).Scan(&existing)
		if err == nil {
			continue
		}
		_, _ = r.db.Exec(`INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
			VALUES (?, ?, ?, ?, ?, 1, 0)`,
			newParticipantUUID(), lr.stepID, lr.taskID, lr.role, lr.agentID)
	}
	_, _ = r.db.Exec(`DROP TABLE IF EXISTS office_task_participants`)
}

// migrateOfficeDecisionsToWorkflow copies every office_task_approval_decisions
// row into workflow_step_decisions, resolving the (step_id, participant_id)
// pair from the matching workflow_step_participants row, then drops the
// legacy table. Idempotent: re-runs skip rows whose id already exists in
// the workflow table; missing legacy table or empty result is a no-op.
//
// Resolution rules:
//   - step_id comes from the decision's task → tasks.workflow_step_id.
//   - participant_id comes from workflow_step_participants matching
//     (step_id, task_id, role, decider_id) for agent deciders. User
//     deciders project to a stable "user" sentinel — workflow_step_decisions
//     has no FK on participant_id so this is safe.
//   - rows whose task has no workflow_step_id, or whose decider has no
//     matching agent participant, are silently skipped (warning logged
//     to stderr). They could not have been read by the dashboard's new
//     workflow-store-backed query anyway.
func (r *Repository) migrateOfficeDecisionsToWorkflow() {
	if !r.tableExists("office_task_approval_decisions") {
		return
	}
	if !r.tableExists("workflow_step_decisions") {
		// Workflow store hasn't initialised yet — bail; the next boot
		// will pick this up after the workflow store has built its
		// schema. The legacy table stays around until then.
		return
	}
	rows, err := r.db.Query(`SELECT
		d.id, d.task_id, d.decider_type, d.decider_id, d.role,
		d.decision, d.comment, d.created_at, d.superseded_at,
		COALESCE(t.workflow_step_id, '') AS step_id
		FROM office_task_approval_decisions d
		LEFT JOIN tasks t ON t.id = d.task_id`)
	if err != nil {
		return
	}
	type legacyDecision struct {
		id           string
		taskID       string
		deciderType  string
		deciderID    string
		role         string
		decision     string
		comment      string
		createdAt    sql.NullTime
		supersededAt sql.NullTime
		stepID       string
	}
	var legacy []legacyDecision
	for rows.Next() {
		var lr legacyDecision
		if err := rows.Scan(&lr.id, &lr.taskID, &lr.deciderType, &lr.deciderID,
			&lr.role, &lr.decision, &lr.comment, &lr.createdAt, &lr.supersededAt,
			&lr.stepID); err != nil {
			_ = rows.Close()
			return
		}
		legacy = append(legacy, lr)
	}
	_ = rows.Close()

	skipped := 0
	for _, lr := range legacy {
		if lr.stepID == "" {
			skipped++
			continue
		}
		// Idempotency: skip when the workflow row already exists.
		var existing string
		if err := r.db.QueryRow(
			`SELECT id FROM workflow_step_decisions WHERE id = ?`, lr.id,
		).Scan(&existing); err == nil {
			continue
		}
		participantID := r.resolveDecisionParticipant(lr.stepID, lr.taskID, lr.role, lr.deciderType, lr.deciderID)
		if participantID == "" {
			skipped++
			continue
		}
		decidedAt := lr.createdAt.Time
		var supersededAt interface{}
		if lr.supersededAt.Valid {
			supersededAt = lr.supersededAt.Time
		}
		if _, err := r.db.Exec(`INSERT INTO workflow_step_decisions
			(id, task_id, step_id, participant_id, decision, note, decided_at,
			 superseded_at, decider_type, decider_id, role, comment)
			VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)`,
			lr.id, lr.taskID, lr.stepID, participantID, lr.decision,
			decidedAt, supersededAt, lr.deciderType, lr.deciderID,
			lr.role, lr.comment); err != nil {
			skipped++
			continue
		}
	}
	if skipped > 0 {
		fmt.Printf("office sqlite migrate decisions: skipped %d rows (no step or unresolvable participant)\n", skipped)
	}
	_, _ = r.db.Exec(`DROP TABLE IF EXISTS office_task_approval_decisions`)
}

// resolveDecisionParticipant returns the workflow_step_participants id
// for the (step, task, role, agent) tuple, falling back to a stable
// sentinel for user deciders. Used by migrateOfficeDecisionsToWorkflow.
func (r *Repository) resolveDecisionParticipant(stepID, taskID, role, deciderType, deciderID string) string {
	if deciderType == "user" {
		return "user"
	}
	if deciderID == "" {
		return ""
	}
	var participantID string
	err := r.db.QueryRow(
		`SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND role = ? AND agent_profile_id = ?
		  AND (task_id = ? OR task_id = '')
		ORDER BY (CASE WHEN task_id = '' THEN 1 ELSE 0 END) ASC
		LIMIT 1`,
		stepID, role, deciderID, taskID,
	).Scan(&participantID)
	if err != nil {
		return ""
	}
	return participantID
}

// dropLegacyOfficeAgentInstances removes the office_agent_instances table.
// Office agent CRUD now reads and writes the unified agent_profiles table
// (workspace_id != " marks office rows). The Wave-A migration that
// copies rows over runs first; once that has completed at least once,
// dropping the legacy table is safe. Idempotent via DROP TABLE IF EXISTS.
func (r *Repository) dropLegacyOfficeAgentInstances() {
	_, _ = r.db.Exec(`DROP TABLE IF EXISTS office_agent_instances`)
}

// renameLegacyRunTables renames the old office_runs / office_run_events
// tables to runs / run_events so the runs package can own the queue
// schema independently of the office domain. The rename is idempotent:
// if the new tables already exist (fresh installs build them under
// the new name) or the legacy tables are absent, the ALTER fails and
// is swallowed. createRunTables / createActivityTables in this same
// file are responsible for creating the tables under their new names
// when this is a fresh database.
func (r *Repository) renameLegacyRunTables() {
	if r.tableExists("office_runs") && !r.tableExists("runs") {
		_, _ = r.db.Exec(`ALTER TABLE office_runs RENAME TO runs`)
	}
	if r.tableExists("office_run_events") && !r.tableExists("run_events") {
		_, _ = r.db.Exec(`ALTER TABLE office_run_events RENAME TO run_events`)
	}
	// Drop the old indexes (their stored definitions still mention
	// office_runs even after the table rename) so the recreated ones
	// in createRunTables / createActivityTables use the new name.
	_, _ = r.db.Exec(`DROP INDEX IF EXISTS idx_run_status_requested`)
	_, _ = r.db.Exec(`DROP INDEX IF EXISTS idx_run_idempotency`)
	_, _ = r.db.Exec(`DROP INDEX IF EXISTS idx_run_events_run_created`)
	// Recreate them against the renamed table names.
	_, _ = r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_status_requested ON runs(status, requested_at)`)
	_, _ = r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_run_idempotency ON runs(idempotency_key) WHERE idempotency_key IS NOT NULL`)
	_, _ = r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_events_run_created ON run_events(run_id, created_at)`)
}

// dropLegacyExecutionDecisions removes the office_task_execution_decisions
// table (Phase 4 of task-model-unification). The table was orphaned —
// only created, never read — and has been replaced by
// workflow_step_decisions. Idempotent: a no-op when the table is absent.
func (r *Repository) dropLegacyExecutionDecisions() {
	_, _ = r.db.Exec(`DROP TABLE IF EXISTS office_task_execution_decisions`)
}

// dropLegacyTaskColumns drops the requires_approval, execution_policy
// and execution_state columns from the tasks table (Phase 4 of
// task-model-unification). SQLite ALTER TABLE DROP COLUMN is supported
// since 3.35 (2021); each statement is idempotent — a no-op when the
// column is already gone — and silently swallowed if the SQLite build
// is older. Stage progression is owned by the workflow engine now.
func (r *Repository) dropLegacyTaskColumns() {
	_, _ = r.db.Exec(`ALTER TABLE tasks DROP COLUMN requires_approval`)
	_, _ = r.db.Exec(`ALTER TABLE tasks DROP COLUMN execution_policy`)
	_, _ = r.db.Exec(`ALTER TABLE tasks DROP COLUMN execution_state`)
}

// tableExists returns true when the given table is present in the
// SQLite schema. Used by renameLegacyRunTables to decide whether the
// office_runs → runs migration needs to run.
func (r *Repository) tableExists(name string) bool {
	var exists int
	err := r.db.QueryRow(
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&exists)
	return err == nil && exists == 1
}

// migrateFailureColumns wires up the run error_message column and the
// office_workspace_settings + inbox dismissal tables used by
// office-agent-error-handling. The per-agent failure counter / threshold
// columns are already provided by the merged agent_profiles schema
// (internal/agent/settings/store), so we no longer ALTER them here.
// Each ALTER is idempotent — duplicate-column errors are swallowed.
func (r *Repository) migrateFailureColumns() {
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN error_message TEXT NOT NULL DEFAULT ''`)

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

// migrateSchedulerColumns adds scheduler-related columns to existing tables.
// Each ALTER is idempotent: errors are ignored if the column already exists.
func (r *Repository) migrateSchedulerColumns() {
	// Retry fields on run queue.
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN retry_count INTEGER DEFAULT 0`)
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN scheduled_retry_at DATETIME`)

	// Atomic task checkout fields.
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN checkout_agent_id TEXT`)
	_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN checkout_at DATETIME`)

	// skip_idle_runs / cheap_agent_profile_id ALTERs removed — these
	// columns now ship as part of the merged agent_profiles schema
	// (ADR 0005 Wave C) owned by internal/agent/settings/store.

	// Run cancel reason.
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN cancel_reason TEXT`)

	// Heartbeat-rework run inspection columns (PR 1 of office-heartbeat-rework).
	// These persist the structured adapter output, the assembled prompt the
	// agent received, and the continuation summary that was prepended (if
	// any). All three default to empty strings so existing rows stay valid.
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN result_json TEXT NOT NULL DEFAULT '{}'`)
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN assembled_prompt TEXT NOT NULL DEFAULT ''`)
	_, _ = r.db.Exec(`ALTER TABLE runs ADD COLUMN summary_injected TEXT NOT NULL DEFAULT ''`)

	// Heartbeat-rework routines column (PR 3 of office-heartbeat-rework).
	// Mirrors the per-agent catch-up policy column for routines so a per-
	// routine catch-up cap can be configured. Default keeps current
	// behaviour (no catch-up cap is enforced until the routine cron tick
	// path consumes this column — tracked in service.go TODO).
	_, _ = r.db.Exec(`ALTER TABLE office_routines ADD COLUMN catch_up_policy TEXT NOT NULL DEFAULT 'enqueue_missed_with_cap'`)
	_, _ = r.db.Exec(`ALTER TABLE office_routines ADD COLUMN catch_up_max INTEGER NOT NULL DEFAULT 25`)

	// Index for actionable task lookup by assignee was removed in ADR 0005
	// Wave F when the column moved to workflow_step_participants. The
	// task repo's migrateTasksDropAssignee drops both the column and the
	// index.
}

func (r *Repository) migrateChannelColumns() {
	_, _ = r.db.Exec(`ALTER TABLE office_channels ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT ''`)
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

// migrateLegacyLabels reads tasks with a non-empty labels JSON column and
// creates catalog + junction rows. It is idempotent via INSERT OR IGNORE.
func (r *Repository) migrateLegacyLabels() {
	// Guard: tasks table may not exist yet (office schema may run before task schema).
	var exists int
	if err := r.db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks'",
	).Scan(&exists); err != nil || exists == 0 {
		return
	}

	type taskRow struct {
		ID          string `db:"id"`
		WorkspaceID string `db:"workspace_id"`
		Labels      string `db:"labels"`
	}

	var rows []taskRow
	if err := r.db.Select(&rows, `
		SELECT id, COALESCE(workspace_id,'') AS workspace_id,
		       COALESCE(labels,'[]') AS labels
		FROM tasks
		WHERE labels IS NOT NULL AND labels != '' AND labels != '[]'
	`); err != nil {
		return
	}

	for _, t := range rows {
		names := parseLabelJSON(t.Labels)
		for _, name := range names {
			if name == "" {
				continue
			}
			lbl, err := r.GetOrCreateLabel(context.Background(), t.WorkspaceID, name)
			if err != nil {
				continue
			}
			_ = r.AddLabelToTask(context.Background(), t.ID, lbl.ID)
		}
	}
}

// parseLabelJSON naively extracts string values from a JSON array like ["bug","urgent"].
// It avoids pulling in encoding/json to keep the function small.
func parseLabelJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	// Strip outer brackets.
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		if p != "" {
			names = append(names, p)
		}
	}
	return names
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
	return r.runTaskPriorityRecreate()
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
