// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
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
	// BIGINT, not INTEGER: office_cost_events.tokens_cached_in routinely
	// accumulates well past int4's 2,147,483,647 ceiling over a long-running
	// session (the reported bug measured up to 98,805,109 on one already-
	// completed task). SQLite's INTEGER is 64-bit regardless, but on Postgres
	// INTEGER is int4 - an overflowing session would abort the single
	// multi-column UPDATE in IncrementTaskSessionUsage, silently taking
	// tokens_in/tokens_out/cost_subcents down with it for that session.
	r.migrate.Apply("task_sessions.tokens_cached_in", `ALTER TABLE task_sessions ADD COLUMN tokens_cached_in BIGINT NOT NULL DEFAULT 0`)
}

// runMigrations applies idempotent ALTER TABLE migrations for schema evolution.
func (r *Repository) runMigrations() error {
	if err := r.migrateTaskPriorityToTextPostgres(); err != nil {
		return err
	}
	if err := r.ensureTaskWorkspaceFoldersSchema(); err != nil {
		return err
	}
	if err := r.ensureRepositorySetsSchema(); err != nil {
		return err
	}
	if err := r.ensureRepositoryBranchPoliciesSchema(); err != nil {
		return err
	}
	r.migrate.Apply("task_sessions.execution_profile_id", `ALTER TABLE task_sessions ADD COLUMN execution_profile_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_sessions.route_generation", `ALTER TABLE task_sessions ADD COLUMN route_generation INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_sessions.route_state", `ALTER TABLE task_sessions ADD COLUMN route_state TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_sessions.route_reason", `ALTER TABLE task_sessions ADD COLUMN route_reason TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_sessions.downstream_acp_session_id", `ALTER TABLE task_sessions ADD COLUMN downstream_acp_session_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("dynamic_route_states.continuation_json", `ALTER TABLE dynamic_route_states ADD COLUMN continuation_json TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("dynamic_route_states.policy_state_json", `ALTER TABLE dynamic_route_states ADD COLUMN policy_state_json TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("executors_running.execution_profile_id", `ALTER TABLE executors_running ADD COLUMN execution_profile_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("executors_running.last_message_uuid", `ALTER TABLE executors_running ADD COLUMN last_message_uuid TEXT DEFAULT ''`)
	r.migrate.Apply("executors_running.metadata", `ALTER TABLE executors_running ADD COLUMN metadata TEXT DEFAULT '{}'`)
	// local_pid holds a host-local liveness handle (the standalone agentctl
	// control-server PID Kandev spawns) for local/standalone rows. It is kept
	// deliberately separate from the SSH-only `pid` column, which holds an
	// agentctl PID on the *remote* host. See ADR 0025 / issue #1597.
	r.migrate.Apply("executors_running.local_pid", `ALTER TABLE executors_running ADD COLUMN local_pid INTEGER DEFAULT 0`)
	r.migrate.Apply("tasks.is_ephemeral", `ALTER TABLE tasks ADD COLUMN is_ephemeral INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_repositories.checkout_branch", `ALTER TABLE task_repositories ADD COLUMN checkout_branch TEXT DEFAULT ''`)
	r.migrate.Apply("task_repositories.branch_policy_id", `ALTER TABLE task_repositories ADD COLUMN branch_policy_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_repositories.branch_policy_name", `ALTER TABLE task_repositories ADD COLUMN branch_policy_name TEXT DEFAULT ''`)
	r.migrate.Apply("task_repositories.branch_policy_base_branch", `ALTER TABLE task_repositories ADD COLUMN branch_policy_base_branch TEXT DEFAULT ''`)
	r.migrate.Apply("task_repositories.branch_policy_branch_template", `ALTER TABLE task_repositories ADD COLUMN branch_policy_branch_template TEXT DEFAULT ''`)
	r.migrate.Apply("task_repositories.branch_policy_pull_request_target", `ALTER TABLE task_repositories ADD COLUMN branch_policy_pull_request_target TEXT DEFAULT ''`)
	// Multi-branch support: drop the old UNIQUE(task_id, repository_id) and
	// replace it with UNIQUE(task_id, repository_id, checkout_branch) so the
	// same repo can appear multiple times in a task on different branches.
	if err := r.migrateTaskRepositoriesAllowMultiBranch(); err != nil {
		return err
	}
	r.migrate.Apply("tasks.wip_admitted", `ALTER TABLE tasks ADD COLUMN wip_admitted INTEGER NOT NULL DEFAULT 1`)
	r.migrate.Apply("tasks.queued_for_step_id", `ALTER TABLE tasks ADD COLUMN queued_for_step_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("tasks.queued_at", `ALTER TABLE tasks ADD COLUMN queued_at TIMESTAMP`)
	r.migrate.Apply("task_sessions.base_commit_sha", `ALTER TABLE task_sessions ADD COLUMN base_commit_sha TEXT DEFAULT ''`)
	r.migrate.Apply("workspaces.default_config_agent_profile_id", `ALTER TABLE workspaces ADD COLUMN default_config_agent_profile_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_sessions.task_environment_id", `ALTER TABLE task_sessions ADD COLUMN task_environment_id TEXT DEFAULT ''`)
	r.migrate.Apply("tasks.parent_id", `ALTER TABLE tasks ADD COLUMN parent_id TEXT DEFAULT ''`)
	r.migrate.Apply("tasks.autopilot_enabled", `ALTER TABLE tasks ADD COLUMN autopilot_enabled INTEGER NOT NULL DEFAULT 0`)
	// Remove FK constraint on workflow_id to allow ephemeral tasks without workflows
	if err := r.migrateTasksRemoveWorkflowFK(); err != nil {
		return err
	}
	if err := r.dropRetiredSlackIntegration(); err != nil {
		return err
	}
	r.migrate.Apply("idx_tasks_queued_for_step", `CREATE INDEX IF NOT EXISTS idx_tasks_queued_for_step ON tasks(queued_for_step_id, queued_at)`)
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
	r.migrate.Apply("task_environments.materialization_session_id", `ALTER TABLE task_environments ADD COLUMN materialization_session_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_environments.container_bootstrap_nonce_secret_id", `ALTER TABLE task_environments ADD COLUMN container_bootstrap_nonce_secret_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_environments.container_control_auth_token_secret_id", `ALTER TABLE task_environments ADD COLUMN container_control_auth_token_secret_id TEXT DEFAULT ''`)
	if err := r.migrateTaskEnvironmentsRemoveAgentExecutionID(); err != nil {
		return err
	}
	if err := r.migrateTaskEnvironmentReposAllowMultiBranch(); err != nil {
		return err
	}
	r.migrate.Apply("workflows.sort_order", `ALTER TABLE workflows ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("workflows.agent_profile_id", `ALTER TABLE workflows ADD COLUMN agent_profile_id TEXT DEFAULT ''`)
	r.migrate.Apply("workflows.hidden", `ALTER TABLE workflows ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_sessions.workspace_path", `ALTER TABLE task_sessions ADD COLUMN workspace_path TEXT DEFAULT ''`)
	// User-supplied session tab label. Must run after the
	// migrateSessionsRemoveAgentExecutionID rebuild above — that rebuild
	// recreates task_sessions from an explicit column list and would drop a
	// column added earlier.
	r.migrate.Apply("task_sessions.name", `ALTER TABLE task_sessions ADD COLUMN name TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.copy_files", `ALTER TABLE repositories ADD COLUMN copy_files TEXT DEFAULT ''`)
	r.migrate.Apply("repository_secret_bindings.table", `
		CREATE TABLE IF NOT EXISTS repository_secret_bindings (
			repository_id TEXT NOT NULL,
			key TEXT NOT NULL,
			secret_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY (repository_id, key),
			FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
		)`)
	r.migrate.Apply("repository_secret_bindings.index", `
		CREATE INDEX IF NOT EXISTS idx_repository_secret_bindings_repository
		ON repository_secret_bindings(repository_id)`)
	r.migrate.Apply("repositories.remote_url", `ALTER TABLE repositories ADD COLUMN remote_url TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.provider_host", `ALTER TABLE repositories ADD COLUMN provider_host TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.provider_scope", `ALTER TABLE repositories ADD COLUMN provider_scope TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.provider_host.github_backfill", `
		UPDATE repositories
		SET provider_host = 'https://github.com'
		WHERE LOWER(TRIM(provider)) = 'github'
			AND TRIM(COALESCE(provider_host, '')) = ''`)
	r.migrate.Apply("repositories.worktree_branch_template", `ALTER TABLE repositories ADD COLUMN worktree_branch_template TEXT DEFAULT ''`)
	r.migrate.Apply("repositories.worktree_branch_template.backfill", `
		UPDATE repositories
		SET worktree_branch_template = COALESCE(NULLIF(TRIM(worktree_branch_prefix), ''), 'feature/') || '{title}-{suffix}'
		WHERE TRIM(COALESCE(worktree_branch_template, '')) = ''`)
	r.migrate.Apply("task_plans.implementation_started_at", `ALTER TABLE task_plans ADD COLUMN implementation_started_at TIMESTAMP`)
	r.migrate.Apply("task_plans.implementation_started_session_id", `ALTER TABLE task_plans ADD COLUMN implementation_started_session_id TEXT`)
	r.migrate.Apply("task_plans.implementation_started_by", `ALTER TABLE task_plans ADD COLUMN implementation_started_by TEXT`)

	// Authoritative per-message change signal (chat render-perf). SQLite forbids a
	// non-constant default on ADD COLUMN, so the column is added nullable and
	// existing rows are backfilled to created_at; new inserts/updates set it
	// explicitly in CreateMessage/UpdateMessage. The backfill UPDATE is idempotent
	// (WHERE updated_at IS NULL).
	r.migrate.Apply("task_session_messages.updated_at", `ALTER TABLE task_session_messages ADD COLUMN updated_at TIMESTAMP`)
	r.migrate.Apply("task_session_messages.updated_at.backfill", `UPDATE task_session_messages SET updated_at = created_at WHERE updated_at IS NULL`)
	r.migrate.Apply("idx_messages_session_updated", `CREATE INDEX IF NOT EXISTS idx_messages_session_updated ON task_session_messages(task_session_id, updated_at)`)

	// task_session_commits gains a uniqueness constraint before its writer
	// starts firing from more than just archive capture (CreateSessionCommit
	// was previously a plain INSERT). Must dedupe existing duplicates first:
	// CREATE UNIQUE INDEX fails on a duplicate pair, and MigrateLogger.Apply
	// swallows non-"already exists" errors, so an unhandled duplicate would
	// silently leave both the index and every future ON CONFLICT missing.
	if err := r.migrateSessionCommitsDedupeAndActivation(); err != nil {
		return err
	}

	// Backfill the per-session cost/token columns. Runs after the gated
	// task_sessions rebuilds above so it repairs legacy DBs whose schema can no
	// longer trigger a rebuild (see migrateSessionsAddCostColumns).
	r.migrateSessionsAddCostColumns()
	// AC-28: widen the three still-INTEGER rollup columns to BIGINT (Postgres
	// only). Must run after migrateSessionsAddCostColumns so a legacy DB has
	// the columns to widen before this ALTERs their type.
	r.migrateTaskSessionsRollupColumnsToBigint()

	// Office task extensions - net-new columns on existing main tables.
	// Idempotent ALTERs; main upgrades pick them up at first boot.
	// The transient in-branch columns (requires_approval,
	// execution_policy, execution_state, assignee_agent_profile_id,
	// task_sessions.agent_instance_id) were never on main and are
	// therefore not added or dropped here.
	r.migrate.Apply("tasks.origin", `ALTER TABLE tasks ADD COLUMN origin TEXT DEFAULT 'manual'`)
	r.migrate.Apply("tasks.project_id", `ALTER TABLE tasks ADD COLUMN project_id TEXT DEFAULT ''`)
	r.migrate.Apply("idx_tasks_project_id", `CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id)`)
	r.migrate.Apply("tasks.labels", `ALTER TABLE tasks ADD COLUMN labels TEXT DEFAULT '[]'`)
	r.migrate.Apply("tasks.identifier", `ALTER TABLE tasks ADD COLUMN identifier TEXT`)
	// Office task-handoffs phase 6 - tag tasks archived as part of a cascade so
	// unarchive can restore exactly the descendants that cascade archived.
	r.migrate.Apply("tasks.archived_by_cascade_id", `ALTER TABLE tasks ADD COLUMN archived_by_cascade_id TEXT DEFAULT ''`)

	// Task create-idempotency (docs/specs/tasks/system-design/external-id-idempotency.md).
	// external_id needs an explicit deterministic collation: SQLite TEXT
	// columns already compare BINARY by default, but an unqualified Postgres
	// column silently inherits the database's default collation, which may be
	// case-insensitive or nondeterministic. The partial unique index syntax
	// (CREATE UNIQUE INDEX ... WHERE ...) is supported identically by both
	// dialects, so it needs no branch.
	if dialect.IsPostgres(r.db.DriverName()) {
		r.migrate.Apply("tasks.external_id", `ALTER TABLE tasks ADD COLUMN external_id TEXT COLLATE "C"`)
	} else {
		r.migrate.Apply("tasks.external_id", `ALTER TABLE tasks ADD COLUMN external_id TEXT COLLATE BINARY`)
	}
	r.migrate.Apply("tasks.external_id_settled_at", `ALTER TABLE tasks ADD COLUMN external_id_settled_at TIMESTAMP`)
	r.migrate.Apply("uniq_tasks_external_id", `CREATE UNIQUE INDEX IF NOT EXISTS uniq_tasks_external_id ON tasks(workspace_id, external_id) WHERE external_id IS NOT NULL`)

	// Office workspace extensions
	r.migrate.Apply("workspaces.task_prefix", `ALTER TABLE workspaces ADD COLUMN task_prefix TEXT DEFAULT 'KAN'`)
	r.migrate.Apply("workspaces.task_sequence", `ALTER TABLE workspaces ADD COLUMN task_sequence INTEGER DEFAULT 0`)
	r.migrate.Apply("workspaces.office_workflow_id", `ALTER TABLE workspaces ADD COLUMN office_workflow_id TEXT DEFAULT ''`)

	// Office session cost tracking extensions are declared in
	// initSessionWorktreeSchema's CREATE TABLE (cost_subcents, tokens_in,
	// tokens_cached_in, tokens_out). task_sessions.agent_profile_id existed
	// on main as NOT NULL; migrateSessionsRemoveAgentExecutionID rebuilds the
	// table with the column nullable and the cost columns added.

	r.migrate.Apply("workflows.is_system", `ALTER TABLE workflows ADD COLUMN is_system INTEGER DEFAULT 0`)

	// Phase 2 (ADR-0004) - workflows.style is a UX hint for the frontend
	// ("kanban" | "office" | "custom"). Backend code MUST NOT branch on
	// this value. Idempotent ALTER; default "kanban" preserves the current
	// presentation for existing workflows.
	r.migrate.Apply("workflows.style", `ALTER TABLE workflows ADD COLUMN style TEXT NOT NULL DEFAULT 'kanban'`)

	// Workflow-sync provenance: which system owns the workflow definition
	// ("manual" | "github") and, for synced workflows, the repo-relative
	// file path it was synced from.
	r.migrate.Apply("workflows.source", `ALTER TABLE workflows ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`)
	r.migrate.Apply("workflows.source_path", `ALTER TABLE workflows ADD COLUMN source_path TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("workflows.prompt", `ALTER TABLE workflows ADD COLUMN prompt TEXT NOT NULL DEFAULT ''`)
	if err := r.ensureImproveKandevWorkflowTemplateUniqueness(); err != nil {
		return err
	}

	// Native code review — indexes for the task_review_* tables. Declared here
	// rather than in initTaskReviewSchema because schema init is a no-op on an
	// existing DB (see AGENTS.md "Schema & migrations").
	r.migrate.Apply("idx_task_review_runs_task", `CREATE INDEX IF NOT EXISTS idx_task_review_runs_task ON task_review_runs(task_id, created_at)`)
	r.migrate.Apply("idx_task_review_findings_run", `CREATE INDEX IF NOT EXISTS idx_task_review_findings_run ON task_review_findings(run_id)`)
	r.migrate.Apply("idx_task_review_findings_task_status", `CREATE INDEX IF NOT EXISTS idx_task_review_findings_task_status ON task_review_findings(task_id, status)`)
	r.migrate.Apply("idx_task_review_findings_anchor", `CREATE INDEX IF NOT EXISTS idx_task_review_findings_anchor ON task_review_findings(task_id, repository_name, file_path)`)

	// entry_id carries the step-transition ledger row identifier of the
	// step-entry action that requested a run_code_review pass
	// (AC-OFFICE-STEP-ENTRY-001.10). The partial unique index is the durable
	// backstop: a redelivery of the same entry must not create a second run
	// row even if a caller races the FindRunByEntryID pre-check. Must run
	// after the ADD COLUMN — see AGENTS.md "Schema & migrations".
	r.migrate.Apply("task_review_runs.entry_id", `ALTER TABLE task_review_runs ADD COLUMN entry_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("idx_task_review_runs_entry_id",
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_task_review_runs_entry_id ON task_review_runs(entry_id) WHERE entry_id != ''`)

	// ADR 0005 Wave F — ensure the runner-projection tables exist so
	// task SELECTs that reference them via correlated subquery don't
	// fail. Required for tests and any environment where the workflow
	// repo hasn't run yet.
	r.ensureRunnerProjectionTables()
	// Keep the projection table compatible with databases whose workflow
	// repository has not replayed its own migrations yet. These additive
	// migrations are idempotent and preserve the false default for legacy rows.
	r.migrate.Apply("workflow_steps.auto_advance_requires_signal", `ALTER TABLE workflow_steps ADD COLUMN auto_advance_requires_signal INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("workflow_steps.cancel_triggers_turn_complete", `ALTER TABLE workflow_steps ADD COLUMN cancel_triggers_turn_complete INTEGER NOT NULL DEFAULT 0`)

	// Slack-style unread divider: the read cursor a session advances to the
	// latest message id whenever it becomes the visible chat panel. The
	// frontend snapshots the prior value before the advance to position the
	// "New" divider (see models.TaskSession.LastReadMessageID).
	r.migrate.Apply("task_sessions.last_read_message_id", `ALTER TABLE task_sessions ADD COLUMN last_read_message_id TEXT DEFAULT ''`)
	r.migrate.Apply("task_session_turns.execution_profile_id", `ALTER TABLE task_session_turns ADD COLUMN execution_profile_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_session_turns.route_generation", `ALTER TABLE task_session_turns ADD COLUMN route_generation BIGINT NOT NULL DEFAULT 0`)

	// Bounded task-level status projection. Keep this on the replay path as well
	// as the fresh schema path so an existing installation gets the table
	// without a destructive rebuild.
	r.migrate.Apply("task_status_summaries.table", `
		CREATE TABLE IF NOT EXISTS task_status_summaries (
			task_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			revision INTEGER NOT NULL DEFAULT 0,
			summary TEXT NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`)
	r.migrate.Apply("task_status_summaries.workspace", `
		CREATE INDEX IF NOT EXISTS idx_task_status_summaries_workspace
			ON task_status_summaries(workspace_id)`)

	if err := r.clearRecoveredAgentErrors(); err != nil {
		return err
	}

	// The execution-aware task_session_subagents schema is created directly by
	// initSubagentContextSchema. The predecessor change that introduced the
	// table is not part of the supported upgrade path, so there is no
	// intermediate-shape rebuild to run here. Only the historical-message
	// backfill belongs in the migration phase.
	r.migrateSubagentContextBackfill()

	// Durable per-session prompt ordinals. prompt_seq is allocated from a
	// per-session sequence counter inside the create write boundary, so an
	// ordinal survives message deletion and clock corrections. Previously the
	// ordinal was derived from the session's remaining user rows (a correlated
	// count), which renumbered prompts after a delete. SQLite forbids
	// non-constant column defaults, so the column is added with DEFAULT 0 and
	// existing user rows are backfilled with the previously derived ordinal
	// (count of user rows ordered before them by the normalized microsecond
	// key, ties by id); the counter is seeded to each session's backfilled
	// maximum. The backfill predicate (prompt_seq = 0) and ON CONFLICT keep
	// every statement idempotent across replays.
	r.migrate.Apply("task_session_messages.prompt_seq", `ALTER TABLE task_session_messages ADD COLUMN prompt_seq INTEGER NOT NULL DEFAULT 0`)
	r.migrate.Apply("task_session_prompt_seq.table", `
		CREATE TABLE IF NOT EXISTS task_session_prompt_seq (
			task_session_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL
		)`)
	if err := r.backfillPromptSeq(); err != nil {
		return err
	}

	// Workflow step display snapshot on plan revisions, same pattern as
	// author_name: the step a task was on when the revision was written.
	// Pre-existing revisions get empty strings, matching the fresh-DB default.
	r.migrate.Apply("task_plan_revisions.workflow_step_id", `ALTER TABLE task_plan_revisions ADD COLUMN workflow_step_id TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_plan_revisions.workflow_step_name", `ALTER TABLE task_plan_revisions ADD COLUMN workflow_step_name TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("task_plan_revisions.workflow_step_color", `ALTER TABLE task_plan_revisions ADD COLUMN workflow_step_color TEXT NOT NULL DEFAULT ''`)

	return nil
}

// backfillPromptSeq assigns existing user rows their previously derived
// ordinal (the correlated count over the session's user rows using the same
// normalized-microsecond predicate the create boundary used) and seeds each
// session's sequence counter at its backfilled maximum. Idempotent: user rows
// already carrying a nonzero prompt_seq are untouched, and the counter seed
// ignores existing rows.
func (r *Repository) backfillPromptSeq() error {
	nmU := dialect.NormalizedMicrosecond(r.db.DriverName(), "u.created_at")
	nmM := dialect.NormalizedMicrosecond(r.db.DriverName(), "task_session_messages.created_at")
	update := fmt.Sprintf(`
		UPDATE task_session_messages
		SET prompt_seq = (
			SELECT COUNT(*) FROM task_session_messages u
			WHERE u.task_session_id = task_session_messages.task_session_id
			  AND u.author_type = 'user'
			  AND (%s < %s OR (%s = %s AND u.id <= task_session_messages.id))
		)
		WHERE author_type = 'user' AND prompt_seq = 0`, nmU, nmM, nmU, nmM)
	if _, err := r.db.Exec(update); err != nil {
		return fmt.Errorf("backfill prompt_seq: %w", err)
	}
	seed := `
		INSERT INTO task_session_prompt_seq (task_session_id, last_seq)
		SELECT task_session_id, MAX(prompt_seq) FROM task_session_messages
		WHERE author_type = 'user' AND prompt_seq > 0
		GROUP BY task_session_id
		ON CONFLICT(task_session_id) DO NOTHING`
	if _, err := r.db.Exec(seed); err != nil {
		return fmt.Errorf("seed prompt sequence counters: %w", err)
	}
	return nil
}

// clearRecoveredAgentErrors repairs sessions whose stored agent failure was
// overtaken by successful work before the orchestrator learned to clear it.
//
// Nothing used to retire `last_agent_error`, so a failure the agent recovered
// from weeks ago still reads as live and keeps a red error icon on the task list
// forever. The orchestrator now clears the record on turn completion; this
// applies the same rule to history.
//
// Deliberately narrow: a record is cleared only when the session has an ordinary
// agent message newer than the failure — the same "the agent has produced good
// output since" signal. A failure with no successful work after it is current
// and is left alone.
func (r *Repository) clearRecoveredAgentErrors() error {
	postgres := dialect.IsPostgres(r.db.DriverName())

	sessionError := func(key string) string {
		return jsonText(postgres, "s.metadata", "last_agent_error", key)
	}
	recoveredSessions := `
		SELECT s.id
		FROM task_sessions s
		WHERE ` + sessionError("message") + ` IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM task_session_messages m
				WHERE m.task_session_id = s.id
					AND m.author_type = 'agent'
					AND m.type NOT IN ('error', 'status')
					AND ` + timestampColumn(postgres, "m.created_at") + ` > ` +
		timestampFromText(postgres, sessionError("occurred_at")) + `
			)`

	r.migrate.Apply("task_sessions.last_agent_error.recovered_cleanup",
		`UPDATE task_sessions SET metadata = `+jsonRemoveKey(postgres, "metadata", "last_agent_error")+
			` WHERE id IN (`+recoveredSessions+`)`)

	// The summary row caches the derived error, so clearing the session record
	// alone would leave the icon up until that session next emitted an event.
	// A cached error naming a session that has no stored failure is stale by
	// definition, which also sweeps up any earlier drift.
	r.migrate.Apply("task_status_summaries.active_error.recovered_cleanup", `
		UPDATE task_status_summaries
		SET summary = `+jsonRemoveKey(postgres, "summary", "active_error")+`
		WHERE `+jsonText(postgres, "summary", "active_error", "session_id")+` IN (
			SELECT s.id FROM task_sessions s
			WHERE `+sessionError("message")+` IS NULL
		)`)
	return nil
}

// jsonColumn makes a TEXT-typed JSON column safe to parse. Both dialects raise
// on an empty or malformed document — Postgres on the `::jsonb` cast, SQLite in
// `json_extract` — and these statements scan every row before filtering, so a
// single such row aborts the whole migration. The repository writes ” and
// 'null' for "no metadata", so both are normalized the way the rest of the
// package already does.
func jsonColumn(postgres bool, column string) string {
	empty, open := "'{}'", ""
	if postgres {
		empty, open = "'{}'::jsonb", "::jsonb"
	}
	return "(CASE WHEN " + column + " IS NULL OR " + column + " = 'null' OR " + column +
		" = '' THEN " + empty + " ELSE " + column + open + " END)"
}

// jsonText extracts a nested JSON text value from a TEXT-typed JSON column.
func jsonText(postgres bool, column, parent, key string) string {
	base := jsonColumn(postgres, column)
	if postgres {
		return "(" + base + " #>> '{" + parent + "," + key + "}')"
	}
	return "json_extract(" + base + ", '$." + parent + "." + key + "')"
}

// timestampColumn normalizes a timestamp column for comparison. SQLite stores
// timestamps as text and would compare them lexically, so it needs Julian days;
// on Postgres the column is already a timestamp.
func timestampColumn(postgres bool, column string) string {
	if postgres {
		return "(" + column + ")::timestamptz"
	}
	return "julianday(" + column + ")"
}

// timestampFromText normalizes a timestamp extracted from JSON so it compares
// against timestampColumn — the two carry different text shapes
// ('2026-08-01 10:00:00+00:00' vs RFC3339 '2026-08-01T10:00:00Z'). NULLIF keeps
// an empty stored timestamp from raising on the Postgres cast; it then compares
// as NULL, so the row simply does not match.
func timestampFromText(postgres bool, expression string) string {
	if postgres {
		return "(NULLIF(" + expression + ", '')::timestamptz)"
	}
	return "julianday(" + expression + ")"
}

func jsonRemoveKey(postgres bool, column, key string) string {
	base := jsonColumn(postgres, column)
	if postgres {
		return "(" + base + " - '" + key + "')::text"
	}
	return "json_remove(" + base + ", '$." + key + "')"
}

// jsonKey extracts a text value at a JSON path from a TEXT-typed JSON column.
// key may itself be dot-separated ("normalized.kind") to reach a field nested
// several levels deep — jsonInt and jsonBoolToInt build on this by folding
// their parent/key pair into one such path before extracting.
func jsonKey(postgres bool, column, key string) string {
	base := jsonColumn(postgres, column)
	segments := strings.Join(strings.Split(key, "."), ",")
	if postgres {
		return "(" + base + " #>> '{" + segments + "}')"
	}
	return "json_extract(" + base + ", '$." + strings.ReplaceAll(segments, ",", ".") + "')"
}

// jsonInt extracts a nested numeric field and normalizes it the way every
// unreported-or-invalid metric in this table normalizes: NULL, never a
// fabricated 0 for "not reported" and never a negative count (AC-7, AC-9,
// AC-23). Postgres casts to BIGINT, SQLite to INTEGER.
func jsonInt(postgres bool, column, parent, key string) string {
	extracted := jsonKey(postgres, column, parent+"."+key)
	castType := "INTEGER"
	if postgres {
		castType = "BIGINT"
	}
	cast := "CAST(NULLIF(" + extracted + ", '') AS " + castType + ")"
	return "(CASE WHEN " + cast + " < 0 THEN NULL ELSE " + cast + " END)"
}

// jsonBoolToInt normalizes is_async's dialect-inconsistent boolean spelling —
// Postgres's #>> text extraction yields 'true'/'false', SQLite's json_extract
// on a JSON boolean yields '1'/'0' — into the single 1/0 the column stores.
// An absent or false field yields 0, matching the column's own DEFAULT 0.
func jsonBoolToInt(postgres bool, column, parent, key string) string {
	extracted := jsonKey(postgres, column, parent+"."+key)
	// SQLite's json_extract returns a JSON boolean as the storage-class
	// INTEGER 1/0, not the text '1'/'0' — comparing that INTEGER against a
	// TEXT literal via IN never matches (SQLite orders INTEGER < TEXT by
	// storage class), so the extracted value must be cast to TEXT first.
	return "(CASE WHEN CAST(" + extracted + " AS TEXT) IN ('1', 'true') THEN 1 ELSE 0 END)"
}

// ensureImproveKandevWorkflowTemplateUniqueness removes the broad index from
// the initial implementation, reconciles legacy bootstrap duplicates, then
// enforces uniqueness only for the two hidden workflows created by this flow.
func (r *Repository) ensureImproveKandevWorkflowTemplateUniqueness() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin improve kandev workflow migration: %w", err)
	}
	// Keep this template list synchronized with every hidden Improve Kandev
	// workflow bootstrapped by internal/improvekandev. Add new IDs to both this
	// reconciliation query and the partial-index predicate below.
	if _, err := tx.Exec(`
		UPDATE workflows
		SET workflow_template_id = ''
		WHERE id IN (
			SELECT duplicate.id
			FROM workflows AS duplicate
			WHERE duplicate.workflow_template_id IN ('improve-kandev', 'report-kandev-issue')
				AND EXISTS (
					SELECT 1
					FROM workflows AS canonical
					WHERE canonical.workspace_id = duplicate.workspace_id
						AND canonical.workflow_template_id = duplicate.workflow_template_id
						AND canonical.hidden = duplicate.hidden
						AND (
							canonical.created_at < duplicate.created_at
							OR (canonical.created_at = duplicate.created_at AND canonical.id < duplicate.id)
						)
				)
		)`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("reconcile improve kandev workflow duplicates: %w", err)
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS uniq_workflows_workspace_template_hidden`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("drop broad workflow template index: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uniq_improve_kandev_workflows
		ON workflows(workspace_id, workflow_template_id, hidden)
		WHERE workflow_template_id IN ('improve-kandev', 'report-kandev-issue')`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("create improve kandev workflow index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit improve kandev workflow migration: %w", err)
	}
	return nil
}

// ensureTaskWorkspaceFoldersSchema upgrades databases created before durable
// folder attachments existed. CREATE TABLE/INDEX IF NOT EXISTS is replay-safe
// on SQLite and Postgres.
func (r *Repository) ensureTaskWorkspaceFoldersSchema() error {
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS task_workspace_folders (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			local_path TEXT NOT NULL,
			display_name TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			UNIQUE(task_id, local_path),
			UNIQUE(task_id, display_name)
		);
		CREATE INDEX IF NOT EXISTS idx_task_workspace_folders_task_position
			ON task_workspace_folders(task_id, position);
	`); err != nil {
		return fmt.Errorf("create task workspace folders schema: %w", err)
	}
	return nil
}

// ensureRepositorySetsSchema upgrades databases created before repository sets
// existed. It replays the same DDL as the schema-init step; CREATE TABLE/INDEX
// IF NOT EXISTS is replay-safe on SQLite and Postgres.
func (r *Repository) ensureRepositorySetsSchema() error {
	if _, err := r.db.Exec(repositorySetsSchemaDDL); err != nil {
		return fmt.Errorf("create repository sets schema: %w", err)
	}
	return nil
}

// ensureRepositoryBranchPoliciesSchema replays the policy DDL for databases
// created before repository branch policies existed.
func (r *Repository) ensureRepositoryBranchPoliciesSchema() error {
	if _, err := r.db.Exec(repositoryBranchPoliciesSchemaDDL); err != nil {
		return fmt.Errorf("create repository branch policies schema: %w", err)
	}
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
	if dialect.IsPostgres(r.db.DriverName()) {
		return false, nil
	}

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
			wip_admitted INTEGER NOT NULL DEFAULT 1,
			queued_for_step_id TEXT NOT NULL DEFAULT '',
			queued_at TIMESTAMP,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			autopilot_enabled INTEGER NOT NULL DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`INSERT INTO tasks_new SELECT
			id, workspace_id, workflow_id, workflow_step_id, title, description,
			state, priority, position, wip_admitted, queued_for_step_id, queued_at, metadata, is_ephemeral, parent_id, autopilot_enabled, archived_at, created_at, updated_at
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
	if dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}

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

// commitCaptureActivatedAtMetaKey is the kandev_meta key published by
// migrateSessionCommitsDedupeAndActivation.
const commitCaptureActivatedAtMetaKey = "commit_capture_activated_at"

// migrateSessionCommitsDedupeAndActivation enforces uniqueness on
// task_session_commits(session_id, commit_sha) and publishes the point in
// time commit capture started firing from more than just archive capture, so
// downstream readers (the Rill extract, ListSessionCodeStats) can tell
// "capture wasn't running yet" apart from "session made zero commits" -
// both previously read as a plain 0.
//
// Dedup keeps the earliest-observed row per (session_id, commit_sha), ties
// broken by id: task_session_commits is an append-only observation ledger
// (a rebase/squash adds new SHAs, it never retroactively changes which row
// was first seen), so "earliest seen" is the row future replays preserve.
//
// Rebase/squash decision: immutable observation history, not the final
// branch object set. A rebase that drops a previously-observed commit SHA
// from reachable history leaves that row in place - it is not deleted or
// reconciled against the current branch. Same tradeoff the task brief
// already accepts for summed commit diffstats counting churn and reverts
// (task_session_git_snapshots deltas are the net-branch-growth answer;
// commit rows are the observation-history answer, published side by side,
// not merged into one number). Live capture makes this more visible than
// the old archive-only design (which only ever ran GetGitLog once, at
// archive time, so it naturally reflected whatever was reachable from HEAD
// at that instant) - continuous capture can now persist a commit that a
// later rebase makes unreachable. Accepted rather than reconciled: pruning
// on every rebase/force-push would require diffing against live git state
// on every sweep, and "what got captured" is itself useful observability
// (e.g. abandoned work), not just noise.
//
// Must run before CreateSessionCommit starts firing from more than archive -
// CREATE UNIQUE INDEX fails on an existing duplicate pair, and
// MigrateLogger.Apply swallows non-"already exists" errors, so an unhandled
// duplicate would silently leave both the index and every future
// ON CONFLICT missing. That is exactly why these two statements, unlike most
// migrations in this file, do NOT go through r.migrate.Apply: the writer's
// ON CONFLICT (session_id, commit_sha) target hard-requires this index to
// exist, so a failure here must abort boot (propagated below) rather than
// leave every future commit insert failing silently forever.
func (r *Repository) migrateSessionCommitsDedupeAndActivation() error {
	if _, err := r.db.Exec(`
		DELETE FROM task_session_commits
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id,
					ROW_NUMBER() OVER (
						PARTITION BY session_id, commit_sha
						ORDER BY created_at ASC, id ASC
					) AS rn
				FROM task_session_commits
			) ranked
			WHERE rn = 1
		)
	`); err != nil {
		return fmt.Errorf("dedupe task_session_commits: %w", err)
	}
	if _, err := r.db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_session_commits_session_sha ON task_session_commits(session_id, commit_sha)`,
	); err != nil {
		return fmt.Errorf("create uniq_session_commits_session_sha: %w", err)
	}
	if r.log != nil {
		r.log.Info("migration applied", zap.String("name", "task_session_commits.dedupe_and_unique_index"))
	}

	// kandev_meta already exists by the time repository migrations run in
	// production (persistence.Provide creates it before opening any
	// repository), but repo-level tests build a bare DB via NewWithDB where
	// it does not, so recreate it defensively.
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS kandev_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		return fmt.Errorf("ensure kandev_meta: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := r.db.Exec(r.db.Rebind(`
		INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO NOTHING
	`), commitCaptureActivatedAtMetaKey, now); err != nil {
		return fmt.Errorf("write %s: %w", commitCaptureActivatedAtMetaKey, err)
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
			execution_profile_id TEXT NOT NULL DEFAULT '',
			route_generation INTEGER NOT NULL DEFAULT 0,
			route_state TEXT NOT NULL DEFAULT '',
			route_reason TEXT NOT NULL DEFAULT '',
			downstream_acp_session_id TEXT NOT NULL DEFAULT '',
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
			tokens_cached_in BIGINT NOT NULL DEFAULT 0,
			tokens_out INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`INSERT INTO task_sessions_new SELECT
			id, task_id, agent_profile_id, execution_profile_id,
			route_generation, route_state, route_reason, downstream_acp_session_id,
			executor_id, executor_profile_id, environment_id, repository_id, base_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, is_passthrough, review_status,
			COALESCE(base_commit_sha, ''), COALESCE(task_environment_id, ''),
			0, 0, 0, 0
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
			materialization_session_id TEXT DEFAULT '',
			worktree_id TEXT DEFAULT '',
			worktree_path TEXT DEFAULT '',
			worktree_branch TEXT DEFAULT '',
			workspace_path TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			container_bootstrap_nonce_secret_id TEXT DEFAULT '',
			container_control_auth_token_secret_id TEXT DEFAULT '',
			sandbox_id TEXT DEFAULT '',
			task_dir_name TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`INSERT INTO task_environments_new SELECT
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			control_port, status, '', worktree_id, worktree_path, worktree_branch,
			workspace_path, container_id, COALESCE(container_bootstrap_nonce_secret_id, ''), COALESCE(container_control_auth_token_secret_id, ''), sandbox_id,
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

func (r *Repository) migrateTaskEnvironmentReposAllowMultiBranch() error {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.migrateTaskEnvironmentReposAllowMultiBranchPostgres()
	}
	return r.recreateTableNamed(
		"task_environment_repos.recreate_allow_multi_branch",
		"task_environment_repos",
		"UNIQUE(task_environment_id, repository_id)",
		[]string{
			`CREATE TABLE task_environment_repos_new (
				id TEXT PRIMARY KEY,
				task_environment_id TEXT NOT NULL,
				repository_id TEXT NOT NULL,
				branch_slug TEXT NOT NULL DEFAULT '',
				worktree_id TEXT DEFAULT '',
				worktree_path TEXT DEFAULT '',
				worktree_branch TEXT DEFAULT '',
				position INTEGER DEFAULT 0,
				error_message TEXT DEFAULT '',
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE,
				UNIQUE(task_environment_id, repository_id, branch_slug)
			)`,
			`INSERT INTO task_environment_repos_new SELECT
				id, task_environment_id, repository_id, '',
				worktree_id, worktree_path, worktree_branch,
				position, error_message, created_at, updated_at
			FROM task_environment_repos`,
			`DROP TABLE task_environment_repos`,
			`ALTER TABLE task_environment_repos_new RENAME TO task_environment_repos`,
			`CREATE INDEX IF NOT EXISTS idx_task_environment_repos_env_id ON task_environment_repos(task_environment_id)`,
			`CREATE INDEX IF NOT EXISTS idx_task_environment_repos_repository_id ON task_environment_repos(repository_id)`,
		},
	)
}

func (r *Repository) migrateTaskEnvironmentReposAllowMultiBranchPostgres() error {
	if _, err := r.db.Exec(`ALTER TABLE task_environment_repos ADD COLUMN IF NOT EXISTS branch_slug TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add task_environment_repos.branch_slug: %w", err)
	}
	if _, err := r.db.Exec(`
DO $$
DECLARE
	old_constraint_name text;
BEGIN
	SELECT con.conname INTO old_constraint_name
	FROM pg_constraint con
	JOIN pg_class rel ON rel.oid = con.conrelid
	JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
	WHERE rel.relname = 'task_environment_repos'
		AND nsp.nspname = current_schema()
		AND con.contype = 'u'
		AND (
			SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
			FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
			JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
		) = ARRAY['task_environment_id', 'repository_id'];

	IF old_constraint_name IS NOT NULL THEN
		EXECUTE format('ALTER TABLE task_environment_repos DROP CONSTRAINT %I', old_constraint_name);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
		WHERE rel.relname = 'task_environment_repos'
			AND nsp.nspname = current_schema()
			AND con.contype = 'u'
			AND (
				SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
				FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
				JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
			) = ARRAY['task_environment_id', 'repository_id', 'branch_slug']
	) THEN
		ALTER TABLE task_environment_repos
			ADD CONSTRAINT task_environment_repos_env_repo_branch_key
			UNIQUE (task_environment_id, repository_id, branch_slug);
	END IF;
END $$;
	`); err != nil {
		return fmt.Errorf("migrate task_environment_repos unique constraint: %w", err)
	}
	return nil
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
				branch_policy_id TEXT DEFAULT '',
				branch_policy_name TEXT DEFAULT '',
				branch_policy_base_branch TEXT DEFAULT '',
				branch_policy_branch_template TEXT DEFAULT '',
				branch_policy_pull_request_target TEXT DEFAULT '',
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
				COALESCE(branch_policy_id, ''), COALESCE(branch_policy_name, ''),
				COALESCE(branch_policy_base_branch, ''), COALESCE(branch_policy_branch_template, ''),
				COALESCE(branch_policy_pull_request_target, ''),
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
			execution_profile_id TEXT NOT NULL DEFAULT '',
			route_generation INTEGER NOT NULL DEFAULT 0,
			route_state TEXT NOT NULL DEFAULT '',
			route_reason TEXT NOT NULL DEFAULT '',
			downstream_acp_session_id TEXT NOT NULL DEFAULT '',
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
			id, task_id, agent_execution_id, container_id, agent_profile_id, execution_profile_id,
			route_generation, route_state, route_reason, downstream_acp_session_id,
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

// The legacy startup heals for task environments (backfillTaskEnvironments,
// backfillTaskEnvironmentRepos, healTaskEnvironmentWorkspacePaths) were folded
// into the one-time worktree ownership cutover
// (normalizeTaskWorktreeOwnership in worktree_ownership_migration.go). The
// legacy tables and columns they read no longer exist at startup.
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

// Startup healing of orphaned workflow_step_id values was removed: a raw SQL
// UPDATE reassigning tasks to a workflow's start step bypasses every
// domain-level invariant the task service enforces on a move (WIP limits,
// task-state sync, position bookkeeping, session/on_exit/on_enter handling,
// transition history, and event publication). The Kanban/Pipeline "Needs
// Reassignment" fallback column now keeps orphaned tasks visible without any
// automatic mutation, and any real repair should go through
// task.Service.MoveTask (or a dedicated, explicit reassignment operation)
// rather than a migration-time SQL statement.
