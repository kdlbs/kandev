CREATE TABLE agent_continuation_summaries (
		agent_profile_id  TEXT NOT NULL,
		scope             TEXT NOT NULL,
		content           TEXT NOT NULL DEFAULT '',
		content_tokens    INTEGER NOT NULL DEFAULT 0,
		updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_by_run_id TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (agent_profile_id, scope)
	);
CREATE TABLE agent_profile_mcp_configs (
		profile_id TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0,
		servers_json TEXT NOT NULL DEFAULT '{}',
		meta_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE
	);
CREATE TABLE agent_profiles (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_display_name TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		mode TEXT DEFAULT NULL,
		migrated_from TEXT DEFAULT NULL,
		auto_approve INTEGER NOT NULL DEFAULT 0,
		dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
		allow_indexing INTEGER NOT NULL DEFAULT 1,
		cli_passthrough INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		user_modified INTEGER NOT NULL DEFAULT 0,
		plan TEXT DEFAULT '',
		cli_flags TEXT DEFAULT NULL,
		env_vars TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		workspace_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		icon TEXT NOT NULL DEFAULT '',
		reports_to TEXT NOT NULL DEFAULT '',
		skill_ids TEXT NOT NULL DEFAULT '[]',
		desired_skills TEXT NOT NULL DEFAULT '[]',
		custom_prompt TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle'
			CHECK (status IN ('idle','working','paused','stopped','pending_approval')),
		pause_reason TEXT NOT NULL DEFAULT '',
		last_run_finished_at TIMESTAMP,
		max_concurrent_sessions INTEGER NOT NULL DEFAULT 1,
		cooldown_sec INTEGER NOT NULL DEFAULT 0,
		skip_idle_runs INTEGER NOT NULL DEFAULT 0,
		consecutive_failures INTEGER NOT NULL DEFAULT 0,
		failure_threshold INTEGER NOT NULL DEFAULT 3,
		executor_preference TEXT NOT NULL DEFAULT '',
		budget_monthly_cents INTEGER NOT NULL DEFAULT 0,
		settings TEXT NOT NULL DEFAULT '{}',
		permissions TEXT NOT NULL DEFAULT '{}',
		command_prefix TEXT NOT NULL DEFAULT '', fallback_model TEXT NOT NULL DEFAULT '', auto_fallback INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	);
CREATE TABLE agent_wakeup_requests (
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
CREATE TABLE agents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		workspace_id TEXT DEFAULT NULL,
		supports_mcp INTEGER NOT NULL DEFAULT 0,
		mcp_config_path TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	, tui_config TEXT DEFAULT NULL);
CREATE TABLE auth_api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			token_sha256 TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME,
			expires_at DATETIME
		);
CREATE TABLE auth_identities (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'local',
			subject TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE (user_id, provider),
			UNIQUE (provider, subject)
		);
CREATE TABLE auth_invites (
			id TEXT PRIMARY KEY,
			token_sha256 TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'member',
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			used_by TEXT,
			used_at DATETIME
		);
CREATE TABLE auth_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_sha256 TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			user_agent TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE custom_prompts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
CREATE TABLE dynamic_agent_profiles (
		profile_id TEXT PRIMARY KEY,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE
	);
CREATE TABLE dynamic_agent_routes (
		dynamic_profile_id TEXT NOT NULL,
		position INTEGER NOT NULL,
		execution_profile_id TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		rules_json TEXT NOT NULL DEFAULT '{}',
		PRIMARY KEY (dynamic_profile_id, position),
		FOREIGN KEY (dynamic_profile_id) REFERENCES dynamic_agent_profiles(profile_id) ON DELETE CASCADE,
		FOREIGN KEY (execution_profile_id) REFERENCES agent_profiles(id) ON DELETE RESTRICT
	);
CREATE TABLE dynamic_installation_keys (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			key_bytes BLOB NOT NULL,
			created_at TIMESTAMP NOT NULL
		);
CREATE TABLE dynamic_resource_circuits (
			resource_key TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			until_at TIMESTAMP,
			code TEXT NOT NULL DEFAULT '',
			probe_until TIMESTAMP,
			updated_at TIMESTAMP NOT NULL
		);
