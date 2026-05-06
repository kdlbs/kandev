// Package sqlite provides SQLite-based repository for office entities.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// newParticipantUUID is a thin wrapper over uuid.New so the migration
// statements stay readable without import noise. Hoisted to the package
// scope because Go disallows function literals in package-level var
// initialisation when used in compound expressions.
func newParticipantUUID() string { return uuid.New().String() }

// RunnerProjection returns the correlated subquery that resolves the
// effective per-task runner from workflow_step_participants, falling
// back to the step's primary agent_profile_id (ADR 0005 Wave F).
// Inlined into SELECT clauses where the legacy
// tasks.assignee_agent_profile_id column would have been read.
//
// The alias is the table alias of the tasks row (e.g. "t" or "tasks");
// the resulting SQL fragment evaluates to "" when neither a runner row
// nor a step primary exists.
func RunnerProjection(alias string) string {
	if alias == "" {
		alias = "tasks"
	}
	return `COALESCE(
		(SELECT wsp.agent_profile_id FROM workflow_step_participants wsp
		 WHERE wsp.step_id = ` + alias + `.workflow_step_id
		   AND wsp.task_id = ` + alias + `.id
		   AND wsp.role = 'runner'
		 ORDER BY wsp.position ASC, wsp.id ASC LIMIT 1),
		(SELECT ws.agent_profile_id FROM workflow_steps ws WHERE ws.id = ` + alias + `.workflow_step_id),
		''
	)`
}

// Repository provides SQLite-based office storage operations. The
// runs queue methods (CreateRun, ClaimNextEligibleRun, AppendRunEvent,
// etc.) are provided via the embedded *runssqlite.Repository — those
// were lifted out of the office package in Phase 3 of
// task-model-unification but stay accessible here so existing office
// code paths (and the office service's QueueRun delegation) keep
// working without churn.
type Repository struct {
	*runssqlite.Repository

	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewWithDB creates a new office repository with existing database connections.
func NewWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	repo := &Repository{
		Repository: runssqlite.NewWithDB(writer, reader),
		db:         writer,
		ro:         reader,
	}
	if err := repo.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize office schema: %w", err)
	}
	return repo, nil
}

// RunsRepository returns the embedded runs queue repository so
// callers in the runs package can hold a typed handle without
// importing the office repository.
func (r *Repository) RunsRepository() *runssqlite.Repository { return r.Repository }

// CommentRunStatus is the slim per-comment run snapshot returned by
// GetRunsByCommentIDs. The type lives in the runs package now;
// re-exported here so callers (dashboard, etc.) that imported the
// office sqlite package keep compiling without a flag-day rename.
type CommentRunStatus = runssqlite.CommentRunStatus

// RunCostRollup carries per-run aggregated token + cost numbers
// returned by GetRunWithCosts. Re-exported alias — see CommentRunStatus.
type RunCostRollup = runssqlite.RunCostRollup

// ExecRaw executes a raw SQL statement against the writer database.
// Intended for test setup; production code should use typed methods.
func (r *Repository) ExecRaw(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.ExecContext(ctx, r.db.Rebind(query), args...)
}

// ReaderDB returns the read-replica handle. Exposed so the
// internal/office/summary package can issue ad-hoc workspace-scope
// queries (blocked tasks, activity counts) that don't have first-
// class repo methods today. Production code outside the office
// package should keep using typed methods.
func (r *Repository) ReaderDB() *sqlx.DB { return r.ro }

// initSchema creates all office tables if they don't exist.
func (r *Repository) initSchema() error {
	if err := r.createCoreTables(); err != nil {
		return err
	}
	if err := r.createExtensionTables(); err != nil {
		return err
	}
	r.runMigrations()
	return nil
}

// createCoreTables creates the primary office entity tables.
func (r *Repository) createCoreTables() error {
	if err := r.createAgentTables(); err != nil {
		return err
	}
	if err := r.createProjectTables(); err != nil {
		return err
	}
	if err := r.createAgentRuntimeTable(); err != nil {
		return err
	}
	if err := r.createCostTables(); err != nil {
		return err
	}
	if err := r.createRunTables(); err != nil {
		return err
	}
	if err := r.createTreeHoldTables(); err != nil {
		return err
	}
	if err := r.createWorkspaceGroupTables(); err != nil {
		return err
	}
	if err := r.createRoutineTables(); err != nil {
		return err
	}
	if err := r.createApprovalTables(); err != nil {
		return err
	}
	return nil
}

