






CREATE TABLE agent_continuation_summaries (
    agent_profile_id text NOT NULL,
    scope text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    content_tokens integer DEFAULT 0 NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by_run_id text DEFAULT ''::text NOT NULL
);



CREATE TABLE agent_profile_mcp_configs (
    profile_id text NOT NULL,
    enabled integer DEFAULT 0 NOT NULL,
    servers_json text DEFAULT '{}'::text NOT NULL,
    meta_json text DEFAULT '{}'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE agent_profiles (
    id text NOT NULL,
    agent_id text NOT NULL,
    name text NOT NULL,
    agent_display_name text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    mode text,
    migrated_from text,
    auto_approve integer DEFAULT 0 NOT NULL,
    dangerously_skip_permissions integer DEFAULT 0 NOT NULL,
    allow_indexing integer DEFAULT 1 NOT NULL,
    cli_passthrough integer DEFAULT 0 NOT NULL,
    enabled integer DEFAULT 1 NOT NULL,
    user_modified integer DEFAULT 0 NOT NULL,
    plan text DEFAULT ''::text,
    cli_flags text,
    env_vars text DEFAULT '[]'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    deleted_at timestamp without time zone,
    workspace_id text DEFAULT ''::text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    reports_to text DEFAULT ''::text NOT NULL,
    skill_ids text DEFAULT '[]'::text NOT NULL,
    desired_skills text DEFAULT '[]'::text NOT NULL,
    custom_prompt text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'idle'::text NOT NULL,
    pause_reason text DEFAULT ''::text NOT NULL,
    last_run_finished_at timestamp without time zone,
    max_concurrent_sessions integer DEFAULT 1 NOT NULL,
    cooldown_sec integer DEFAULT 0 NOT NULL,
    skip_idle_runs integer DEFAULT 0 NOT NULL,
    consecutive_failures integer DEFAULT 0 NOT NULL,
    failure_threshold integer DEFAULT 3 NOT NULL,
    executor_preference text DEFAULT ''::text NOT NULL,
    budget_monthly_cents integer DEFAULT 0 NOT NULL,
    settings text DEFAULT '{}'::text NOT NULL,
    permissions text DEFAULT '{}'::text NOT NULL,
    command_prefix text DEFAULT ''::text NOT NULL,
    fallback_model text DEFAULT ''::text NOT NULL,
    auto_fallback integer DEFAULT 0 NOT NULL,
    CONSTRAINT agent_profiles_status_check CHECK ((status = ANY (ARRAY['idle'::text, 'working'::text, 'paused'::text, 'stopped'::text, 'pending_approval'::text])))
);



CREATE TABLE agent_wakeup_requests (
    id text NOT NULL,
    agent_profile_id text NOT NULL,
    source text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    payload text DEFAULT '{}'::text NOT NULL,
    status text NOT NULL,
    coalesced_count integer DEFAULT 1 NOT NULL,
    idempotency_key text,
    run_id text DEFAULT ''::text NOT NULL,
    requested_at timestamp without time zone NOT NULL,
    claimed_at timestamp without time zone,
    finished_at timestamp without time zone
);



CREATE TABLE agents (
    id text NOT NULL,
    name text NOT NULL,
    workspace_id text,
    supports_mcp integer DEFAULT 0 NOT NULL,
    mcp_config_path text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    tui_config text
);



CREATE TABLE auth_api_tokens (
    id text NOT NULL,
    user_id text NOT NULL,
    name text NOT NULL,
    prefix text NOT NULL,
    token_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    expires_at timestamp with time zone
);



CREATE TABLE auth_identities (
    id text NOT NULL,
    user_id text NOT NULL,
    provider text DEFAULT 'local'::text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    password_hash text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);



CREATE TABLE auth_invites (
    id text NOT NULL,
    token_sha256 text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_by text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_by text,
    used_at timestamp with time zone
);



CREATE TABLE auth_sessions (
    id text NOT NULL,
    user_id text NOT NULL,
    token_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL,
    ip text DEFAULT ''::text NOT NULL
);



CREATE TABLE custom_prompts (
    id text NOT NULL,
    name text NOT NULL,
    content text NOT NULL,
    builtin integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE dynamic_agent_profiles (
    profile_id text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE dynamic_agent_routes (
    dynamic_profile_id text NOT NULL,
    "position" integer NOT NULL,
    execution_profile_id text NOT NULL,
    enabled integer DEFAULT 1 NOT NULL,
    rules_json text DEFAULT '{}'::text NOT NULL
);



CREATE TABLE dynamic_installation_keys (
    id integer NOT NULL,
    key_bytes bytea NOT NULL,
    created_at timestamp without time zone NOT NULL,
    CONSTRAINT dynamic_installation_keys_id_check CHECK ((id = 1))
);



CREATE TABLE dynamic_resource_circuits (
    resource_key text NOT NULL,
    state text NOT NULL,
    until_at timestamp without time zone,
    code text DEFAULT ''::text NOT NULL,
    probe_until timestamp without time zone,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE dynamic_route_attempts (
    id text NOT NULL,
    session_id text NOT NULL,
    logical_profile_id text NOT NULL,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    route_generation bigint NOT NULL,
    profile_version bigint NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE dynamic_route_states (
    session_id text NOT NULL,
    logical_profile_id text NOT NULL,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    route_generation bigint DEFAULT 0 NOT NULL,
    profile_version bigint DEFAULT 0 NOT NULL,
    state text DEFAULT 'selecting'::text NOT NULL,
    continuation_json text DEFAULT ''::text NOT NULL,
    policy_state_json text DEFAULT ''::text NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE editors (
    id text NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    command text NOT NULL,
    scheme text NOT NULL,
    config text,
    installed integer DEFAULT 0 NOT NULL,
    enabled integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE environments (
    id text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    is_system integer DEFAULT 0 NOT NULL,
    worktree_root text DEFAULT ''::text,
    image_tag text DEFAULT ''::text,
    dockerfile text DEFAULT ''::text,
    build_config text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    deleted_at timestamp without time zone
);



CREATE TABLE executor_profiles (
    id text NOT NULL,
    executor_id text NOT NULL,
    name text NOT NULL,
    mcp_policy text DEFAULT ''::text,
    config text DEFAULT '{}'::text,
    prepare_script text DEFAULT ''::text,
    cleanup_script text DEFAULT ''::text,
    env_vars text DEFAULT '[]'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE executors (
    id text NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    is_system integer DEFAULT 0 NOT NULL,
    resumable integer DEFAULT 1 NOT NULL,
    config text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    deleted_at timestamp without time zone
);



CREATE TABLE executors_running (
    id text NOT NULL,
    session_id text NOT NULL,
    task_id text NOT NULL,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    executor_id text NOT NULL,
    runtime text DEFAULT ''::text,
    status text DEFAULT 'starting'::text NOT NULL,
    resumable integer DEFAULT 0 NOT NULL,
    resume_token text DEFAULT ''::text,
    agent_execution_id text DEFAULT ''::text,
    container_id text DEFAULT ''::text,
    agentctl_url text DEFAULT ''::text,
    agentctl_port integer DEFAULT 0,
    pid integer DEFAULT 0,
    local_pid integer DEFAULT 0,
    worktree_id text DEFAULT ''::text,
    worktree_path text DEFAULT ''::text,
    worktree_branch text DEFAULT ''::text,
    last_seen_at timestamp without time zone,
    error_message text DEFAULT ''::text,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    last_message_uuid text DEFAULT ''::text
);



CREATE TABLE kandev_meta (
    key text NOT NULL,
    value text DEFAULT ''::text NOT NULL
);



CREATE TABLE notification_deliveries (
    id text NOT NULL,
    user_id text NOT NULL,
    provider_id text NOT NULL,
    event_type text NOT NULL,
    task_session_id text NOT NULL,
    occurrence_id text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE notification_migrations (
    name text NOT NULL,
    completed_at timestamp without time zone NOT NULL
);



CREATE TABLE notification_providers (
    id text NOT NULL,
    user_id text NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    config text DEFAULT '{}'::text,
    enabled integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE notification_subscriptions (
    id text NOT NULL,
    user_id text NOT NULL,
    provider_id text NOT NULL,
    event_type text NOT NULL,
    enabled integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_activity_log (
    id text NOT NULL,
    workspace_id text NOT NULL,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    action text NOT NULL,
    target_type text DEFAULT ''::text,
    target_id text DEFAULT ''::text,
    details text DEFAULT '{}'::text,
    run_id text DEFAULT ''::text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE office_agent_instructions (
    id text NOT NULL,
    agent_profile_id text NOT NULL,
    filename text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    is_entry integer DEFAULT 0,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);



CREATE TABLE office_agent_memory (
    id text NOT NULL,
    agent_profile_id text NOT NULL,
    layer text NOT NULL,
    key text NOT NULL,
    content text DEFAULT ''::text,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_agent_runtime (
    agent_id text NOT NULL,
    status text DEFAULT 'idle'::text NOT NULL,
    pause_reason text DEFAULT ''::text,
    last_run_finished_at timestamp without time zone,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);



CREATE TABLE office_approvals (
    id text NOT NULL,
    workspace_id text NOT NULL,
    type text NOT NULL,
    requested_by_agent_profile_id text DEFAULT ''::text,
    status text DEFAULT 'pending'::text NOT NULL,
    payload text DEFAULT '{}'::text,
    decision_note text DEFAULT ''::text,
    decided_by text DEFAULT ''::text,
    decided_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_budget_policies (
    id text NOT NULL,
    workspace_id text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL,
    limit_subcents integer NOT NULL,
    period text NOT NULL,
    alert_threshold_pct integer DEFAULT 80,
    action_on_exceed text DEFAULT 'notify_only'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_channels (
    id text NOT NULL,
    workspace_id text NOT NULL,
    agent_profile_id text NOT NULL,
    platform text NOT NULL,
    config text DEFAULT '{}'::text,
    webhook_secret text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text,
    task_id text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_cost_events (
    id text NOT NULL,
    session_id text DEFAULT ''::text,
    task_id text DEFAULT ''::text,
    agent_profile_id text DEFAULT ''::text,
    project_id text DEFAULT ''::text,
    model text DEFAULT ''::text,
    provider text DEFAULT ''::text,
    tokens_in integer DEFAULT 0,
    tokens_cached_in integer DEFAULT 0,
    tokens_cached_read integer,
    tokens_cached_write integer,
    tokens_out integer DEFAULT 0,
    cost_subcents integer DEFAULT 0 NOT NULL,
    estimated integer DEFAULT 0 NOT NULL,
    turn_id text,
    usage_event_id text,
    cost_source text,
    rate_input_per_million integer,
    rate_cached_read_per_million integer,
    rate_cached_write_per_million integer,
    rate_output_per_million integer,
    pricing_catalog_version text,
    cost_contract_version integer,
    occurred_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE office_inbox_dismissals (
    user_id text NOT NULL,
    item_kind text NOT NULL,
    item_id text NOT NULL,
    dismissed_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE office_labels (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    color text DEFAULT '#6b7280'::text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE office_onboarding (
    workspace_id text NOT NULL,
    completed integer DEFAULT 0 NOT NULL,
    ceo_agent_id text DEFAULT ''::text,
    first_task_id text DEFAULT ''::text,
    completed_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);



CREATE TABLE office_projects (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    status text DEFAULT 'active'::text NOT NULL,
    lead_agent_profile_id text DEFAULT ''::text,
    color text DEFAULT ''::text,
    budget_cents integer DEFAULT 0,
    repositories text DEFAULT '[]'::text,
    executor_config text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_provider_health (
    workspace_id text NOT NULL,
    provider_id text NOT NULL,
    scope text NOT NULL,
    scope_value text NOT NULL,
    state text NOT NULL,
    error_code text,
    retry_at timestamp without time zone,
    backoff_step integer DEFAULT 0 NOT NULL,
    last_failure timestamp without time zone,
    last_success timestamp without time zone,
    raw_excerpt text,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE office_routine_runs (
    id text NOT NULL,
    routine_id text NOT NULL,
    trigger_id text DEFAULT ''::text,
    source text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    trigger_payload text DEFAULT '{}'::text,
    linked_task_id text DEFAULT ''::text,
    coalesced_into_run_id text DEFAULT ''::text,
    dispatch_fingerprint text DEFAULT ''::text,
    started_at timestamp without time zone,
    completed_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE office_routine_triggers (
    id text NOT NULL,
    routine_id text NOT NULL,
    kind text NOT NULL,
    cron_expression text DEFAULT ''::text,
    timezone text DEFAULT ''::text,
    public_id text DEFAULT ''::text,
    signing_mode text DEFAULT ''::text,
    secret text DEFAULT ''::text,
    next_run_at timestamp without time zone,
    last_fired_at timestamp without time zone,
    enabled integer DEFAULT 1,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_routines (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    task_template text DEFAULT '{}'::text NOT NULL,
    assignee_agent_profile_id text DEFAULT ''::text,
    status text DEFAULT 'active'::text NOT NULL,
    concurrency_policy text DEFAULT 'skip_if_active'::text,
    catch_up_policy text DEFAULT 'enqueue_missed_with_cap'::text NOT NULL,
    catch_up_max integer DEFAULT 25 NOT NULL,
    variables text DEFAULT '{}'::text,
    last_run_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_run_route_attempts (
    run_id text NOT NULL,
    seq integer NOT NULL,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    provider_id text NOT NULL,
    model text NOT NULL,
    tier text NOT NULL,
    tier_source text DEFAULT ''::text NOT NULL,
    outcome text NOT NULL,
    error_code text,
    error_confidence text,
    adapter_phase text,
    classifier_rule text,
    exit_code integer,
    raw_excerpt text,
    reset_hint timestamp without time zone,
    started_at timestamp without time zone NOT NULL,
    finished_at timestamp without time zone
);



CREATE TABLE office_run_skills (
    run_id text NOT NULL,
    skill_id text NOT NULL,
    version text NOT NULL,
    content_hash text NOT NULL,
    materialized_path text NOT NULL
);



CREATE TABLE office_skills (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text DEFAULT ''::text,
    source_type text DEFAULT 'inline'::text NOT NULL,
    source_locator text DEFAULT ''::text,
    content text DEFAULT ''::text,
    file_inventory text DEFAULT '[]'::text,
    version text DEFAULT ''::text NOT NULL,
    content_hash text DEFAULT ''::text NOT NULL,
    approval_state text DEFAULT 'approved'::text NOT NULL,
    created_by_agent_profile_id text DEFAULT ''::text,
    is_system integer DEFAULT 0 NOT NULL,
    system_version text DEFAULT ''::text NOT NULL,
    default_for_roles text DEFAULT '[]'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE office_task_labels (
    task_id text NOT NULL,
    label_id text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE office_task_tree_hold_members (
    hold_id text NOT NULL,
    task_id text NOT NULL,
    depth integer NOT NULL,
    task_status text DEFAULT ''::text NOT NULL,
    skip_reason text DEFAULT ''::text NOT NULL
);



CREATE TABLE office_task_tree_holds (
    id text NOT NULL,
    workspace_id text NOT NULL,
    root_task_id text NOT NULL,
    mode text NOT NULL,
    release_policy text DEFAULT '{"strategy":"manual"}'::text NOT NULL,
    released_at timestamp without time zone,
    released_by text DEFAULT ''::text,
    released_reason text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE office_workspace_governance (
    workspace_id text NOT NULL,
    key text NOT NULL,
    value integer DEFAULT 0 NOT NULL
);



CREATE TABLE office_workspace_routing (
    workspace_id text NOT NULL,
    enabled integer DEFAULT 0 NOT NULL,
    default_tier text DEFAULT 'balanced'::text NOT NULL,
    provider_order text DEFAULT '[]'::text NOT NULL,
    provider_profiles text DEFAULT '{}'::text NOT NULL,
    tier_per_reason text DEFAULT '{}'::text NOT NULL,
    role_tiers text DEFAULT '{}'::text NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE office_workspace_settings (
    workspace_id text NOT NULL,
    agent_failure_threshold integer DEFAULT 3 NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE parent_child_wake_receipts (
    parent_task_id text NOT NULL,
    child_set_key text NOT NULL,
    delivered_run_id text DEFAULT ''::text NOT NULL,
    delivery_operation_id text DEFAULT ''::text NOT NULL,
    delivered_at timestamp without time zone NOT NULL
);



CREATE TABLE quick_terminal_tabs (
    tab_id text NOT NULL,
    user_id text NOT NULL,
    workspace_id text NOT NULL,
    session_id text,
    sequence integer NOT NULL,
    status text DEFAULT 'connecting'::text NOT NULL,
    exit_code integer,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE quick_terminal_workspace_sequences (
    user_id text NOT NULL,
    workspace_id text NOT NULL,
    next_sequence integer NOT NULL
);



CREATE TABLE repositories (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    source_type text DEFAULT 'local'::text NOT NULL,
    local_path text DEFAULT ''::text,
    provider text DEFAULT ''::text,
    provider_repo_id text DEFAULT ''::text,
    provider_host text DEFAULT ''::text,
    provider_scope text DEFAULT ''::text,
    provider_owner text DEFAULT ''::text,
    provider_name text DEFAULT ''::text,
    remote_url text DEFAULT ''::text,
    default_branch text DEFAULT ''::text,
    worktree_branch_prefix text DEFAULT 'feature/'::text,
    worktree_branch_template text DEFAULT 'feature/{title}-{suffix}'::text,
    pull_before_worktree integer DEFAULT 1 NOT NULL,
    setup_script text DEFAULT ''::text,
    cleanup_script text DEFAULT ''::text,
    dev_script text DEFAULT ''::text,
    copy_files text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    deleted_at timestamp without time zone
);



CREATE TABLE repository_branch_policies (
    id text NOT NULL,
    repository_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    base_branch text NOT NULL,
    branch_template text NOT NULL,
    pull_request_target text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE repository_scripts (
    id text NOT NULL,
    repository_id text NOT NULL,
    name text NOT NULL,
    command text NOT NULL,
    "position" integer DEFAULT 0,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE repository_secret_bindings (
    repository_id text NOT NULL,
    key text NOT NULL,
    secret_id text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE repository_set_items (
    id text NOT NULL,
    repository_set_id text NOT NULL,
    repository_id text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE repository_sets (
    id text NOT NULL,
    workspace_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE run_events (
    run_id text NOT NULL,
    seq integer NOT NULL,
    event_type text NOT NULL,
    level text DEFAULT 'info'::text NOT NULL,
    payload text DEFAULT '{}'::text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE runs (
    id text NOT NULL,
    agent_profile_id text NOT NULL,
    reason text NOT NULL,
    payload text DEFAULT '{}'::text,
    status text DEFAULT 'queued'::text NOT NULL,
    coalesced_count integer DEFAULT 1,
    idempotency_key text,
    context_snapshot text DEFAULT '{}'::text,
    capabilities text DEFAULT '{}'::text NOT NULL,
    input_snapshot text DEFAULT '{}'::text NOT NULL,
    output_summary text DEFAULT ''::text NOT NULL,
    failure_reason text DEFAULT ''::text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    retry_count integer DEFAULT 0,
    scheduled_retry_at timestamp without time zone,
    error_message text DEFAULT ''::text NOT NULL,
    cancel_reason text,
    outcome text,
    logical_provider_order text,
    requested_tier text,
    resolved_execution_profile_id text,
    resolved_provider_id text,
    resolved_model text,
    current_route_attempt_seq integer DEFAULT 0 NOT NULL,
    routing_blocked_status text,
    earliest_retry_at timestamp without time zone,
    route_cycle_baseline_seq integer DEFAULT 0 NOT NULL,
    result_json text DEFAULT '{}'::text NOT NULL,
    assembled_prompt text DEFAULT ''::text NOT NULL,
    summary_injected text DEFAULT ''::text NOT NULL,
    continuation_scope text DEFAULT ''::text NOT NULL,
    requested_at timestamp without time zone NOT NULL,
    claimed_at timestamp without time zone,
    finished_at timestamp without time zone
);



CREATE TABLE runtime_flag_overrides (
    key text NOT NULL,
    value integer NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT runtime_flag_overrides_value_check CHECK ((value = ANY (ARRAY[0, 1])))
);



CREATE TABLE secrets (
    id text NOT NULL,
    name text NOT NULL,
    user_id text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'global'::text NOT NULL,
    workspace_id text DEFAULT ''::text NOT NULL,
    encrypted_value bytea NOT NULL,
    nonce bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE session_file_reviews (
    id text NOT NULL,
    session_id text NOT NULL,
    file_path text NOT NULL,
    reviewed integer DEFAULT 0 NOT NULL,
    diff_hash text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE session_step_history (
    id bigint NOT NULL,
    session_id text NOT NULL,
    from_step_id text,
    to_step_id text NOT NULL,
    trigger text NOT NULL,
    actor_id text,
    metadata text,
    created_at timestamp without time zone NOT NULL
);



CREATE SEQUENCE session_step_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE session_step_history_id_seq OWNED BY session_step_history.id;



CREATE TABLE task_blockers (
    task_id text NOT NULL,
    blocker_task_id text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    CONSTRAINT task_blockers_check CHECK ((task_id <> blocker_task_id))
);



CREATE TABLE task_comments (
    id text NOT NULL,
    task_id text NOT NULL,
    author_type text NOT NULL,
    author_id text NOT NULL,
    body text NOT NULL,
    source text DEFAULT 'user'::text NOT NULL,
    reply_channel_id text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE task_document_revisions (
    id text NOT NULL,
    task_id text NOT NULL,
    document_key text DEFAULT 'plan'::text NOT NULL,
    revision_number integer NOT NULL,
    title text DEFAULT 'Plan'::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    author_kind text DEFAULT 'agent'::text NOT NULL,
    author_name text DEFAULT ''::text NOT NULL,
    revert_of_revision_id text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_documents (
    id text NOT NULL,
    task_id text NOT NULL,
    key text DEFAULT 'plan'::text NOT NULL,
    type text DEFAULT 'plan'::text NOT NULL,
    title text DEFAULT 'Plan'::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    author_kind text DEFAULT 'agent'::text NOT NULL,
    author_name text DEFAULT ''::text NOT NULL,
    filename text DEFAULT ''::text NOT NULL,
    mime_type text DEFAULT ''::text NOT NULL,
    size_bytes integer DEFAULT 0 NOT NULL,
    disk_path text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_environment_repos (
    id text NOT NULL,
    task_environment_id text NOT NULL,
    repository_id text NOT NULL,
    branch_slug text DEFAULT ''::text NOT NULL,
    worktree_id text DEFAULT ''::text,
    worktree_path text DEFAULT ''::text,
    worktree_branch text DEFAULT ''::text,
    "position" integer DEFAULT 0,
    error_message text DEFAULT ''::text,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    merged_at timestamp without time zone,
    deleted_at timestamp without time zone
);



CREATE TABLE task_environments (
    id text NOT NULL,
    task_id text NOT NULL,
    executor_type text DEFAULT ''::text NOT NULL,
    executor_id text DEFAULT ''::text,
    executor_profile_id text DEFAULT ''::text,
    control_port integer DEFAULT 0,
    status text DEFAULT 'creating'::text NOT NULL,
    materialization_session_id text DEFAULT ''::text,
    workspace_path text DEFAULT ''::text,
    container_id text DEFAULT ''::text,
    container_bootstrap_nonce_secret_id text DEFAULT ''::text,
    container_control_auth_token_secret_id text DEFAULT ''::text,
    sandbox_id text DEFAULT ''::text,
    task_dir_name text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_message_attachments (
    id text NOT NULL,
    owner_id text DEFAULT ''::text NOT NULL,
    workspace_id text NOT NULL,
    task_id text DEFAULT ''::text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    message_id text DEFAULT ''::text NOT NULL,
    queue_id text DEFAULT ''::text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    mime_type text DEFAULT 'application/octet-stream'::text NOT NULL,
    kind text DEFAULT 'resource'::text NOT NULL,
    delivery_mode text DEFAULT 'prompt'::text NOT NULL,
    size_bytes integer NOT NULL,
    storage_key text NOT NULL,
    state text DEFAULT 'staged'::text NOT NULL,
    expires_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_plan_revisions (
    id text NOT NULL,
    task_id text NOT NULL,
    revision_number integer NOT NULL,
    title text DEFAULT 'Plan'::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    author_kind text DEFAULT 'agent'::text NOT NULL,
    author_name text DEFAULT ''::text NOT NULL,
    revert_of_revision_id text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_plans (
    id text NOT NULL,
    task_id text NOT NULL,
    title text DEFAULT 'Plan'::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    created_by text DEFAULT 'agent'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    implementation_started_at timestamp without time zone,
    implementation_started_session_id text,
    implementation_started_by text
);



CREATE TABLE task_repositories (
    id text NOT NULL,
    task_id text NOT NULL,
    repository_id text NOT NULL,
    base_branch text DEFAULT ''::text,
    checkout_branch text DEFAULT ''::text,
    branch_policy_id text DEFAULT ''::text,
    branch_policy_name text DEFAULT ''::text,
    branch_policy_base_branch text DEFAULT ''::text,
    branch_policy_branch_template text DEFAULT ''::text,
    branch_policy_pull_request_target text DEFAULT ''::text,
    "position" integer DEFAULT 0,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_resource_cleanup_jobs (
    id text NOT NULL,
    operation_id text NOT NULL,
    task_id text NOT NULL,
    trigger text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    resource_snapshot text DEFAULT '{}'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp without time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    completed_at timestamp without time zone
);



CREATE TABLE task_review_findings (
    id text NOT NULL,
    run_id text NOT NULL,
    task_id text NOT NULL,
    repository_id text DEFAULT ''::text NOT NULL,
    repository_name text DEFAULT ''::text NOT NULL,
    file_path text NOT NULL,
    start_line integer NOT NULL,
    end_line integer NOT NULL,
    side text DEFAULT 'additions'::text NOT NULL,
    severity text DEFAULT 'minor'::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    title text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    suggestion text DEFAULT ''::text NOT NULL,
    anchor_text text DEFAULT ''::text NOT NULL,
    file_diff_hash text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    resolved_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_review_runs (
    id text NOT NULL,
    task_id text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    trigger text DEFAULT 'manual'::text NOT NULL,
    workflow_step_id text DEFAULT ''::text NOT NULL,
    agent_id text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    finding_count integer DEFAULT 0 NOT NULL,
    file_count integer DEFAULT 0 NOT NULL,
    repository_count integer DEFAULT 0 NOT NULL,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    response_tokens integer DEFAULT 0 NOT NULL,
    duration_ms integer DEFAULT 0 NOT NULL,
    entry_id text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    completed_at timestamp without time zone
);



CREATE TABLE task_session_commits (
    id text NOT NULL,
    session_id text NOT NULL,
    commit_sha text NOT NULL,
    parent_sha text DEFAULT ''::text,
    author_name text DEFAULT ''::text,
    author_email text DEFAULT ''::text,
    commit_message text DEFAULT ''::text,
    committed_at timestamp without time zone NOT NULL,
    pre_commit_snapshot_id text DEFAULT ''::text,
    post_commit_snapshot_id text DEFAULT ''::text,
    files_changed integer DEFAULT 0,
    insertions integer DEFAULT 0,
    deletions integer DEFAULT 0,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE task_session_git_snapshots (
    id text NOT NULL,
    task_environment_id text NOT NULL,
    session_id text,
    snapshot_type text NOT NULL,
    branch text NOT NULL,
    remote_branch text DEFAULT ''::text,
    head_commit text DEFAULT ''::text,
    base_commit text DEFAULT ''::text,
    ahead integer DEFAULT 0,
    behind integer DEFAULT 0,
    files text DEFAULT '{}'::text,
    triggered_by text DEFAULT ''::text,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE task_session_messages (
    id text NOT NULL,
    task_session_id text NOT NULL,
    task_id text DEFAULT ''::text,
    turn_id text NOT NULL,
    author_type text DEFAULT 'user'::text NOT NULL,
    author_id text DEFAULT ''::text,
    content text NOT NULL,
    requests_input integer DEFAULT 0,
    type text DEFAULT 'message'::text NOT NULL,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    prompt_seq integer DEFAULT 0 NOT NULL
);



CREATE TABLE task_session_prompt_seq (
    task_session_id text NOT NULL,
    last_seq integer NOT NULL
);



CREATE TABLE task_session_subagents (
    id text NOT NULL,
    task_session_id text NOT NULL,
    task_id text NOT NULL,
    turn_id text,
    tool_call_id text NOT NULL,
    agent_execution_id text DEFAULT 'unknown'::text NOT NULL,
    parent_tool_call_id text,
    subagent_type text,
    description text,
    agent_id text,
    child_session_id text,
    model text,
    agent_status text,
    tool_status text,
    is_async integer DEFAULT 0 NOT NULL,
    total_tokens integer,
    tool_use_count integer,
    duration_ms integer,
    source text DEFAULT 'live'::text NOT NULL,
    observed_at timestamp without time zone NOT NULL,
    settled_at timestamp without time zone,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_session_turns (
    id text NOT NULL,
    task_session_id text NOT NULL,
    task_id text NOT NULL,
    started_at timestamp without time zone NOT NULL,
    completed_at timestamp without time zone,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    route_generation bigint DEFAULT 0 NOT NULL,
    metadata text DEFAULT '{}'::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_sessions (
    id text NOT NULL,
    task_id text NOT NULL,
    agent_execution_id text DEFAULT ''::text NOT NULL,
    container_id text DEFAULT ''::text NOT NULL,
    agent_profile_id text,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    route_generation integer DEFAULT 0 NOT NULL,
    route_state text DEFAULT ''::text NOT NULL,
    route_reason text DEFAULT ''::text NOT NULL,
    downstream_acp_session_id text DEFAULT ''::text NOT NULL,
    executor_id text DEFAULT ''::text,
    executor_profile_id text DEFAULT ''::text,
    environment_id text DEFAULT ''::text,
    repository_id text DEFAULT ''::text,
    base_branch text DEFAULT ''::text,
    agent_profile_snapshot text DEFAULT '{}'::text,
    executor_snapshot text DEFAULT '{}'::text,
    environment_snapshot text DEFAULT '{}'::text,
    repository_snapshot text DEFAULT '{}'::text,
    state text DEFAULT 'CREATED'::text NOT NULL,
    error_message text DEFAULT ''::text,
    metadata text DEFAULT '{}'::text,
    started_at timestamp without time zone NOT NULL,
    completed_at timestamp without time zone,
    updated_at timestamp without time zone NOT NULL,
    is_primary integer DEFAULT 0,
    is_passthrough integer DEFAULT 0,
    review_status text DEFAULT ''::text,
    base_commit_sha text DEFAULT ''::text,
    task_environment_id text DEFAULT ''::text,
    cost_subcents bigint DEFAULT 0 NOT NULL,
    tokens_in bigint DEFAULT 0 NOT NULL,
    tokens_cached_in bigint DEFAULT 0 NOT NULL,
    tokens_out bigint DEFAULT 0 NOT NULL,
    workspace_path text DEFAULT ''::text,
    name text DEFAULT ''::text,
    last_read_message_id text DEFAULT ''::text
);



CREATE TABLE task_status_summaries (
    task_id text NOT NULL,
    workspace_id text NOT NULL,
    revision integer DEFAULT 0 NOT NULL,
    summary text DEFAULT '{}'::text NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_step_transitions (
    id bigint NOT NULL,
    task_id text NOT NULL,
    session_id text,
    from_workflow_id text,
    from_workflow_step_id text,
    to_workflow_id text,
    to_workflow_step_id text,
    trigger text NOT NULL,
    actor_kind text NOT NULL,
    actor_id text,
    contract_version integer NOT NULL,
    occurred_at timestamp without time zone NOT NULL
);



CREATE SEQUENCE task_step_transitions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE task_step_transitions_id_seq OWNED BY task_step_transitions.id;



CREATE TABLE task_usage_events (
    id bigint NOT NULL,
    usage_event_id text NOT NULL,
    task_id text NOT NULL,
    session_id text,
    turn_id text,
    agent_profile_id text DEFAULT ''::text NOT NULL,
    agent_type text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    provider text DEFAULT ''::text NOT NULL,
    tokens_in bigint DEFAULT 0 NOT NULL,
    tokens_cached_read bigint,
    tokens_cached_write bigint,
    tokens_out bigint,
    tokens_thought bigint,
    tokens_total bigint DEFAULT 0 NOT NULL,
    cost_subcents bigint DEFAULT 0 NOT NULL,
    cost_source text NOT NULL,
    estimated integer DEFAULT 0 NOT NULL,
    rate_input_per_million bigint,
    rate_cached_read_per_million bigint,
    rate_cached_write_per_million bigint,
    rate_output_per_million bigint,
    pricing_catalog_version text,
    contract_version integer NOT NULL,
    occurred_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE SEQUENCE task_usage_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE task_usage_events_id_seq OWNED BY task_usage_events.id;



CREATE TABLE task_walkthroughs (
    id text NOT NULL,
    task_id text NOT NULL,
    title text DEFAULT 'Walkthrough'::text NOT NULL,
    steps text DEFAULT '[]'::text NOT NULL,
    created_by text DEFAULT 'agent'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_workspace_folders (
    id text NOT NULL,
    task_id text NOT NULL,
    local_path text NOT NULL,
    display_name text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE task_workspace_group_members (
    workspace_group_id text NOT NULL,
    task_id text NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    released_at timestamp without time zone,
    release_reason text DEFAULT ''::text NOT NULL,
    released_by_cascade_id text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE TABLE task_workspace_groups (
    id text NOT NULL,
    workspace_id text NOT NULL,
    owner_task_id text NOT NULL,
    materialized_path text DEFAULT ''::text NOT NULL,
    materialized_environment_id text DEFAULT ''::text NOT NULL,
    materialized_kind text NOT NULL,
    owned_by_kandev integer DEFAULT 0 NOT NULL,
    cleanup_policy text DEFAULT 'never_delete'::text NOT NULL,
    cleanup_status text DEFAULT 'active'::text NOT NULL,
    cleaned_at timestamp without time zone,
    cleanup_error text DEFAULT ''::text NOT NULL,
    restore_status text DEFAULT 'not_needed'::text NOT NULL,
    restore_error text DEFAULT ''::text NOT NULL,
    restore_config_json text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE tasks (
    id text NOT NULL,
    workspace_id text DEFAULT ''::text NOT NULL,
    workflow_id text DEFAULT ''::text NOT NULL,
    workflow_step_id text DEFAULT ''::text NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text,
    state text DEFAULT 'TODO'::text,
    priority text DEFAULT 'medium'::text NOT NULL,
    "position" integer DEFAULT 0,
    wip_admitted integer DEFAULT 1 NOT NULL,
    queued_for_step_id text DEFAULT ''::text NOT NULL,
    queued_at timestamp without time zone,
    metadata text DEFAULT '{}'::text,
    autopilot_enabled integer DEFAULT 0 NOT NULL,
    archived_at timestamp without time zone,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    is_ephemeral integer DEFAULT 0 NOT NULL,
    parent_id text DEFAULT ''::text,
    origin text DEFAULT 'manual'::text,
    project_id text DEFAULT ''::text,
    labels text DEFAULT '[]'::text,
    identifier text,
    archived_by_cascade_id text DEFAULT ''::text,
    external_id text COLLATE pg_catalog."C",
    external_id_settled_at timestamp without time zone,
    checkout_agent_id text,
    checkout_at timestamp without time zone,
    checkout_run_id text,
    CONSTRAINT tasks_priority_check CHECK ((priority = ANY (ARRAY['critical'::text, 'high'::text, 'medium'::text, 'low'::text])))
);



CREATE TABLE telemetry_activations (
    contract_key text NOT NULL,
    contract_version integer NOT NULL,
    activated_at timestamp without time zone NOT NULL
);



CREATE TABLE user_agent_profile_recent_use (
    user_id text NOT NULL,
    context text NOT NULL,
    profile_ids text DEFAULT '[]'::text NOT NULL,
    revision bigint DEFAULT 0 NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE user_terminals (
    id text NOT NULL,
    task_id text NOT NULL,
    environment_id text NOT NULL,
    seq integer NOT NULL,
    custom_name text,
    state text DEFAULT 'open'::text NOT NULL,
    initial_command text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE users (
    id text NOT NULL,
    email text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    role text DEFAULT 'admin'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    settings text DEFAULT '{}'::text NOT NULL,
    settings_revision bigint DEFAULT 0 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE utility_agent_calls (
    id text NOT NULL,
    utility_id text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    resolved_prompt text DEFAULT ''::text NOT NULL,
    response text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    agent_profile_id text DEFAULT ''::text NOT NULL,
    execution_profile_id text DEFAULT ''::text NOT NULL,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    response_tokens integer DEFAULT 0 NOT NULL,
    duration_ms integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    completed_at timestamp without time zone
);



CREATE TABLE utility_agents (
    id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    prompt text NOT NULL,
    agent_id text DEFAULT 'claude-acp'::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    agent_profile_id text DEFAULT ''::text NOT NULL,
    profile_binding_state text DEFAULT 'explicit'::text NOT NULL,
    builtin integer DEFAULT 0 NOT NULL,
    enabled integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE workflow_step_decisions (
    id text NOT NULL,
    task_id text NOT NULL,
    step_id text NOT NULL,
    participant_id text NOT NULL,
    decision text NOT NULL,
    note text DEFAULT ''::text,
    decided_at timestamp without time zone NOT NULL,
    superseded_at timestamp without time zone,
    decider_type text DEFAULT ''::text NOT NULL,
    decider_id text DEFAULT ''::text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    comment text DEFAULT ''::text NOT NULL
);



CREATE TABLE workflow_step_entries (
    id bigint NOT NULL,
    task_id text NOT NULL,
    step_id text NOT NULL,
    entry_seq integer NOT NULL,
    digest text NOT NULL,
    created_at timestamp without time zone NOT NULL
);



CREATE SEQUENCE workflow_step_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE workflow_step_entries_id_seq OWNED BY workflow_step_entries.id;



CREATE TABLE workflow_step_entry_markers (
    id bigint NOT NULL,
    entry_id integer NOT NULL,
    "position" integer NOT NULL,
    kind text NOT NULL,
    state text NOT NULL,
    operation_id text,
    error text,
    claimed_at timestamp without time zone,
    completed_at timestamp without time zone
);



CREATE SEQUENCE workflow_step_entry_markers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE workflow_step_entry_markers_id_seq OWNED BY workflow_step_entry_markers.id;



CREATE TABLE workflow_step_participants (
    id text NOT NULL,
    step_id text DEFAULT ''::text NOT NULL,
    task_id text DEFAULT ''::text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    agent_profile_id text DEFAULT ''::text NOT NULL,
    decision_required integer DEFAULT 0 NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT '1970-01-01 00:00:00'::timestamp without time zone NOT NULL,
    provenance text DEFAULT 'manual'::text NOT NULL
);



CREATE TABLE workflow_steps (
    id text NOT NULL,
    workflow_id text DEFAULT ''::text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    color text,
    prompt text,
    events text,
    allow_manual_move integer DEFAULT 1,
    is_start_step integer DEFAULT 0,
    show_in_command_panel integer DEFAULT 1,
    auto_archive_after_hours integer DEFAULT 0,
    agent_profile_id text DEFAULT ''::text NOT NULL,
    stage_type text DEFAULT 'custom'::text NOT NULL,
    auto_advance_requires_signal integer DEFAULT 0 NOT NULL,
    cancel_triggers_turn_complete integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    wip_limit integer DEFAULT 0 NOT NULL,
    pull_from_step_id text DEFAULT ''::text NOT NULL
);



CREATE TABLE workflow_templates (
    id text NOT NULL,
    name text NOT NULL,
    description text,
    is_system integer DEFAULT 0,
    steps text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);



CREATE TABLE workflows (
    id text NOT NULL,
    workspace_id text DEFAULT ''::text NOT NULL,
    workflow_template_id text DEFAULT ''::text,
    name text NOT NULL,
    description text DEFAULT ''::text,
    prompt text DEFAULT ''::text NOT NULL,
    hidden integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    agent_profile_id text DEFAULT ''::text,
    is_system integer DEFAULT 0,
    style text DEFAULT 'kanban'::text NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    source_path text DEFAULT ''::text NOT NULL
);



CREATE TABLE workspaces (
    id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    owner_id text DEFAULT ''::text,
    default_executor_id text DEFAULT ''::text,
    default_environment_id text DEFAULT ''::text,
    default_agent_profile_id text DEFAULT ''::text,
    default_config_agent_profile_id text DEFAULT ''::text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    task_prefix text DEFAULT 'KAN'::text,
    task_sequence integer DEFAULT 0,
    office_workflow_id text DEFAULT ''::text
);



ALTER TABLE ONLY session_step_history ALTER COLUMN id SET DEFAULT nextval('session_step_history_id_seq'::regclass);



ALTER TABLE ONLY task_step_transitions ALTER COLUMN id SET DEFAULT nextval('task_step_transitions_id_seq'::regclass);



ALTER TABLE ONLY task_usage_events ALTER COLUMN id SET DEFAULT nextval('task_usage_events_id_seq'::regclass);



ALTER TABLE ONLY workflow_step_entries ALTER COLUMN id SET DEFAULT nextval('workflow_step_entries_id_seq'::regclass);



ALTER TABLE ONLY workflow_step_entry_markers ALTER COLUMN id SET DEFAULT nextval('workflow_step_entry_markers_id_seq'::regclass);



ALTER TABLE ONLY agent_continuation_summaries
    ADD CONSTRAINT agent_continuation_summaries_pkey PRIMARY KEY (agent_profile_id, scope);



ALTER TABLE ONLY agent_profile_mcp_configs
    ADD CONSTRAINT agent_profile_mcp_configs_pkey PRIMARY KEY (profile_id);



ALTER TABLE ONLY agent_profiles
    ADD CONSTRAINT agent_profiles_pkey PRIMARY KEY (id);



ALTER TABLE ONLY agent_wakeup_requests
    ADD CONSTRAINT agent_wakeup_requests_pkey PRIMARY KEY (id);



ALTER TABLE ONLY agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);



ALTER TABLE ONLY auth_api_tokens
    ADD CONSTRAINT auth_api_tokens_pkey PRIMARY KEY (id);



ALTER TABLE ONLY auth_api_tokens
    ADD CONSTRAINT auth_api_tokens_token_sha256_key UNIQUE (token_sha256);



ALTER TABLE ONLY auth_identities
    ADD CONSTRAINT auth_identities_pkey PRIMARY KEY (id);



ALTER TABLE ONLY auth_identities
    ADD CONSTRAINT auth_identities_provider_subject_key UNIQUE (provider, subject);



ALTER TABLE ONLY auth_identities
    ADD CONSTRAINT auth_identities_user_id_provider_key UNIQUE (user_id, provider);



ALTER TABLE ONLY auth_invites
    ADD CONSTRAINT auth_invites_pkey PRIMARY KEY (id);



ALTER TABLE ONLY auth_invites
    ADD CONSTRAINT auth_invites_token_sha256_key UNIQUE (token_sha256);



ALTER TABLE ONLY auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY auth_sessions
    ADD CONSTRAINT auth_sessions_token_sha256_key UNIQUE (token_sha256);



ALTER TABLE ONLY custom_prompts
    ADD CONSTRAINT custom_prompts_name_key UNIQUE (name);



ALTER TABLE ONLY custom_prompts
    ADD CONSTRAINT custom_prompts_pkey PRIMARY KEY (id);



ALTER TABLE ONLY dynamic_agent_profiles
    ADD CONSTRAINT dynamic_agent_profiles_pkey PRIMARY KEY (profile_id);



ALTER TABLE ONLY dynamic_agent_routes
    ADD CONSTRAINT dynamic_agent_routes_pkey PRIMARY KEY (dynamic_profile_id, "position");



ALTER TABLE ONLY dynamic_installation_keys
    ADD CONSTRAINT dynamic_installation_keys_pkey PRIMARY KEY (id);



ALTER TABLE ONLY dynamic_resource_circuits
    ADD CONSTRAINT dynamic_resource_circuits_pkey PRIMARY KEY (resource_key);



ALTER TABLE ONLY dynamic_route_attempts
    ADD CONSTRAINT dynamic_route_attempts_pkey PRIMARY KEY (id);



ALTER TABLE ONLY dynamic_route_attempts
    ADD CONSTRAINT dynamic_route_attempts_session_id_route_generation_key UNIQUE (session_id, route_generation);



ALTER TABLE ONLY dynamic_route_states
    ADD CONSTRAINT dynamic_route_states_pkey PRIMARY KEY (session_id);



ALTER TABLE ONLY editors
    ADD CONSTRAINT editors_pkey PRIMARY KEY (id);



ALTER TABLE ONLY editors
    ADD CONSTRAINT editors_type_key UNIQUE (type);



ALTER TABLE ONLY environments
    ADD CONSTRAINT environments_pkey PRIMARY KEY (id);



ALTER TABLE ONLY executor_profiles
    ADD CONSTRAINT executor_profiles_pkey PRIMARY KEY (id);



ALTER TABLE ONLY executors
    ADD CONSTRAINT executors_pkey PRIMARY KEY (id);



ALTER TABLE ONLY executors_running
    ADD CONSTRAINT executors_running_pkey PRIMARY KEY (id);



ALTER TABLE ONLY executors_running
    ADD CONSTRAINT executors_running_session_id_key UNIQUE (session_id);



ALTER TABLE ONLY kandev_meta
    ADD CONSTRAINT kandev_meta_pkey PRIMARY KEY (key);



ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (id);



ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_provider_id_event_type_occurrence_i_key UNIQUE (provider_id, event_type, occurrence_id);



ALTER TABLE ONLY notification_migrations
    ADD CONSTRAINT notification_migrations_pkey PRIMARY KEY (name);



ALTER TABLE ONLY notification_providers
    ADD CONSTRAINT notification_providers_pkey PRIMARY KEY (id);



ALTER TABLE ONLY notification_subscriptions
    ADD CONSTRAINT notification_subscriptions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY notification_subscriptions
    ADD CONSTRAINT notification_subscriptions_provider_id_event_type_key UNIQUE (provider_id, event_type);



ALTER TABLE ONLY office_activity_log
    ADD CONSTRAINT office_activity_log_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_agent_instructions
    ADD CONSTRAINT office_agent_instructions_agent_profile_id_filename_key UNIQUE (agent_profile_id, filename);



ALTER TABLE ONLY office_agent_instructions
    ADD CONSTRAINT office_agent_instructions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_agent_memory
    ADD CONSTRAINT office_agent_memory_agent_profile_id_layer_key_key UNIQUE (agent_profile_id, layer, key);



ALTER TABLE ONLY office_agent_memory
    ADD CONSTRAINT office_agent_memory_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_agent_runtime
    ADD CONSTRAINT office_agent_runtime_pkey PRIMARY KEY (agent_id);



ALTER TABLE ONLY office_approvals
    ADD CONSTRAINT office_approvals_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_budget_policies
    ADD CONSTRAINT office_budget_policies_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_channels
    ADD CONSTRAINT office_channels_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_cost_events
    ADD CONSTRAINT office_cost_events_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_inbox_dismissals
    ADD CONSTRAINT office_inbox_dismissals_pkey PRIMARY KEY (user_id, item_kind, item_id);



ALTER TABLE ONLY office_labels
    ADD CONSTRAINT office_labels_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_labels
    ADD CONSTRAINT office_labels_workspace_id_name_key UNIQUE (workspace_id, name);



ALTER TABLE ONLY office_onboarding
    ADD CONSTRAINT office_onboarding_pkey PRIMARY KEY (workspace_id);



ALTER TABLE ONLY office_projects
    ADD CONSTRAINT office_projects_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_provider_health
    ADD CONSTRAINT office_provider_health_pkey PRIMARY KEY (workspace_id, provider_id, scope, scope_value);



ALTER TABLE ONLY office_routine_runs
    ADD CONSTRAINT office_routine_runs_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_routine_triggers
    ADD CONSTRAINT office_routine_triggers_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_routines
    ADD CONSTRAINT office_routines_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_run_route_attempts
    ADD CONSTRAINT office_run_route_attempts_pkey PRIMARY KEY (run_id, seq);



ALTER TABLE ONLY office_run_skills
    ADD CONSTRAINT office_run_skills_pkey PRIMARY KEY (run_id, skill_id);



ALTER TABLE ONLY office_skills
    ADD CONSTRAINT office_skills_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_skills
    ADD CONSTRAINT office_skills_workspace_id_slug_key UNIQUE (workspace_id, slug);



ALTER TABLE ONLY office_task_labels
    ADD CONSTRAINT office_task_labels_pkey PRIMARY KEY (task_id, label_id);



ALTER TABLE ONLY office_task_tree_hold_members
    ADD CONSTRAINT office_task_tree_hold_members_pkey PRIMARY KEY (hold_id, task_id);



ALTER TABLE ONLY office_task_tree_holds
    ADD CONSTRAINT office_task_tree_holds_pkey PRIMARY KEY (id);



ALTER TABLE ONLY office_workspace_governance
    ADD CONSTRAINT office_workspace_governance_pkey PRIMARY KEY (workspace_id, key);



ALTER TABLE ONLY office_workspace_routing
    ADD CONSTRAINT office_workspace_routing_pkey PRIMARY KEY (workspace_id);



ALTER TABLE ONLY office_workspace_settings
    ADD CONSTRAINT office_workspace_settings_pkey PRIMARY KEY (workspace_id);



ALTER TABLE ONLY parent_child_wake_receipts
    ADD CONSTRAINT parent_child_wake_receipts_pkey PRIMARY KEY (parent_task_id);



ALTER TABLE ONLY quick_terminal_tabs
    ADD CONSTRAINT quick_terminal_tabs_pkey PRIMARY KEY (tab_id);



ALTER TABLE ONLY quick_terminal_tabs
    ADD CONSTRAINT quick_terminal_tabs_user_id_workspace_id_sequence_key UNIQUE (user_id, workspace_id, sequence);



ALTER TABLE ONLY quick_terminal_workspace_sequences
    ADD CONSTRAINT quick_terminal_workspace_sequences_pkey PRIMARY KEY (user_id, workspace_id);



ALTER TABLE ONLY repositories
    ADD CONSTRAINT repositories_pkey PRIMARY KEY (id);



ALTER TABLE ONLY repository_branch_policies
    ADD CONSTRAINT repository_branch_policies_pkey PRIMARY KEY (id);



ALTER TABLE ONLY repository_scripts
    ADD CONSTRAINT repository_scripts_pkey PRIMARY KEY (id);



ALTER TABLE ONLY repository_secret_bindings
    ADD CONSTRAINT repository_secret_bindings_pkey PRIMARY KEY (repository_id, key);



ALTER TABLE ONLY repository_set_items
    ADD CONSTRAINT repository_set_items_pkey PRIMARY KEY (id);



ALTER TABLE ONLY repository_set_items
    ADD CONSTRAINT repository_set_items_repository_set_id_repository_id_key UNIQUE (repository_set_id, repository_id);



ALTER TABLE ONLY repository_sets
    ADD CONSTRAINT repository_sets_pkey PRIMARY KEY (id);



ALTER TABLE ONLY repository_sets
    ADD CONSTRAINT repository_sets_workspace_id_name_key UNIQUE (workspace_id, name);



ALTER TABLE ONLY run_events
    ADD CONSTRAINT run_events_pkey PRIMARY KEY (run_id, seq);



ALTER TABLE ONLY runs
    ADD CONSTRAINT runs_pkey PRIMARY KEY (id);



ALTER TABLE ONLY runtime_flag_overrides
    ADD CONSTRAINT runtime_flag_overrides_pkey PRIMARY KEY (key);



ALTER TABLE ONLY secrets
    ADD CONSTRAINT secrets_pkey PRIMARY KEY (id);



ALTER TABLE ONLY session_file_reviews
    ADD CONSTRAINT session_file_reviews_pkey PRIMARY KEY (id);



ALTER TABLE ONLY session_file_reviews
    ADD CONSTRAINT session_file_reviews_session_id_file_path_key UNIQUE (session_id, file_path);



ALTER TABLE ONLY session_step_history
    ADD CONSTRAINT session_step_history_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_blockers
    ADD CONSTRAINT task_blockers_pkey PRIMARY KEY (task_id, blocker_task_id);



ALTER TABLE ONLY task_comments
    ADD CONSTRAINT task_comments_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_document_revisions
    ADD CONSTRAINT task_document_revisions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_document_revisions
    ADD CONSTRAINT task_document_revisions_task_id_document_key_revision_numbe_key UNIQUE (task_id, document_key, revision_number);



ALTER TABLE ONLY task_documents
    ADD CONSTRAINT task_documents_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_documents
    ADD CONSTRAINT task_documents_task_id_key_key UNIQUE (task_id, key);



ALTER TABLE ONLY task_environment_repos
    ADD CONSTRAINT task_environment_repos_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_environment_repos
    ADD CONSTRAINT task_environment_repos_task_environment_id_repository_id_br_key UNIQUE (task_environment_id, repository_id, branch_slug);



ALTER TABLE ONLY task_environments
    ADD CONSTRAINT task_environments_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_message_attachments
    ADD CONSTRAINT task_message_attachments_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_message_attachments
    ADD CONSTRAINT task_message_attachments_storage_key_key UNIQUE (storage_key);



ALTER TABLE ONLY task_plan_revisions
    ADD CONSTRAINT task_plan_revisions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_plan_revisions
    ADD CONSTRAINT task_plan_revisions_task_id_revision_number_key UNIQUE (task_id, revision_number);



ALTER TABLE ONLY task_plans
    ADD CONSTRAINT task_plans_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_plans
    ADD CONSTRAINT task_plans_task_id_key UNIQUE (task_id);



ALTER TABLE ONLY task_repositories
    ADD CONSTRAINT task_repositories_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_repositories
    ADD CONSTRAINT task_repositories_task_id_repository_id_base_branch_checkou_key UNIQUE (task_id, repository_id, base_branch, checkout_branch);



ALTER TABLE ONLY task_resource_cleanup_jobs
    ADD CONSTRAINT task_resource_cleanup_jobs_operation_id_key UNIQUE (operation_id);



ALTER TABLE ONLY task_resource_cleanup_jobs
    ADD CONSTRAINT task_resource_cleanup_jobs_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_review_findings
    ADD CONSTRAINT task_review_findings_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_review_runs
    ADD CONSTRAINT task_review_runs_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_session_commits
    ADD CONSTRAINT task_session_commits_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_session_git_snapshots
    ADD CONSTRAINT task_session_git_snapshots_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_session_messages
    ADD CONSTRAINT task_session_messages_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_session_prompt_seq
    ADD CONSTRAINT task_session_prompt_seq_pkey PRIMARY KEY (task_session_id);



ALTER TABLE ONLY task_session_subagents
    ADD CONSTRAINT task_session_subagents_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_session_subagents
    ADD CONSTRAINT task_session_subagents_task_session_id_agent_execution_id_t_key UNIQUE (task_session_id, agent_execution_id, tool_call_id);



ALTER TABLE ONLY task_session_turns
    ADD CONSTRAINT task_session_turns_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_sessions
    ADD CONSTRAINT task_sessions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_status_summaries
    ADD CONSTRAINT task_status_summaries_pkey PRIMARY KEY (task_id);



ALTER TABLE ONLY task_step_transitions
    ADD CONSTRAINT task_step_transitions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_usage_events
    ADD CONSTRAINT task_usage_events_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_walkthroughs
    ADD CONSTRAINT task_walkthroughs_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_walkthroughs
    ADD CONSTRAINT task_walkthroughs_task_id_key UNIQUE (task_id);



ALTER TABLE ONLY task_workspace_folders
    ADD CONSTRAINT task_workspace_folders_pkey PRIMARY KEY (id);



ALTER TABLE ONLY task_workspace_folders
    ADD CONSTRAINT task_workspace_folders_task_id_display_name_key UNIQUE (task_id, display_name);



ALTER TABLE ONLY task_workspace_folders
    ADD CONSTRAINT task_workspace_folders_task_id_local_path_key UNIQUE (task_id, local_path);



ALTER TABLE ONLY task_workspace_group_members
    ADD CONSTRAINT task_workspace_group_members_pkey PRIMARY KEY (workspace_group_id, task_id);



ALTER TABLE ONLY task_workspace_groups
    ADD CONSTRAINT task_workspace_groups_pkey PRIMARY KEY (id);



ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);



ALTER TABLE ONLY telemetry_activations
    ADD CONSTRAINT telemetry_activations_pkey PRIMARY KEY (contract_key, contract_version);



ALTER TABLE ONLY workflow_step_entry_markers
    ADD CONSTRAINT uniq_workflow_step_entry_markers_entry_position UNIQUE (entry_id, "position");



ALTER TABLE ONLY user_agent_profile_recent_use
    ADD CONSTRAINT user_agent_profile_recent_use_pkey PRIMARY KEY (user_id, context);



ALTER TABLE ONLY user_terminals
    ADD CONSTRAINT user_terminals_pkey PRIMARY KEY (id);



ALTER TABLE ONLY user_terminals
    ADD CONSTRAINT user_terminals_task_id_seq_key UNIQUE (task_id, seq);



ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);



ALTER TABLE ONLY utility_agent_calls
    ADD CONSTRAINT utility_agent_calls_pkey PRIMARY KEY (id);



ALTER TABLE ONLY utility_agents
    ADD CONSTRAINT utility_agents_name_key UNIQUE (name);



ALTER TABLE ONLY utility_agents
    ADD CONSTRAINT utility_agents_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_step_decisions
    ADD CONSTRAINT workflow_step_decisions_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_step_entries
    ADD CONSTRAINT workflow_step_entries_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_step_entries
    ADD CONSTRAINT workflow_step_entries_task_id_step_id_entry_seq_key UNIQUE (task_id, step_id, entry_seq);



ALTER TABLE ONLY workflow_step_entry_markers
    ADD CONSTRAINT workflow_step_entry_markers_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_step_participants
    ADD CONSTRAINT workflow_step_participants_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_steps
    ADD CONSTRAINT workflow_steps_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflow_templates
    ADD CONSTRAINT workflow_templates_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workflows
    ADD CONSTRAINT workflows_pkey PRIMARY KEY (id);



ALTER TABLE ONLY workspaces
    ADD CONSTRAINT workspaces_pkey PRIMARY KEY (id);



CREATE INDEX idx_activity_run_id ON office_activity_log USING btree (run_id) WHERE (run_id <> ''::text);



CREATE INDEX idx_activity_session_id ON office_activity_log USING btree (session_id) WHERE (session_id <> ''::text);



CREATE INDEX idx_activity_workspace_created ON office_activity_log USING btree (workspace_id, created_at DESC);



CREATE INDEX idx_agent_profiles_agent_id ON agent_profiles USING btree (agent_id);



CREATE INDEX idx_agent_profiles_reports_to ON agent_profiles USING btree (reports_to) WHERE (reports_to <> ''::text);



CREATE INDEX idx_agent_profiles_role ON agent_profiles USING btree (workspace_id, role) WHERE (role <> ''::text);



CREATE INDEX idx_agent_profiles_workspace ON agent_profiles USING btree (workspace_id);



CREATE UNIQUE INDEX idx_agents_name ON agents USING btree (name);



CREATE INDEX idx_auth_api_tokens_user ON auth_api_tokens USING btree (user_id);



CREATE INDEX idx_auth_sessions_user ON auth_sessions USING btree (user_id);



CREATE INDEX idx_commits_session_time ON task_session_commits USING btree (session_id, committed_at);



CREATE INDEX idx_dynamic_agent_routes_execution_profile ON dynamic_agent_routes USING btree (execution_profile_id);



CREATE INDEX idx_dynamic_route_attempts_session ON dynamic_route_attempts USING btree (session_id, route_generation);



CREATE INDEX idx_git_snapshots_environment ON task_session_git_snapshots USING btree (task_environment_id, created_at DESC);



CREATE INDEX idx_git_snapshots_environment_type ON task_session_git_snapshots USING btree (task_environment_id, snapshot_type, created_at DESC);



CREATE INDEX idx_git_snapshots_session ON task_session_git_snapshots USING btree (session_id, created_at DESC);



CREATE INDEX idx_git_snapshots_type ON task_session_git_snapshots USING btree (session_id, snapshot_type);



CREATE INDEX idx_messages_author_type ON task_session_messages USING btree (task_session_id, author_type, type);



CREATE INDEX idx_messages_created_at ON task_session_messages USING btree (created_at);



CREATE INDEX idx_messages_metadata_pending_id ON task_session_messages USING btree (task_session_id, (((metadata)::jsonb ->> 'pending_id'::text)));



CREATE INDEX idx_messages_metadata_tool_call_id ON task_session_messages USING btree (task_session_id, (((metadata)::jsonb ->> 'tool_call_id'::text)));



CREATE INDEX idx_messages_prompt_order ON task_session_messages USING btree (task_session_id, created_at, id);



CREATE INDEX idx_messages_prompt_user_order ON task_session_messages USING btree (task_session_id, author_type, created_at, id);



CREATE INDEX idx_messages_session_created ON task_session_messages USING btree (task_session_id, created_at);



CREATE INDEX idx_messages_session_id ON task_session_messages USING btree (task_session_id);



CREATE INDEX idx_messages_session_updated ON task_session_messages USING btree (task_session_id, updated_at);



CREATE INDEX idx_messages_task_author_created ON task_session_messages USING btree (task_id, author_type, type, created_at DESC);



CREATE INDEX idx_messages_turn ON task_session_messages USING btree (turn_id);



CREATE INDEX idx_messages_turn_id ON task_session_messages USING btree (turn_id);



CREATE INDEX idx_notification_deliveries_session_id ON notification_deliveries USING btree (task_session_id);



CREATE INDEX idx_notification_providers_user_id ON notification_providers USING btree (user_id);



CREATE INDEX idx_notification_subscriptions_provider_id ON notification_subscriptions USING btree (provider_id);



CREATE INDEX idx_notification_subscriptions_user_id ON notification_subscriptions USING btree (user_id);



CREATE INDEX idx_office_cost_agent ON office_cost_events USING btree (agent_profile_id);



CREATE INDEX idx_office_cost_occurred ON office_cost_events USING btree (occurred_at DESC);



CREATE INDEX idx_office_cost_task ON office_cost_events USING btree (task_id);



CREATE INDEX idx_office_inbox_dismissals_kind ON office_inbox_dismissals USING btree (item_kind, item_id);



CREATE INDEX idx_office_labels_workspace ON office_labels USING btree (workspace_id);



CREATE INDEX idx_office_task_labels_label ON office_task_labels USING btree (label_id);



CREATE INDEX idx_quick_terminal_tabs_session ON quick_terminal_tabs USING btree (session_id);



CREATE INDEX idx_quick_terminal_tabs_workspace ON quick_terminal_tabs USING btree (user_id, workspace_id, sequence);



CREATE INDEX idx_repos_workspace ON repositories USING btree (workspace_id) WHERE (deleted_at IS NULL);



CREATE INDEX idx_repositories_workspace_id ON repositories USING btree (workspace_id);



CREATE INDEX idx_repository_branch_policies_repository_id ON repository_branch_policies USING btree (repository_id);



CREATE INDEX idx_repository_scripts_repo_id ON repository_scripts USING btree (repository_id);



CREATE INDEX idx_repository_secret_bindings_repository ON repository_secret_bindings USING btree (repository_id);



CREATE INDEX idx_repository_set_items_repository_id ON repository_set_items USING btree (repository_id);



CREATE INDEX idx_repository_set_items_set_position ON repository_set_items USING btree (repository_set_id, "position");



CREATE INDEX idx_repository_sets_workspace_id ON repository_sets USING btree (workspace_id);



CREATE INDEX idx_run_events_run_created ON run_events USING btree (run_id, created_at);



CREATE INDEX idx_run_failure_scope_status ON runs USING btree (agent_profile_id, continuation_scope, status);



CREATE UNIQUE INDEX idx_run_idempotency ON runs USING btree (idempotency_key) WHERE (idempotency_key IS NOT NULL);



CREATE INDEX idx_run_payload_comment_id ON runs USING btree ((((payload)::jsonb ->> 'comment_id'::text)));



CREATE INDEX idx_run_status_requested ON runs USING btree (status, requested_at);



CREATE INDEX idx_session_commits_session ON task_session_commits USING btree (session_id, committed_at DESC);



CREATE INDEX idx_session_commits_sha ON task_session_commits USING btree (commit_sha);



CREATE INDEX idx_session_file_reviews_session ON session_file_reviews USING btree (session_id);



CREATE INDEX idx_session_step_history_session ON session_step_history USING btree (session_id);



CREATE INDEX idx_sessions_agent_started ON task_sessions USING btree (agent_profile_id, started_at);



CREATE INDEX idx_sessions_repo ON task_sessions USING btree (repository_id);



CREATE INDEX idx_sessions_task_started ON task_sessions USING btree (task_id, started_at);



CREATE INDEX idx_subagents_session_id ON task_session_subagents USING btree (task_session_id);



CREATE INDEX idx_subagents_task_id ON task_session_subagents USING btree (task_id);



CREATE INDEX idx_subagents_turn_id ON task_session_subagents USING btree (turn_id);



CREATE INDEX idx_task_comments_task_created ON task_comments USING btree (task_id, created_at);



CREATE INDEX idx_task_document_revisions_task_key ON task_document_revisions USING btree (task_id, document_key, revision_number DESC);



CREATE INDEX idx_task_documents_task_id ON task_documents USING btree (task_id);



CREATE INDEX idx_task_environment_repos_env_id ON task_environment_repos USING btree (task_environment_id);



CREATE INDEX idx_task_environment_repos_repository_id ON task_environment_repos USING btree (repository_id);



CREATE INDEX idx_task_environments_status ON task_environments USING btree (status);



CREATE INDEX idx_task_environments_task_id ON task_environments USING btree (task_id);



CREATE INDEX idx_task_message_attachments_owner_state ON task_message_attachments USING btree (owner_id, state, expires_at);



CREATE INDEX idx_task_message_attachments_scope ON task_message_attachments USING btree (workspace_id, task_id, session_id, state);



CREATE INDEX idx_task_plan_revisions_task_created ON task_plan_revisions USING btree (task_id, created_at DESC);



CREATE INDEX idx_task_plan_revisions_task_number ON task_plan_revisions USING btree (task_id, revision_number DESC);



CREATE INDEX idx_task_plans_task_id ON task_plans USING btree (task_id);



CREATE INDEX idx_task_repos_repo ON task_repositories USING btree (repository_id, task_id);



CREATE INDEX idx_task_repos_task ON task_repositories USING btree (task_id, repository_id);



CREATE INDEX idx_task_repositories_repository_id ON task_repositories USING btree (repository_id);



CREATE INDEX idx_task_repositories_task_id ON task_repositories USING btree (task_id);



CREATE INDEX idx_task_resource_cleanup_jobs_due ON task_resource_cleanup_jobs USING btree (state, next_attempt_at, created_at);



CREATE INDEX idx_task_resource_cleanup_jobs_task_id ON task_resource_cleanup_jobs USING btree (task_id);



CREATE INDEX idx_task_review_findings_anchor ON task_review_findings USING btree (task_id, repository_name, file_path);



CREATE INDEX idx_task_review_findings_run ON task_review_findings USING btree (run_id);



CREATE INDEX idx_task_review_findings_task_status ON task_review_findings USING btree (task_id, status);



CREATE UNIQUE INDEX idx_task_review_runs_entry_id ON task_review_runs USING btree (entry_id) WHERE (entry_id <> ''::text);



CREATE INDEX idx_task_review_runs_task ON task_review_runs USING btree (task_id, created_at);



CREATE INDEX idx_task_sessions_state ON task_sessions USING btree (state);



CREATE INDEX idx_task_sessions_task_id ON task_sessions USING btree (task_id);



CREATE INDEX idx_task_sessions_task_state ON task_sessions USING btree (task_id, state);



CREATE INDEX idx_task_status_summaries_workspace ON task_status_summaries USING btree (workspace_id);



CREATE INDEX idx_task_step_transitions_occurred ON task_step_transitions USING btree (occurred_at);



CREATE INDEX idx_task_step_transitions_task ON task_step_transitions USING btree (task_id, occurred_at, id);



CREATE INDEX idx_task_usage_events_session_turn ON task_usage_events USING btree (session_id, turn_id);



CREATE INDEX idx_task_usage_events_task ON task_usage_events USING btree (task_id, occurred_at, id);



CREATE INDEX idx_task_walkthroughs_task_id ON task_walkthroughs USING btree (task_id);



CREATE INDEX idx_task_workspace_folders_task_position ON task_workspace_folders USING btree (task_id, "position");



CREATE INDEX idx_task_workspace_group_members_cascade ON task_workspace_group_members USING btree (released_by_cascade_id);



CREATE INDEX idx_task_workspace_group_members_task ON task_workspace_group_members USING btree (task_id);



CREATE INDEX idx_task_workspace_groups_owner ON task_workspace_groups USING btree (owner_task_id);



CREATE INDEX idx_task_workspace_groups_workspace ON task_workspace_groups USING btree (workspace_id);



CREATE INDEX idx_tasks_archived_at ON tasks USING btree (archived_at);



CREATE INDEX idx_tasks_parent_id ON tasks USING btree (parent_id);



CREATE INDEX idx_tasks_project_id ON tasks USING btree (project_id);



CREATE INDEX idx_tasks_queued_for_step ON tasks USING btree (queued_for_step_id, queued_at);



CREATE INDEX idx_tasks_updated ON tasks USING btree (workspace_id, updated_at);



CREATE INDEX idx_tasks_workflow_id ON tasks USING btree (workflow_id);



CREATE INDEX idx_tasks_workflow_state ON tasks USING btree (workflow_step_id, state);



CREATE INDEX idx_tasks_workflow_step_id ON tasks USING btree (workflow_step_id);



CREATE INDEX idx_tasks_workspace_archived ON tasks USING btree (workspace_id, archived_at);



CREATE INDEX idx_tasks_workspace_created ON tasks USING btree (workspace_id, created_at);



CREATE INDEX idx_tasks_workspace_id ON tasks USING btree (workspace_id);



CREATE INDEX idx_tree_hold_members_task ON office_task_tree_hold_members USING btree (task_id);



CREATE INDEX idx_tree_holds_root_mode ON office_task_tree_holds USING btree (root_task_id, mode, released_at);



CREATE INDEX idx_turns_session_id ON task_session_turns USING btree (task_session_id);



CREATE INDEX idx_turns_session_started ON task_session_turns USING btree (task_session_id, started_at);



CREATE INDEX idx_turns_session_times ON task_session_turns USING btree (task_session_id, started_at, completed_at);



CREATE INDEX idx_turns_task ON task_session_turns USING btree (task_id);



CREATE INDEX idx_turns_task_id ON task_session_turns USING btree (task_id);



CREATE INDEX idx_user_terminals_env ON user_terminals USING btree (environment_id);



CREATE INDEX idx_user_terminals_task ON user_terminals USING btree (task_id);



CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);



CREATE INDEX idx_utility_calls_created_at ON utility_agent_calls USING btree (created_at);



CREATE INDEX idx_utility_calls_session_id ON utility_agent_calls USING btree (session_id);



CREATE INDEX idx_utility_calls_utility_id ON utility_agent_calls USING btree (utility_id);



CREATE INDEX idx_wakeup_agent_status ON agent_wakeup_requests USING btree (agent_profile_id, status);



CREATE UNIQUE INDEX idx_wakeup_idempotency ON agent_wakeup_requests USING btree (idempotency_key) WHERE ((idempotency_key IS NOT NULL) AND (idempotency_key <> ''::text));



CREATE INDEX idx_workflow_step_decisions_active ON workflow_step_decisions USING btree (task_id, role) WHERE (superseded_at IS NULL);



CREATE INDEX idx_workflow_step_decisions_participant ON workflow_step_decisions USING btree (participant_id);



CREATE INDEX idx_workflow_step_decisions_task_step ON workflow_step_decisions USING btree (task_id, step_id);



CREATE INDEX idx_workflow_step_entries_task_step ON workflow_step_entries USING btree (task_id, step_id);



CREATE INDEX idx_workflow_step_entry_markers_entry ON workflow_step_entry_markers USING btree (entry_id);



CREATE UNIQUE INDEX idx_workflow_step_participants_natural_key ON workflow_step_participants USING btree (step_id, task_id, role, agent_profile_id);



CREATE INDEX idx_workflow_step_participants_role ON workflow_step_participants USING btree (step_id, role);



CREATE INDEX idx_workflow_step_participants_step ON workflow_step_participants USING btree (step_id);



CREATE INDEX idx_workflow_step_participants_task ON workflow_step_participants USING btree (task_id) WHERE (task_id <> ''::text);



CREATE INDEX idx_workflow_step_participants_task_role ON workflow_step_participants USING btree (task_id, role) WHERE (task_id <> ''::text);



CREATE INDEX idx_workflow_steps_pull_from ON workflow_steps USING btree (pull_from_step_id);



CREATE UNIQUE INDEX idx_workflow_steps_single_start ON workflow_steps USING btree (workflow_id) WHERE (is_start_step = 1);



CREATE INDEX idx_workflow_steps_workflow ON workflow_steps USING btree (workflow_id);



CREATE INDEX idx_workflows_workspace_id ON workflows USING btree (workspace_id);



CREATE UNIQUE INDEX uniq_improve_kandev_workflows ON workflows USING btree (workspace_id, workflow_template_id, hidden) WHERE (workflow_template_id = ANY (ARRAY['improve-kandev'::text, 'report-kandev-issue'::text]));



CREATE UNIQUE INDEX uniq_office_cost_usage_event ON office_cost_events USING btree (usage_event_id) WHERE (usage_event_id IS NOT NULL);



CREATE UNIQUE INDEX uniq_repository_branch_policies_repository_lower_name ON repository_branch_policies USING btree (repository_id, lower(name));



CREATE UNIQUE INDEX uniq_repository_sets_workspace_lower_name ON repository_sets USING btree (workspace_id, lower(name));



CREATE UNIQUE INDEX uniq_session_commits_session_sha ON task_session_commits USING btree (session_id, commit_sha);



CREATE UNIQUE INDEX uniq_task_environments_task_id ON task_environments USING btree (task_id);



CREATE UNIQUE INDEX uniq_task_usage_events_usage_event_id ON task_usage_events USING btree (usage_event_id);



CREATE UNIQUE INDEX uniq_tasks_external_id ON tasks USING btree (workspace_id, external_id) WHERE (external_id IS NOT NULL);



CREATE UNIQUE INDEX uniq_workflow_step_decisions_active_decider ON workflow_step_decisions USING btree (task_id, step_id, decider_id, role) WHERE ((superseded_at IS NULL) AND (decider_id <> ''::text) AND (role <> ''::text));



ALTER TABLE ONLY agent_profile_mcp_configs
    ADD CONSTRAINT agent_profile_mcp_configs_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE;



ALTER TABLE ONLY agent_profiles
    ADD CONSTRAINT agent_profiles_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;



ALTER TABLE ONLY dynamic_agent_profiles
    ADD CONSTRAINT dynamic_agent_profiles_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE;



ALTER TABLE ONLY dynamic_agent_routes
    ADD CONSTRAINT dynamic_agent_routes_dynamic_profile_id_fkey FOREIGN KEY (dynamic_profile_id) REFERENCES dynamic_agent_profiles(profile_id) ON DELETE CASCADE;



ALTER TABLE ONLY dynamic_agent_routes
    ADD CONSTRAINT dynamic_agent_routes_execution_profile_id_fkey FOREIGN KEY (execution_profile_id) REFERENCES agent_profiles(id) ON DELETE RESTRICT;



ALTER TABLE ONLY dynamic_route_attempts
    ADD CONSTRAINT dynamic_route_attempts_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY dynamic_route_states
    ADD CONSTRAINT dynamic_route_states_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY executor_profiles
    ADD CONSTRAINT executor_profiles_executor_id_fkey FOREIGN KEY (executor_id) REFERENCES executors(id);



ALTER TABLE ONLY notification_subscriptions
    ADD CONSTRAINT notification_subscriptions_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES notification_providers(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_routine_runs
    ADD CONSTRAINT office_routine_runs_routine_id_fkey FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_routine_triggers
    ADD CONSTRAINT office_routine_triggers_routine_id_fkey FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_run_route_attempts
    ADD CONSTRAINT office_run_route_attempts_run_id_fkey FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_task_labels
    ADD CONSTRAINT office_task_labels_label_id_fkey FOREIGN KEY (label_id) REFERENCES office_labels(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_task_labels
    ADD CONSTRAINT office_task_labels_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_task_tree_hold_members
    ADD CONSTRAINT office_task_tree_hold_members_hold_id_fkey FOREIGN KEY (hold_id) REFERENCES office_task_tree_holds(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_task_tree_hold_members
    ADD CONSTRAINT office_task_tree_hold_members_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY office_task_tree_holds
    ADD CONSTRAINT office_task_tree_holds_root_task_id_fkey FOREIGN KEY (root_task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY repositories
    ADD CONSTRAINT repositories_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_branch_policies
    ADD CONSTRAINT repository_branch_policies_repository_id_fkey FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_scripts
    ADD CONSTRAINT repository_scripts_repository_id_fkey FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_secret_bindings
    ADD CONSTRAINT repository_secret_bindings_repository_id_fkey FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_set_items
    ADD CONSTRAINT repository_set_items_repository_id_fkey FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_set_items
    ADD CONSTRAINT repository_set_items_repository_set_id_fkey FOREIGN KEY (repository_set_id) REFERENCES repository_sets(id) ON DELETE CASCADE;



ALTER TABLE ONLY repository_sets
    ADD CONSTRAINT repository_sets_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE;



ALTER TABLE ONLY session_file_reviews
    ADD CONSTRAINT session_file_reviews_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY session_step_history
    ADD CONSTRAINT session_step_history_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_document_revisions
    ADD CONSTRAINT task_document_revisions_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_documents
    ADD CONSTRAINT task_documents_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_environment_repos
    ADD CONSTRAINT task_environment_repos_task_environment_id_fkey FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_environments
    ADD CONSTRAINT task_environments_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_message_attachments
    ADD CONSTRAINT task_message_attachments_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_plan_revisions
    ADD CONSTRAINT task_plan_revisions_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_plans
    ADD CONSTRAINT task_plans_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_repositories
    ADD CONSTRAINT task_repositories_repository_id_fkey FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_repositories
    ADD CONSTRAINT task_repositories_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_review_findings
    ADD CONSTRAINT task_review_findings_run_id_fkey FOREIGN KEY (run_id) REFERENCES task_review_runs(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_review_findings
    ADD CONSTRAINT task_review_findings_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_review_runs
    ADD CONSTRAINT task_review_runs_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_commits
    ADD CONSTRAINT task_session_commits_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_git_snapshots
    ADD CONSTRAINT task_session_git_snapshots_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL;



ALTER TABLE ONLY task_session_git_snapshots
    ADD CONSTRAINT task_session_git_snapshots_task_environment_id_fkey FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_messages
    ADD CONSTRAINT task_session_messages_task_session_id_fkey FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_messages
    ADD CONSTRAINT task_session_messages_turn_id_fkey FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_subagents
    ADD CONSTRAINT task_session_subagents_task_session_id_fkey FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_session_turns
    ADD CONSTRAINT task_session_turns_task_session_id_fkey FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_sessions
    ADD CONSTRAINT task_sessions_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_status_summaries
    ADD CONSTRAINT task_status_summaries_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_step_transitions
    ADD CONSTRAINT task_step_transitions_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL;



ALTER TABLE ONLY task_step_transitions
    ADD CONSTRAINT task_step_transitions_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_usage_events
    ADD CONSTRAINT task_usage_events_session_id_fkey FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL;



ALTER TABLE ONLY task_usage_events
    ADD CONSTRAINT task_usage_events_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_walkthroughs
    ADD CONSTRAINT task_walkthroughs_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_workspace_folders
    ADD CONSTRAINT task_workspace_folders_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY task_workspace_group_members
    ADD CONSTRAINT task_workspace_group_members_workspace_group_id_fkey FOREIGN KEY (workspace_group_id) REFERENCES task_workspace_groups(id) ON DELETE CASCADE;



ALTER TABLE ONLY user_agent_profile_recent_use
    ADD CONSTRAINT user_agent_profile_recent_use_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;



ALTER TABLE ONLY utility_agent_calls
    ADD CONSTRAINT utility_agent_calls_utility_id_fkey FOREIGN KEY (utility_id) REFERENCES utility_agents(id) ON DELETE CASCADE;



ALTER TABLE ONLY workflow_step_entries
    ADD CONSTRAINT workflow_step_entries_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;



ALTER TABLE ONLY workflow_step_entry_markers
    ADD CONSTRAINT workflow_step_entry_markers_entry_id_fkey FOREIGN KEY (entry_id) REFERENCES workflow_step_entries(id) ON DELETE CASCADE;

INSERT INTO kandev_meta (key, value)
VALUES ('fixture.sentinel', 'v0.93.0-postgres-sentinel')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;