CREATE TABLE dynamic_route_attempts (
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
CREATE TABLE dynamic_route_states (
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
CREATE TABLE editors (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		command TEXT NOT NULL,
		scheme TEXT NOT NULL,
		config TEXT,
		installed INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE environments (
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
CREATE TABLE executor_profiles (
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
CREATE TABLE executors (
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
CREATE TABLE executors_running (
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
	, last_message_uuid TEXT DEFAULT '');
CREATE TABLE kandev_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE notification_deliveries (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		task_session_id TEXT NOT NULL,
		occurrence_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(provider_id, event_type, occurrence_id)
	);
CREATE TABLE notification_migrations (
		name TEXT PRIMARY KEY,
		completed_at TIMESTAMP NOT NULL
	);
CREATE TABLE notification_providers (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE notification_subscriptions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(provider_id, event_type),
		FOREIGN KEY (provider_id) REFERENCES notification_providers(id) ON DELETE CASCADE
	);
CREATE TABLE office_activity_log (
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
		created_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_agent_instructions (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		is_entry INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(agent_profile_id, filename)
	);
CREATE TABLE office_agent_memory (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		layer TEXT NOT NULL,
		key TEXT NOT NULL,
		content TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(agent_profile_id, layer, key)
	);
CREATE TABLE office_agent_runtime (
		agent_id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'idle',
		pause_reason TEXT DEFAULT '',
		last_run_finished_at TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE office_approvals (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		type TEXT NOT NULL,
		requested_by_agent_profile_id TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		payload TEXT DEFAULT '{}',
		decision_note TEXT DEFAULT '',
		decided_by TEXT DEFAULT '',
		decided_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_budget_policies (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		scope_type TEXT NOT NULL,
		scope_id TEXT NOT NULL,
		limit_subcents INTEGER NOT NULL,
		period TEXT NOT NULL,
		alert_threshold_pct INTEGER DEFAULT 80,
		action_on_exceed TEXT DEFAULT 'notify_only',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_channels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		webhook_secret TEXT NOT NULL DEFAULT '',
		status TEXT DEFAULT 'active',
		task_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_cost_events (
		id TEXT PRIMARY KEY,
		session_id TEXT DEFAULT '',
		task_id TEXT DEFAULT '',
		agent_profile_id TEXT DEFAULT '',
		project_id TEXT DEFAULT '',
		model TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		tokens_in INTEGER DEFAULT 0,
		tokens_cached_in INTEGER DEFAULT 0,
		tokens_cached_read INTEGER,
		tokens_cached_write INTEGER,
		tokens_out INTEGER DEFAULT 0,
		cost_subcents INTEGER NOT NULL DEFAULT 0,
		estimated INTEGER NOT NULL DEFAULT 0,
		turn_id TEXT,
		usage_event_id TEXT,
		cost_source TEXT,
		rate_input_per_million INTEGER,
		rate_cached_read_per_million INTEGER,
		rate_cached_write_per_million INTEGER,
		rate_output_per_million INTEGER,
		pricing_catalog_version TEXT,
		cost_contract_version INTEGER,
		occurred_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_inbox_dismissals (
		user_id TEXT NOT NULL,
		item_kind TEXT NOT NULL,
		item_id TEXT NOT NULL,
		dismissed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, item_kind, item_id)
	);
CREATE TABLE office_labels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		color TEXT NOT NULL DEFAULT '#6b7280',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, name)
	);
CREATE TABLE office_onboarding (
		workspace_id TEXT PRIMARY KEY,
		completed INTEGER NOT NULL DEFAULT 0,
		ceo_agent_id TEXT DEFAULT '',
		first_task_id TEXT DEFAULT '',
		completed_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE office_projects (
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
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_provider_health (
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
	);
CREATE TABLE office_routine_runs (
		id TEXT PRIMARY KEY,
		routine_id TEXT NOT NULL,
		trigger_id TEXT DEFAULT '',
		source TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'received',
		trigger_payload TEXT DEFAULT '{}',
		linked_task_id TEXT DEFAULT '',
		coalesced_into_run_id TEXT DEFAULT '',
		dispatch_fingerprint TEXT DEFAULT '',
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
	);
CREATE TABLE office_routine_triggers (
		id TEXT PRIMARY KEY,
		routine_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		cron_expression TEXT DEFAULT '',
		timezone TEXT DEFAULT '',
		public_id TEXT DEFAULT '',
		signing_mode TEXT DEFAULT '',
		secret TEXT DEFAULT '',
		next_run_at TIMESTAMP,
		last_fired_at TIMESTAMP,
		enabled INTEGER DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
	);
CREATE TABLE office_routines (
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
		last_run_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE office_run_route_attempts (
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
	);
CREATE TABLE office_run_skills (
		run_id TEXT NOT NULL,
		skill_id TEXT NOT NULL,
		version TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		materialized_path TEXT NOT NULL,
		PRIMARY KEY (run_id, skill_id)
	);
CREATE TABLE office_skills (
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
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(workspace_id, slug)
	);
CREATE TABLE office_task_labels (
		task_id TEXT NOT NULL,
		label_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (task_id, label_id),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (label_id) REFERENCES office_labels(id) ON DELETE CASCADE
	);
CREATE TABLE office_task_tree_hold_members (
		hold_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		depth INTEGER NOT NULL,
		task_status TEXT NOT NULL DEFAULT '',
		skip_reason TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (hold_id, task_id),
		FOREIGN KEY (hold_id) REFERENCES office_task_tree_holds(id) ON DELETE CASCADE,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
CREATE TABLE office_task_tree_holds (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		root_task_id TEXT NOT NULL,
		mode TEXT NOT NULL,
		release_policy TEXT NOT NULL DEFAULT '{"strategy":"manual"}',
		released_at TIMESTAMP,
		released_by TEXT DEFAULT '',
		released_reason TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (root_task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
CREATE TABLE office_workspace_governance (
		workspace_id TEXT NOT NULL,
		key          TEXT NOT NULL,
		value        INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, key)
	);
CREATE TABLE office_workspace_routing (
		workspace_id      TEXT PRIMARY KEY,
		enabled           INTEGER NOT NULL DEFAULT 0,
		default_tier      TEXT    NOT NULL DEFAULT 'balanced',
		provider_order    TEXT    NOT NULL DEFAULT '[]',
		provider_profiles TEXT    NOT NULL DEFAULT '{}',
		tier_per_reason   TEXT    NOT NULL DEFAULT '{}',
		role_tiers        TEXT    NOT NULL DEFAULT '{}',
		updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE office_workspace_settings (
		workspace_id TEXT PRIMARY KEY,
		agent_failure_threshold INTEGER NOT NULL DEFAULT 3,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE parent_child_wake_receipts (
		parent_task_id        TEXT PRIMARY KEY,
		child_set_key         TEXT NOT NULL,
		delivered_run_id      TEXT NOT NULL DEFAULT '',
		delivery_operation_id TEXT NOT NULL DEFAULT '',
		delivered_at          TIMESTAMP NOT NULL
	);
CREATE TABLE quick_terminal_tabs (
			tab_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			session_id TEXT,
			sequence INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'connecting',
			exit_code INTEGER,
			error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, workspace_id, sequence)
		);
CREATE TABLE quick_terminal_workspace_sequences (
			user_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			next_sequence INTEGER NOT NULL,
			PRIMARY KEY(user_id, workspace_id)
		);
CREATE TABLE repositories (
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
CREATE TABLE repository_branch_policies (
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
CREATE TABLE repository_scripts (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);
CREATE TABLE repository_secret_bindings (
		repository_id TEXT NOT NULL,
		key TEXT NOT NULL,
		secret_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (repository_id, key),
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);
CREATE TABLE repository_set_items (
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
CREATE TABLE repository_sets (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
		UNIQUE(workspace_id, name)
	);
CREATE TABLE run_events (
		run_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		payload TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (run_id, seq)
	);
CREATE TABLE runs (
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
		scheduled_retry_at TIMESTAMP,
		error_message TEXT NOT NULL DEFAULT '',
		cancel_reason TEXT,
		-- outcome (task-delivery-ledger): nullable, one of the eight
		-- Office run outcome values on the finished path, NULL on failed
		-- and on every pre-activation row. See migrateRunOutcome for the
		-- ADD COLUMN that converges existing databases.
		outcome TEXT,
		-- Provider routing (office-provider-routing).
		logical_provider_order TEXT,
		requested_tier TEXT,
		resolved_execution_profile_id TEXT,
		resolved_provider_id TEXT,
		resolved_model TEXT,
		current_route_attempt_seq INTEGER NOT NULL DEFAULT 0,
		routing_blocked_status TEXT,
		earliest_retry_at TIMESTAMP,
		-- route_cycle_baseline_seq marks the floor at which the current
		-- retry cycle began. excludedFromAttempts filters prior attempt
		-- rows with seq <= baseline so a parked-then-lifted run gets a
		-- fresh exclusion list instead of re-inheriting every provider
		-- that failed in the previous cycle.
		route_cycle_baseline_seq INTEGER NOT NULL DEFAULT 0,
		-- Heartbeat-rework run inspection columns: structured adapter
		-- output, the assembled prompt the agent received, and the
		-- continuation summary that was prepended (if any).
		result_json TEXT NOT NULL DEFAULT '{}',
		assembled_prompt TEXT NOT NULL DEFAULT '',
		summary_injected TEXT NOT NULL DEFAULT '',
		-- continuation_scope is the continuation-summary scope key
		-- (models.ContinuationScopeForRun) decided once at run creation
		-- and persisted so every later reader/writer of this run's
		-- continuation summary uses the same value instead of
		-- re-deriving it against a context_snapshot a coalesced wakeup
		-- may have since patched.
		continuation_scope TEXT NOT NULL DEFAULT '',
		requested_at TIMESTAMP NOT NULL,
		claimed_at TIMESTAMP,
		finished_at TIMESTAMP
	);
CREATE TABLE runtime_flag_overrides (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL CHECK (value IN (0, 1)),
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE secrets (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		user_id         TEXT NOT NULL DEFAULT '',
		scope           TEXT NOT NULL DEFAULT 'global',
		workspace_id    TEXT NOT NULL DEFAULT '',
		encrypted_value BLOB NOT NULL,
		nonce           BLOB NOT NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE session_file_reviews (
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
CREATE TABLE session_step_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		from_step_id TEXT,
		to_step_id TEXT NOT NULL,
		trigger TEXT NOT NULL,
		actor_id TEXT,
		metadata TEXT,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
CREATE TABLE task_blockers (
		task_id TEXT NOT NULL,
		blocker_task_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (task_id, blocker_task_id),
		CHECK (task_id != blocker_task_id)
	);
CREATE TABLE task_comments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		author_type TEXT NOT NULL,
		author_id TEXT NOT NULL,
		body TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'user',
		reply_channel_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL
	);
CREATE TABLE task_document_revisions (
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
CREATE TABLE task_documents (
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
CREATE TABLE task_environment_repos (
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
CREATE TABLE task_environments (
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
CREATE TABLE task_message_attachments (
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
CREATE TABLE task_plan_revisions (
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
CREATE TABLE task_plans (
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
CREATE TABLE task_repositories (
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
CREATE TABLE task_resource_cleanup_jobs (
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
CREATE TABLE task_review_findings (
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
CREATE TABLE task_review_runs (
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
CREATE TABLE task_session_commits (
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
CREATE TABLE task_session_git_snapshots (
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
CREATE TABLE task_session_messages (
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
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, prompt_seq INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE
	);
CREATE TABLE task_session_prompt_seq (
			task_session_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL
		);
CREATE TABLE task_session_subagents (
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
CREATE TABLE task_session_turns (
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
CREATE TABLE "task_sessions" (
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
			tokens_out INTEGER NOT NULL DEFAULT 0, workspace_path TEXT DEFAULT '', name TEXT DEFAULT '', last_read_message_id TEXT DEFAULT '',
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
CREATE TABLE task_status_summaries (
		task_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		revision INTEGER NOT NULL DEFAULT 0,
		summary TEXT NOT NULL DEFAULT '{}',
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
CREATE TABLE task_step_transitions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
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
CREATE TABLE task_usage_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		usage_event_id TEXT NOT NULL,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
		turn_id TEXT,
		agent_profile_id TEXT NOT NULL DEFAULT '',
		agent_type TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		tokens_in BIGINT NOT NULL DEFAULT 0,
		tokens_cached_read BIGINT,
		tokens_cached_write BIGINT,
		tokens_out BIGINT,
		tokens_thought BIGINT,
		tokens_total BIGINT NOT NULL DEFAULT 0,
		cost_subcents BIGINT NOT NULL DEFAULT 0,
		cost_source TEXT NOT NULL,
		estimated INTEGER NOT NULL DEFAULT 0,
		rate_input_per_million BIGINT,
		rate_cached_read_per_million BIGINT,
		rate_cached_write_per_million BIGINT,
		rate_output_per_million BIGINT,
		pricing_catalog_version TEXT,
		contract_version INTEGER NOT NULL,
		occurred_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
CREATE TABLE task_walkthroughs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL DEFAULT 'Walkthrough',
		steps TEXT NOT NULL DEFAULT '[]',
		created_by TEXT NOT NULL DEFAULT 'agent',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
CREATE TABLE task_workspace_folders (
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
CREATE TABLE task_workspace_group_members (
		workspace_group_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		released_at TIMESTAMP,
		release_reason TEXT NOT NULL DEFAULT '',
		released_by_cascade_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (workspace_group_id, task_id),
		FOREIGN KEY (workspace_group_id) REFERENCES task_workspace_groups(id) ON DELETE CASCADE
	);
CREATE TABLE task_workspace_groups (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		owner_task_id TEXT NOT NULL,
		materialized_path TEXT NOT NULL DEFAULT '',
		materialized_environment_id TEXT NOT NULL DEFAULT '',
		materialized_kind TEXT NOT NULL,
		owned_by_kandev INTEGER NOT NULL DEFAULT 0,
		cleanup_policy TEXT NOT NULL DEFAULT 'never_delete',
		cleanup_status TEXT NOT NULL DEFAULT 'active',
		cleaned_at TIMESTAMP,
		cleanup_error TEXT NOT NULL DEFAULT '',
		restore_status TEXT NOT NULL DEFAULT 'not_needed',
		restore_error TEXT NOT NULL DEFAULT '',
		restore_config_json TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE "tasks" (
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
		);
CREATE TABLE telemetry_activations (
			contract_key     TEXT      NOT NULL,
			contract_version INTEGER   NOT NULL,
			activated_at     TIMESTAMP NOT NULL,
			PRIMARY KEY (contract_key, contract_version)
		);
CREATE TABLE user_agent_profile_recent_use (
		user_id TEXT NOT NULL,
		context TEXT NOT NULL,
		profile_ids TEXT NOT NULL DEFAULT '[]',
		revision BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (user_id, context),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
CREATE TABLE user_terminals (
			id              TEXT PRIMARY KEY,
			task_id         TEXT NOT NULL,
			environment_id  TEXT NOT NULL,
			seq             INTEGER NOT NULL,
			custom_name     TEXT,
			state           TEXT NOT NULL DEFAULT 'open',
			initial_command TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id, seq)
		);
CREATE TABLE users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'admin',
		status TEXT NOT NULL DEFAULT 'active',
		settings TEXT NOT NULL DEFAULT '{}',
		settings_revision BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE utility_agent_calls (
			id TEXT PRIMARY KEY,
			utility_id TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			resolved_prompt TEXT NOT NULL DEFAULT '',
			response TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			execution_profile_id TEXT NOT NULL DEFAULT '',
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			response_tokens INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			FOREIGN KEY (utility_id) REFERENCES utility_agents(id) ON DELETE CASCADE
		);
CREATE TABLE utility_agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT 'claude-acp',
			model TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			profile_binding_state TEXT NOT NULL DEFAULT 'explicit',
			builtin INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
CREATE TABLE workflow_step_decisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		participant_id TEXT NOT NULL,
		decision TEXT NOT NULL,
		note TEXT DEFAULT '',
		decided_at TIMESTAMP NOT NULL,
		superseded_at TIMESTAMP NULL,
		decider_type TEXT NOT NULL DEFAULT '',
		decider_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT ''
	);
CREATE TABLE workflow_step_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		step_id TEXT NOT NULL,
		entry_seq INTEGER NOT NULL,
		digest TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(task_id, step_id, entry_seq)
	);
CREATE TABLE workflow_step_entry_markers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entry_id INTEGER NOT NULL REFERENCES workflow_step_entries(id) ON DELETE CASCADE,
		position INTEGER NOT NULL,
		kind TEXT NOT NULL,
		state TEXT NOT NULL,
		operation_id TEXT,
		error TEXT,
		claimed_at TIMESTAMP,
		completed_at TIMESTAMP,
		CONSTRAINT uniq_workflow_step_entry_markers_entry_position UNIQUE(entry_id, position)
	);
CREATE TABLE workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0
		, created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00', provenance TEXT NOT NULL DEFAULT 'manual');
CREATE TABLE workflow_steps (
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
		, wip_limit INTEGER NOT NULL DEFAULT 0, pull_from_step_id TEXT NOT NULL DEFAULT '');
CREATE TABLE workflow_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		is_system INTEGER DEFAULT 0,
		steps TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
CREATE TABLE workflows (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		hidden INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	, sort_order INTEGER NOT NULL DEFAULT 0, agent_profile_id TEXT DEFAULT '', is_system INTEGER DEFAULT 0, style TEXT NOT NULL DEFAULT 'kanban', source TEXT NOT NULL DEFAULT 'manual', source_path TEXT NOT NULL DEFAULT '');
CREATE TABLE workspaces (
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
	, task_prefix TEXT DEFAULT 'KAN', task_sequence INTEGER DEFAULT 0, office_workflow_id TEXT DEFAULT '');
INSERT INTO kandev_meta (key, value)
VALUES ('fixture.sentinel', 'v0.93.0-sqlite-sentinel')
ON CONFLICT (key) DO UPDATE SET value = excluded.value;
