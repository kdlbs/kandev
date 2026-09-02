// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
)

// initSchema creates the database tables if they don't exist and applies
// pending migrations in order. Steps are sequenced through an explicit
// table so the call list stays readable as new schema/migration steps are
// added without growing the function's cyclomatic complexity.
func (r *Repository) initSchema() error {
	steps := []func() error{
		r.initCoreSchema,
		r.initRepositorySetsSchema,
		r.initRepositoryBranchPoliciesSchema,
		r.initPlansSchema,
		r.initWalkthroughsSchema,
		r.initDocumentsSchema,
		r.initSessionSchema,
		r.initDynamicRoutingSchema,
		r.initStepTransitionsSchema,
		r.initStepEntriesSchema,
		r.initTaskUsageEventsSchema,
		r.initAttachmentsSchema,
		r.initTaskResourceCleanupSchema,
		r.initGitSchema,
		r.initReviewSchema,
		r.initTaskReviewSchema,
		r.migrateExecutorProfiles,
		r.migrateTaskSessions,
		r.ensureDefaultWorkspace,
		r.ensureDefaultExecutorsAndEnvironments,
		r.runMigrations,
		r.hideBuiltinWorkflows,
		r.healBuiltinWorkflowStepFlags,
		r.healBuiltinWorkflowStepParticipantSeats,
		r.normalizeTaskWorktreeOwnership,
		r.healDuplicateTaskEnvironments,
		r.ensureTaskEnvironmentTaskUniqueIndex,
		r.healSessionTaskEnvironmentIDs,
		r.migrateGitSnapshotOwnership,
		r.ensureWorkspaceIndexes,
		r.ensureMessageMetadataIndexes,
		r.ensurePromptOrderIndex,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) initDynamicRoutingSchema() error {
	_, err := r.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS dynamic_route_states (
			session_id TEXT PRIMARY KEY,
			logical_profile_id TEXT NOT NULL,
			execution_profile_id TEXT NOT NULL DEFAULT '',
			route_generation BIGINT NOT NULL DEFAULT 0,
			profile_version BIGINT NOT NULL DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'selecting',
			continuation_json TEXT NOT NULL DEFAULT '',
			policy_state_json TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS dynamic_route_attempts (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			logical_profile_id TEXT NOT NULL,
			execution_profile_id TEXT NOT NULL DEFAULT '',
			route_generation BIGINT NOT NULL,
			profile_version BIGINT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			UNIQUE (session_id, route_generation),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_dynamic_route_attempts_session
			ON dynamic_route_attempts(session_id, route_generation);

		CREATE TABLE IF NOT EXISTS dynamic_resource_circuits (
			resource_key TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			until_at TIMESTAMP,
			code TEXT NOT NULL DEFAULT '',
			probe_until TIMESTAMP,
			updated_at TIMESTAMP NOT NULL
		);

		CREATE TABLE IF NOT EXISTS dynamic_installation_keys (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			key_bytes %s NOT NULL,
			created_at TIMESTAMP NOT NULL
		);
	`, dialect.BlobType(r.db.DriverName())))
	return err
}

const taskResourceCleanupSchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_resource_cleanup_jobs (
		id TEXT PRIMARY KEY,
		operation_id TEXT NOT NULL UNIQUE,
		task_id TEXT NOT NULL,
		trigger TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'pending',
		resource_snapshot TEXT NOT NULL DEFAULT '{}',
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TIMESTAMP,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_task_resource_cleanup_jobs_task_id
		ON task_resource_cleanup_jobs(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_resource_cleanup_jobs_due
		ON task_resource_cleanup_jobs(state, next_attempt_at, created_at);
`

func (r *Repository) initTaskResourceCleanupSchema() error {
	_, err := r.db.Exec(taskResourceCleanupSchemaDDL)
	return err
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
	driver := r.db.DriverName()
	toolCallIndex := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS idx_messages_metadata_tool_call_id ON task_session_messages(task_session_id, (%s))`,
		dialect.JSONExtract(driver, "metadata", "tool_call_id"),
	)
	if _, err := r.db.Exec(toolCallIndex); err != nil {
		return err
	}
	pendingIndex := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS idx_messages_metadata_pending_id ON task_session_messages(task_session_id, (%s))`,
		dialect.JSONExtract(driver, "metadata", "pending_id"),
	)
	if _, err := r.db.Exec(pendingIndex); err != nil {
		return err
	}
	return nil
}

// ensurePromptOrderIndex creates additive expression indexes for prompt-order
// reads. The user-only index also puts author_type before the ordering key, so
// filtered prompt pages skip interleaved agent and tool rows.
func (r *Repository) ensurePromptOrderIndex() error {
	ddl := dialect.PromptOrderIndexDDL(r.db.DriverName(), "idx_messages_prompt_order", "task_session_messages")
	if _, err := r.db.Exec(ddl); err != nil {
		return fmt.Errorf("create prompt-order index: %w", err)
	}
	userDDL := dialect.PromptUserOrderIndexDDL(
		r.db.DriverName(), "idx_messages_prompt_user_order", "task_session_messages",
	)
	if _, err := r.db.Exec(userDDL); err != nil {
		return fmt.Errorf("create prompt-user-order index: %w", err)
	}
	return nil
}

// ensureRunnerProjectionTables creates stub workflow_steps and
// workflow_step_participants tables if they're not yet present. The
// task repo's task SELECT projection includes a correlated subquery
// against both tables to resolve the per-task runner (ADR 0005 Wave F);
// when only the task repo is initialised (e.g. unit tests), the
// workflow repo hasn't created the canonical tables and the queries
// would error with "no such table". Stubs created here are minimal —
// the workflow repo's init still runs and adds the rest of its columns
// via idempotent ALTER and CREATE statements.
func (r *Repository) ensureRunnerProjectionTables() {
	// workflow_steps: matches the full schema declared in the workflow
	// repo so workflow.NewWithDB's later ALTER ADD COLUMNs become no-ops
	// (column-already-exists errors are swallowed). Mirrors
	// internal/workflow/repository/sqlite.go (the canonical owner).
	_, _ = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT NOT NULL DEFAULT '',
			stage_type TEXT NOT NULL DEFAULT 'custom',
			auto_advance_requires_signal INTEGER NOT NULL DEFAULT 0,
			cancel_triggers_turn_complete INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`)
	_, _ = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0
		)`)
}

func (r *Repository) initCoreSchema() error {
	if err := r.initInfraSchema(); err != nil {
		return err
	}
	if err := r.initTaskSchema(); err != nil {
		return err
	}
	if err := r.initTaskStatusSummarySchema(); err != nil {
		return err
	}
	return r.initCoreIndexes()
}

const taskStatusSummarySchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_status_summaries (
		task_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		revision INTEGER NOT NULL DEFAULT 0,
		summary TEXT NOT NULL DEFAULT '{}',
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_status_summaries_workspace
		ON task_status_summaries(workspace_id);
`

func (r *Repository) initTaskStatusSummarySchema() error {
	_, err := r.db.Exec(taskStatusSummarySchemaDDL)
	return err
}

// infraSchemaDDL is the concatenated CREATE TABLE block for infrastructure
// tables (workspaces, executors, environments, workflows). Kept as a single
// DDL string so the table layout reads top-to-bottom; lives in a const so the
// owning function stays within the funlen limit.
const infraSchemaDDL = `
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
		execution_profile_id TEXT NOT NULL DEFAULT '',
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
		local_pid INTEGER DEFAULT 0,
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
		prompt TEXT NOT NULL DEFAULT '',
		hidden INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
`

func (r *Repository) initInfraSchema() error {
	_, err := r.db.Exec(infraSchemaDDL)
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
		wip_admitted INTEGER NOT NULL DEFAULT 1,
		queued_for_step_id TEXT NOT NULL DEFAULT '',
		queued_at TIMESTAMP,
		metadata TEXT DEFAULT '{}',
		autopilot_enabled INTEGER NOT NULL DEFAULT 0,
		archived_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'local',
		local_path TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		provider_repo_id TEXT DEFAULT '',
		provider_host TEXT DEFAULT '',
		provider_scope TEXT DEFAULT '',
		provider_owner TEXT DEFAULT '',
		provider_name TEXT DEFAULT '',
		remote_url TEXT DEFAULT '',
		default_branch TEXT DEFAULT '',
		worktree_branch_prefix TEXT DEFAULT 'feature/',
		worktree_branch_template TEXT DEFAULT 'feature/{title}-{suffix}',
		pull_before_worktree INTEGER NOT NULL DEFAULT 1,
		setup_script TEXT DEFAULT '',
		cleanup_script TEXT DEFAULT '',
		dev_script TEXT DEFAULT '',
		copy_files TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repository_secret_bindings (
		repository_id TEXT NOT NULL,
		key TEXT NOT NULL,
		secret_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (repository_id, key),
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_repository_secret_bindings_repository
		ON repository_secret_bindings(repository_id);

	CREATE TABLE IF NOT EXISTS task_repositories (
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
	);

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

// repositorySetsSchemaDDL declares the repository-set tables. It runs after
// initCoreSchema so `workspaces` and `repositories` exist for the foreign keys.
//
// Membership positions are contiguous from zero and carry no branch: branch
// choice belongs to a task (task_repositories), which is exactly what the user
// still decides after applying a set.
const repositorySetsSchemaDDL = `
	CREATE TABLE IF NOT EXISTS repository_sets (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
		UNIQUE(workspace_id, name)
	);

	CREATE TABLE IF NOT EXISTS repository_set_items (
		id TEXT PRIMARY KEY,
		repository_set_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (repository_set_id) REFERENCES repository_sets(id) ON DELETE CASCADE,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
		UNIQUE(repository_set_id, repository_id)
	);

	CREATE INDEX IF NOT EXISTS idx_repository_sets_workspace_id
		ON repository_sets(workspace_id);
	-- Names are compared case-insensitively, so the plain UNIQUE(workspace_id,
	-- name) above is not the concurrency backstop the service assumes: two
	-- concurrent creates of "Full-stack" and "full-stack" would both pass the
	-- service's lookup and both insert. An expression index closes that, and
	-- LOWER() is available on both SQLite and Postgres.
	CREATE UNIQUE INDEX IF NOT EXISTS uniq_repository_sets_workspace_lower_name
		ON repository_sets(workspace_id, LOWER(name));
	CREATE INDEX IF NOT EXISTS idx_repository_set_items_set_position
		ON repository_set_items(repository_set_id, position);
	CREATE INDEX IF NOT EXISTS idx_repository_set_items_repository_id
		ON repository_set_items(repository_id);
	`

func (r *Repository) initRepositorySetsSchema() error {
	_, err := r.db.Exec(repositorySetsSchemaDDL)
	return err
}

const repositoryBranchPoliciesSchemaDDL = `
	CREATE TABLE IF NOT EXISTS repository_branch_policies (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL,
		branch_template TEXT NOT NULL,
		pull_request_target TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_repository_branch_policies_repository_id
		ON repository_branch_policies(repository_id);
	CREATE UNIQUE INDEX IF NOT EXISTS uniq_repository_branch_policies_repository_lower_name
		ON repository_branch_policies(repository_id, LOWER(name));
`

func (r *Repository) initRepositoryBranchPoliciesSchema() error {
	_, err := r.db.Exec(repositoryBranchPoliciesSchemaDDL)
	return err
}

func (r *Repository) initCoreIndexes() error {
	_, err := r.db.Exec(`
	CREATE INDEX IF NOT EXISTS idx_tasks_workflow_id ON tasks(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_workflow_step_id ON tasks(workflow_step_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_archived_at ON tasks(archived_at);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_task_id ON task_repositories(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_repository_id ON task_repositories(repository_id);
	CREATE INDEX IF NOT EXISTS idx_task_workspace_folders_task_position ON task_workspace_folders(task_id, position);
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
		implementation_started_at TIMESTAMP,
		implementation_started_session_id TEXT,
		implementation_started_by TEXT,
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
		workflow_step_id TEXT NOT NULL DEFAULT '',
		workflow_step_name TEXT NOT NULL DEFAULT '',
		workflow_step_color TEXT NOT NULL DEFAULT '',
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

// initWalkthroughsSchema creates the task_walkthroughs table. Steps are stored
// as a JSON array in a single column (read/written whole — there is no
// per-step query path), keeping the artifact one row per task like task_plans.
func (r *Repository) initWalkthroughsSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_walkthroughs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL DEFAULT 'Walkthrough',
		steps TEXT NOT NULL DEFAULT '[]',
		created_by TEXT NOT NULL DEFAULT 'agent',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_task_walkthroughs_task_id ON task_walkthroughs(task_id);
	`)
	return err
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

// initDocumentsSchema creates the task_documents and task_document_revisions tables.
// These tables generalize task_plans: documents have a key (e.g., "plan", "spec") and type.
func (r *Repository) initDocumentsSchema() error {
	if _, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_documents (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		key TEXT NOT NULL DEFAULT 'plan',
		type TEXT NOT NULL DEFAULT 'plan',
		title TEXT NOT NULL DEFAULT 'Plan',
		content TEXT NOT NULL DEFAULT '',
		author_kind TEXT NOT NULL DEFAULT 'agent',
		author_name TEXT NOT NULL DEFAULT '',
		filename TEXT NOT NULL DEFAULT '',
		mime_type TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		disk_path TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		UNIQUE(task_id, key)
	);
	CREATE INDEX IF NOT EXISTS idx_task_documents_task_id ON task_documents(task_id);

	CREATE TABLE IF NOT EXISTS task_document_revisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		document_key TEXT NOT NULL DEFAULT 'plan',
		revision_number INTEGER NOT NULL,
		title TEXT NOT NULL DEFAULT 'Plan',
		content TEXT NOT NULL DEFAULT '',
		author_kind TEXT NOT NULL DEFAULT 'agent',
		author_name TEXT NOT NULL DEFAULT '',
		revert_of_revision_id TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		UNIQUE (task_id, document_key, revision_number)
	);
	CREATE INDEX IF NOT EXISTS idx_task_document_revisions_task_key
		ON task_document_revisions(task_id, document_key, revision_number DESC);
	`); err != nil {
		return fmt.Errorf("init documents schema: %w", err)
	}
	return nil
}

// initAttachmentsSchema creates the descriptor registry for file-backed prompt
// attachments. The storage key is intentionally opaque and never exposed in
// API responses; bytes are owned by the attachment service.
func (r *Repository) initAttachmentsSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_message_attachments (
		id TEXT PRIMARY KEY,
		owner_id TEXT NOT NULL DEFAULT '',
		workspace_id TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		message_id TEXT NOT NULL DEFAULT '',
		queue_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
		kind TEXT NOT NULL DEFAULT 'resource',
		delivery_mode TEXT NOT NULL DEFAULT 'prompt',
		size_bytes INTEGER NOT NULL,
		storage_key TEXT NOT NULL UNIQUE,
		state TEXT NOT NULL DEFAULT 'staged',
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_task_message_attachments_owner_state
		ON task_message_attachments(owner_id, state, expires_at);
	CREATE INDEX IF NOT EXISTS idx_task_message_attachments_scope
		ON task_message_attachments(workspace_id, task_id, session_id, state);
	`)
	if err != nil {
		return fmt.Errorf("init attachments schema: %w", err)
	}
	return nil
}

func (r *Repository) initSessionSchema() error {
	if err := r.initSessionWorktreeSchema(); err != nil {
		return err
	}
	if err := r.initMessageTurnSchema(); err != nil {
		return err
	}
	return r.initSubagentContextSchema()
}

// initSubagentContextSchema creates task_session_subagents, the durable
// relational record of a subagent (Task tool) invocation. See
// docs/specs/agents/requirements/subagent-context-persistence.md. The three measurement
// columns (total_tokens, tool_use_count, duration_ms) deliberately carry no
// DEFAULT: an unreported value must store NULL, never 0 (75% of observed
// invocations report none of them). turn_id carries no FOREIGN KEY so a turn
// deletion never silently deletes the fan-out record it measured.
//
// agent_execution_id carries DEFAULT 'unknown' (AC-31's reserved sentinel for
// "no execution identity available") so every row has one, and is part of the
// UNIQUE key (task_session_id, agent_execution_id, tool_call_id) — a late
// frame from an earlier, already-completed execution creates/updates its own
// row instead of clobbering a later execution's (AC-32). Historical message
// rows are handled separately by the one-time backfill migration.
func (r *Repository) initSubagentContextSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_session_subagents (
		id                  TEXT PRIMARY KEY,
		task_session_id     TEXT NOT NULL,
		task_id             TEXT NOT NULL,
		turn_id             TEXT,
		tool_call_id        TEXT NOT NULL,
		agent_execution_id  TEXT NOT NULL DEFAULT 'unknown',
		parent_tool_call_id TEXT,
		subagent_type       TEXT,
		description         TEXT,
		agent_id            TEXT,
		child_session_id    TEXT,
		model               TEXT,
		agent_status        TEXT,
		tool_status         TEXT,
		is_async            INTEGER NOT NULL DEFAULT 0,
		total_tokens        INTEGER,
		tool_use_count      INTEGER,
		duration_ms         INTEGER,
		source              TEXT NOT NULL DEFAULT 'live',
		observed_at         TIMESTAMP NOT NULL,
		settled_at          TIMESTAMP,
		updated_at          TIMESTAMP NOT NULL,
		UNIQUE (task_session_id, agent_execution_id, tool_call_id),
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_subagents_session_id ON task_session_subagents(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_subagents_task_id    ON task_session_subagents(task_id);
	CREATE INDEX IF NOT EXISTS idx_subagents_turn_id    ON task_session_subagents(turn_id);
	`)
	return err
}

// initStepTransitionsSchema creates task_step_transitions: one row per
// committed change to tasks.workflow_step_id. This is a new table, not a
// column added to an existing one, so CREATE TABLE IF NOT EXISTS in the init
// block is correct and complete — the "columns only via runMigrations" rule
// governs ALTER TABLE, not table creation.
//
// Deliberately no foreign key to workflow_steps or workflows: steps and
// workflows get deleted, and the historical fact that a card was in a
// now-deleted step must survive that deletion.
func (r *Repository) initStepTransitionsSchema() error {
	idCol := dialect.AutoIncrementIDColumn(r.db.DriverName())
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_step_transitions (
		` + idCol + `,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
		from_workflow_id TEXT,
		from_workflow_step_id TEXT,
		to_workflow_id TEXT,
		to_workflow_step_id TEXT,
		trigger TEXT NOT NULL,
		actor_kind TEXT NOT NULL,
		actor_id TEXT,
		contract_version INTEGER NOT NULL,
		occurred_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_task_step_transitions_task
		ON task_step_transitions(task_id, occurred_at, id);
	CREATE INDEX IF NOT EXISTS idx_task_step_transitions_occurred
		ON task_step_transitions(occurred_at);
	`)
	if err != nil {
		return fmt.Errorf("init step transitions schema: %w", err)
	}
	return nil
}

func (r *Repository) initMessageTurnSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_session_turns (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		execution_profile_id TEXT NOT NULL DEFAULT '',
		route_generation BIGINT NOT NULL DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_turns_session_id ON task_session_turns(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_turns_session_started ON task_session_turns(task_session_id, started_at);
	CREATE INDEX IF NOT EXISTS idx_turns_task_id ON task_session_turns(task_id);

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
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON task_session_messages(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON task_session_messages(created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_session_messages(task_session_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_turn_id ON task_session_messages(turn_id);
	-- The automation run log reads each run's last agent message by task_id
	-- (automation.listRunsWithTaskState). Every other index here leads with
	-- task_session_id, so without this that lookup falls back to the global
	-- created_at index and rescans it once per run row.
	CREATE INDEX IF NOT EXISTS idx_messages_task_author_created
		ON task_session_messages(task_id, author_type, type, created_at DESC);
	-- idx_messages_session_updated is created in runMigrations() after the
	-- updated_at ADD COLUMN + backfill. Creating it here would fail on existing
	-- DBs where CREATE TABLE IF NOT EXISTS is a no-op and the column does not
	-- yet exist (schema init runs before migrations).
	`)
	return err
}

// sessionWorktreeSchemaDDL groups task_sessions, task_environments, and
// task_environment_repos DDL so the owning function stays within the funlen
// limit. task_environment_repos is the only physical-worktree record; the
// legacy task_session_worktrees table and the flat worktree columns on
// task_environments were removed by the one-time cutover migration
// (normalizeTaskWorktreeOwnership).
const sessionWorktreeSchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_sessions (
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
		cost_subcents INTEGER NOT NULL DEFAULT 0,
		tokens_in INTEGER NOT NULL DEFAULT 0,
		tokens_cached_in BIGINT NOT NULL DEFAULT 0,
		tokens_out INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state);

	CREATE TABLE IF NOT EXISTS task_environments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		executor_type TEXT NOT NULL DEFAULT '',
		executor_id TEXT DEFAULT '',
		executor_profile_id TEXT DEFAULT '',
		control_port INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'creating',
		materialization_session_id TEXT DEFAULT '',
		workspace_path TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		container_bootstrap_nonce_secret_id TEXT DEFAULT '',
		container_control_auth_token_secret_id TEXT DEFAULT '',
		sandbox_id TEXT DEFAULT '',
		task_dir_name TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_environments_task_id ON task_environments(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_environments_status ON task_environments(status);

	CREATE TABLE IF NOT EXISTS task_environment_repos (
		id TEXT PRIMARY KEY,
		task_environment_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		branch_slug TEXT NOT NULL DEFAULT '',
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		error_message TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		merged_at TIMESTAMP,
		deleted_at TIMESTAMP,
		FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE,
		UNIQUE(task_environment_id, repository_id, branch_slug)
	);

	CREATE INDEX IF NOT EXISTS idx_task_environment_repos_env_id ON task_environment_repos(task_environment_id);
	CREATE INDEX IF NOT EXISTS idx_task_environment_repos_repository_id ON task_environment_repos(repository_id);
`

func (r *Repository) initSessionWorktreeSchema() error {
	_, err := r.db.Exec(sessionWorktreeSchemaDDL)
	return err
}

func (r *Repository) initGitSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_session_git_snapshots (
		id TEXT PRIMARY KEY,
		task_environment_id TEXT NOT NULL,
		session_id TEXT,
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
		FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL
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

// taskReviewSchemaDDL creates the native code-review tables: one row per
// review pass and one row per anchored finding. Indexes live in
// runMigrations (see AGENTS.md "Schema & migrations") so an existing DB that
// skips this no-op CREATE still gets them.
const taskReviewSchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_review_runs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		trigger TEXT NOT NULL DEFAULT 'manual',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		agent_id TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		finding_count INTEGER NOT NULL DEFAULT 0,
		file_count INTEGER NOT NULL DEFAULT 0,
		repository_count INTEGER NOT NULL DEFAULT 0,
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		response_tokens INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		entry_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS task_review_findings (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		repository_name TEXT NOT NULL DEFAULT '',
		file_path TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		side TEXT NOT NULL DEFAULT 'additions',
		severity TEXT NOT NULL DEFAULT 'minor',
		category TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		body TEXT NOT NULL DEFAULT '',
		suggestion TEXT NOT NULL DEFAULT '',
		anchor_text TEXT NOT NULL DEFAULT '',
		file_diff_hash TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'open',
		resolved_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (run_id) REFERENCES task_review_runs(id) ON DELETE CASCADE,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
`

func (r *Repository) initTaskReviewSchema() error {
	_, err := r.db.Exec(taskReviewSchemaDDL)
	return err
}