// createExtensionTables creates supplementary office tables.
func (r *Repository) createExtensionTables() error {
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
	if err := r.createOnboardingTable(); err != nil {
		return err
	}
	if err := r.createInstructionTable(); err != nil {
		return err
	}
	if err := r.createLabelTables(); err != nil {
		return err
	}
	if err := r.createWorkspaceGovernanceTable(); err != nil {
		return err
	}
	if err := r.createContinuationSummaryTable(); err != nil {
		return err
	}
	if err := r.createAgentWakeupRequestTable(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) createAgentTables() error {
	// office_agent_instances was removed in ADR 0005 Wave C — office agents
	// are now rows in the unified agent_profiles table (workspace_id != '').
	// The settings store (internal/agent/settings/store) owns the
	// agent_profiles schema; office initSchema runs after settings, so
	// the table is already present when this code runs.
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_skills (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		description TEXT DEFAULT '',
		source_type TEXT NOT NULL DEFAULT 'inline',
		source_locator TEXT DEFAULT '',
		content TEXT DEFAULT '',
		file_inventory TEXT DEFAULT '[]',
		version TEXT NOT NULL DEFAULT '',
		content_hash TEXT NOT NULL DEFAULT '',
		approval_state TEXT NOT NULL DEFAULT 'approved',
		created_by_agent_profile_id TEXT DEFAULT '',
		is_system INTEGER NOT NULL DEFAULT 0,
		system_version TEXT NOT NULL DEFAULT '',
		default_for_roles TEXT NOT NULL DEFAULT '[]',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(workspace_id, slug)
	);
	`)
	return err
}

func (r *Repository) createProjectTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_projects (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		lead_agent_profile_id TEXT DEFAULT '',
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

func (r *Repository) createAgentRuntimeTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_agent_runtime (
		agent_id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'idle',
		pause_reason TEXT DEFAULT '',
		last_run_finished_at DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`)
	return err
}

func (r *Repository) createCostTables() error {
	// cost_subcents stores hundredths of a cent (int64). UI divides by
	// 10000 when rendering dollars. The estimated flag is set when
	// token counts were synthesised (e.g. cumulative-delta inference for
	// codex-acp) rather than reported directly by the agent.
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_cost_events (
		id TEXT PRIMARY KEY,
		session_id TEXT DEFAULT '',
		task_id TEXT DEFAULT '',
		agent_profile_id TEXT DEFAULT '',
		project_id TEXT DEFAULT '',
		model TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		tokens_in INTEGER DEFAULT 0,
		tokens_cached_in INTEGER DEFAULT 0,
		tokens_out INTEGER DEFAULT 0,
		cost_subcents INTEGER NOT NULL DEFAULT 0,
		estimated INTEGER NOT NULL DEFAULT 0,
		occurred_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_office_cost_agent ON office_cost_events(agent_profile_id);
	CREATE INDEX IF NOT EXISTS idx_office_cost_occurred ON office_cost_events(occurred_at DESC);
	CREATE INDEX IF NOT EXISTS idx_office_cost_task ON office_cost_events(task_id);

	CREATE TABLE IF NOT EXISTS office_budget_policies (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		scope_type TEXT NOT NULL,
		scope_id TEXT NOT NULL,
		limit_subcents INTEGER NOT NULL,
		period TEXT NOT NULL,
		alert_threshold_pct INTEGER DEFAULT 80,
		action_on_exceed TEXT DEFAULT 'notify_only',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createRunTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		payload TEXT DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'queued',
		coalesced_count INTEGER DEFAULT 1,
		idempotency_key TEXT,
		context_snapshot TEXT DEFAULT '{}',
		capabilities TEXT NOT NULL DEFAULT '{}',
		input_snapshot TEXT NOT NULL DEFAULT '{}',
		output_summary TEXT NOT NULL DEFAULT '',
		failure_reason TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		retry_count INTEGER DEFAULT 0,
		scheduled_retry_at DATETIME,
		error_message TEXT NOT NULL DEFAULT '',
		cancel_reason TEXT,
		requested_at DATETIME NOT NULL,
		claimed_at DATETIME,
		finished_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_run_status_requested ON runs(status, requested_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_run_idempotency ON runs(idempotency_key) WHERE idempotency_key IS NOT NULL;

	CREATE TABLE IF NOT EXISTS office_run_skills (
		run_id TEXT NOT NULL,
		skill_id TEXT NOT NULL,
		version TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		materialized_path TEXT NOT NULL,
		PRIMARY KEY (run_id, skill_id)
	);
	`)
	return err
}

func (r *Repository) createRoutineTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_routines (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		task_template TEXT NOT NULL DEFAULT '{}',
		assignee_agent_profile_id TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		concurrency_policy TEXT DEFAULT 'skip_if_active',
		catch_up_policy TEXT NOT NULL DEFAULT 'enqueue_missed_with_cap',
		catch_up_max INTEGER NOT NULL DEFAULT 25,
		variables TEXT DEFAULT '{}',
		last_run_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS office_routine_triggers (
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
		FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS office_routine_runs (
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
		FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
	);
	`)
	return err
}

func (r *Repository) createApprovalTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_approvals (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		type TEXT NOT NULL,
		requested_by_agent_profile_id TEXT DEFAULT '',
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
	CREATE TABLE IF NOT EXISTS office_activity_log (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		actor_type TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		action TEXT NOT NULL,
		target_type TEXT DEFAULT '',
		target_id TEXT DEFAULT '',
		details TEXT DEFAULT '{}',
		run_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_activity_workspace_created ON office_activity_log(workspace_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_activity_run_id ON office_activity_log(run_id) WHERE run_id != '';
	CREATE INDEX IF NOT EXISTS idx_activity_session_id ON office_activity_log(session_id) WHERE session_id != '';

	CREATE TABLE IF NOT EXISTS run_events (
		run_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		payload TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		PRIMARY KEY (run_id, seq)
	);
	CREATE INDEX IF NOT EXISTS idx_run_events_run_created ON run_events(run_id, created_at);
	`)
	return err
}

func (r *Repository) createMemoryTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_agent_memory (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		layer TEXT NOT NULL,
		key TEXT NOT NULL,
		content TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(agent_profile_id, layer, key)
	);
	`)
	return err
}

func (r *Repository) createChannelTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_channels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		webhook_secret TEXT NOT NULL DEFAULT '',
		status TEXT DEFAULT 'active',
		task_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`)
	return err
}

func (r *Repository) createOnboardingTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_onboarding (
		workspace_id TEXT PRIMARY KEY,
		completed INTEGER NOT NULL DEFAULT 0,
		ceo_agent_id TEXT DEFAULT '',
		first_task_id TEXT DEFAULT '',
		completed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`)
	return err
}

func (r *Repository) createInstructionTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_agent_instructions (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		is_entry INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(agent_profile_id, filename)
	);
	`)
	return err
}

func (r *Repository) createLabelTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_labels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		color TEXT NOT NULL DEFAULT '#6b7280',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, name)
	);
	CREATE INDEX IF NOT EXISTS idx_office_labels_workspace ON office_labels(workspace_id);

	CREATE TABLE IF NOT EXISTS office_task_labels (
		task_id TEXT NOT NULL,
		label_id TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (task_id, label_id),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (label_id) REFERENCES office_labels(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_office_task_labels_label ON office_task_labels(label_id);
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

	-- office_task_participants was removed in ADR 0005 Wave C. Reviewer
	-- and approver rows are now stored in workflow_step_participants
	-- (dual-scoped on task_id + step_id). The migration that copies any
	-- legacy rows over is in migrateOfficeTaskParticipants below.
	`)
	return err
}

func (r *Repository) createWorkspaceGovernanceTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_workspace_governance (
		workspace_id TEXT NOT NULL,
		key          TEXT NOT NULL,
		value        INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, key)
	);
	`)
	return err
}

// createContinuationSummaryTable creates the agent_continuation_summaries
// table introduced in PR 1 of office-heartbeat-rework. The row stores the
// per-(agent, scope) markdown summary that bridges context across taskless
// runs (heartbeats, lightweight routines). Capped at 8 KB on write.
func (r *Repository) createContinuationSummaryTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS agent_continuation_summaries (
		agent_profile_id  TEXT NOT NULL,
		scope             TEXT NOT NULL,
		content           TEXT NOT NULL DEFAULT '',
		content_tokens    INTEGER NOT NULL DEFAULT 0,
		updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_by_run_id TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (agent_profile_id, scope)
	);
	`)
	return err
}

// createAgentWakeupRequestTable creates the agent_wakeup_requests table
// introduced in PR 2 of office-heartbeat-rework. Each row is one
// "wake the agent" request from cron heartbeats, comments, agent-error
// escalations, routines, user mentions, or agent self-requests. The
// dispatcher coalesces / claims / drops them per the agent's policy
// before creating the corresponding runs row.
//
// idempotency_key carries source-level dedup (e.g. heartbeat:<agent>:
// <unix_minute>) — duplicates within the window land on the partial
// UNIQUE index and are rejected. The "" sentinel keeps the index free
// for rows without a key.
func (r *Repository) createAgentWakeupRequestTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS agent_wakeup_requests (
		id                    TEXT PRIMARY KEY,
		agent_profile_id      TEXT NOT NULL,
		source                TEXT NOT NULL,
		reason                TEXT NOT NULL DEFAULT '',
		payload               TEXT NOT NULL DEFAULT '{}',
		status                TEXT NOT NULL,
		coalesced_count       INTEGER NOT NULL DEFAULT 1,
		idempotency_key       TEXT,
		run_id                TEXT NOT NULL DEFAULT '',
		requested_at          TIMESTAMP NOT NULL,
		claimed_at            TIMESTAMP,
		finished_at           TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_wakeup_agent_status ON agent_wakeup_requests(agent_profile_id, status);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_wakeup_idempotency ON agent_wakeup_requests(idempotency_key)
		WHERE idempotency_key IS NOT NULL AND idempotency_key != '';
	`)
	return err
}
