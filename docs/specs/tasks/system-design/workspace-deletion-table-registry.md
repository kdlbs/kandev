---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Workspace Deletion Table Registry

## Authority

The typed `TableEntry` registry at
`apps/backend/internal/task/archivecascade/registry/tables.go` is the sole
schema authority. The inventory rows below are generated from it by
`registry/export-markdown` and carry its `source_sha256` in the header.
They explain owner predicates and lifecycle outcomes but do
not independently define columns, constraints, DDL, or retention semantics. A
production table missing from the generated inventory fails verification.
Temporary migration targets ending in `_new` and formatter placeholders such as
`%s` are not live tables and are rejected if left after a migration.

Predicates are evaluated from the locked workspace/task/workflow/repository/
session/run/profile/watch ID sets captured by `Store.DeleteWorkspace`. `IN S`
means membership in that immutable sorted set. Child rows are locked/deleted
before their named parent. `retain` means the command cannot delete the row.

## Transactionally deleted tables

| Order | Exact table names | Owner predicate or join | Outcome |
| ---: | --- | --- | --- |
| 10 | `workflow_step_entry_markers` | `entry_id IN workflow_step_entries.id` for deleted tasks | delete |
| 11 | `task_review_findings` | `run_id IN task_review_runs.id` for deleted tasks | delete |
| 12 | `task_session_prompt_seq` | `task_session_id IN S(session)` | delete |
| 13 | `task_shares` | `task_session_id IN S(session)` | delete |
| 14 | `queue_session_locks`, `queue_session_state` | `session_id IN S(session)` | delete |
| 15 | `queued_messages`, `pending_moves`, `lifecycle_queue_generations` | `session_id IN S(session)` or `task_id IN S(task)` | delete |
| 16 | `dynamic_route_attempts`, `dynamic_route_states` | `session_id IN S(session)` | delete |
| 17 | `session_file_reviews`, `session_step_history` | `session_id IN S(session)` | delete |
| 18 | `task_session_commits` | `session_id IN S(session)` | delete |
| 19 | `task_session_git_snapshots` | `session_id IN S(session)` or `task_environment_id IN S(environment)` | delete |
| 20 | `task_session_messages`, `task_session_turns`, `task_session_subagents` | `task_session_id IN S(session)` or `task_id IN S(task)` | delete |
| 21 | `task_environment_repos` | `task_environment_id IN S(environment)` except protected shared-group resources | delete after snapshot/proof; rehomed resources survive |
| 22 | `executors_running` | `session_id IN S(session)` or `task_id IN S(task)`; only after stop proof or retained stop evidence | delete after immutable runtime-handle snapshot is retained |
| 23 | `task_sessions`, `task_environments` | `task_id IN S(task)` except environments protected by a surviving shared group | delete after immutable session/environment/resource snapshot is retained; rehomed owners survive |
| 24 | `workflow_step_participants`, `workflow_step_decisions` | `task_id IN S(task)` or `step_id IN S(workflow_step)` | delete |
| 25 | `workflow_step_entries`, `task_step_transitions` | `task_id IN S(task)` | delete |
| 26 | `task_plan_revisions`, `task_plans`, `task_walkthroughs`, `task_document_revisions`, `task_documents` | `task_id IN S(task)` | delete |
| 27 | `task_review_runs`, `task_usage_events` | `task_id IN S(task)` | delete |
| 28 | `task_message_attachments` | `workspace_id = W` or resolved `task_id/session_id IN S` | delete |
| 29 | `task_repositories`, `task_workspace_folders`, `task_status_summaries` | `task_id IN S(task)` | delete |
| 30 | `task_remote_contribution_associations` | `workspace_id = W OR task_id IN S(task)` | delete operation-owned required creation claims; post-Complete provider projections are separate |
| 31 | `task_blockers` | `task_id IN S(task) OR blocker_task_id IN S(task)`; lock both endpoint task sets | delete incident rows only |
| 32 | `task_comments`, `task_delivery_ledger`, `user_terminals` | `task_id IN S(task)` | delete |
| 33 | `office_task_labels` | `task_id IN S(task)` or `label_id IN S(office_label)` | delete |
| 34 | `office_task_tree_hold_members` | `hold_id IN S(tree_hold)` or `task_id IN S(task)` | delete |
| 35 | `office_run_skills`, `run_events`, `office_run_route_attempts`, `parent_child_wake_receipts` | `run_id/parent_run_id/child_run_id IN S(run)` | delete |
| 36 | `office_cost_events`, `office_activity_log`, `office_channels` | `task_id/session_id/run_id IN S` or `workspace_id = W` | delete |
| 37 | `agent_wakeup_requests`, `agent_continuation_summaries`, `office_agent_memory`, `office_agent_instructions`, `office_agent_runtime` | `run_id IN S(run)` or `agent_profile_id IN S(profile)` | delete |
| 38 | `office_routine_runs`, `office_routine_triggers` | `routine_id IN S(routine)` | delete |
| 39 | `office_task_tree_holds` | `workspace_id = W` or `root_task_id IN S(task)` | delete |
| 40 | `runs` | bound task/session/profile resolves to `W` | delete |
| 41 | `office_labels`, `office_approvals`, `office_budget_policies`, `office_provider_health`, `office_workspace_routing`, `office_workspace_settings`, `office_workspace_governance`, `office_onboarding` | `workspace_id = W` | delete |
| 42 | `office_routines`, `office_skills`, `office_projects` | `workspace_id = W` | delete |
| 43 | `task_workspace_group_members` | `workspace_group_id IN S(group)` or `task_id IN S(task)` | delete W-owned edges; preserve foreign-member edges while rehoming |
| 44 | `task_workspace_groups` | `workspace_id = W` | rehome when a foreign member survives; otherwise delete |
| 45 | `office_config_sync_manifest` | `workspace_id = W` | delete |
| 46 | `office_config_sync_configs` | `workspace_id = W` | delete |
| 47 | `automation_runs`, `automation_repositories`, `automation_triggers` | `automation_id IN S(automation)` or `task_id/session_id/repository_id IN S` | delete |
| 48 | `automations` | `workspace_id = W` | delete |
| 49 | `agent_profile_mcp_configs`, `dynamic_agent_routes`, `dynamic_agent_profiles` | parent profile/agent resolves to `W` | delete |
| 50 | `agent_profiles` | `workspace_id = W` or parent `agent_id IN S(agent)`; null/global profiles survive | delete matching rows |
| 51 | `agents` | `workspace_id = W`; null/global agents survive | delete matching rows |
| 52 | `repository_set_items` | `repository_set_id IN S(repository_set)` or `repository_id IN S(repository)` | delete |
| 53 | `repository_secret_bindings`, `repository_scripts`, `repository_branch_policies` | `repository_id IN S(repository)` | delete |
| 54 | `repository_sets` | `workspace_id = W` | delete |
| 55 | `repositories` | `workspace_id = W` | delete after all repository children |
| 56 | `workflow_steps` | `workflow_id IN S(workflow)` | delete after participants |
| 57 | `workflows` | `workspace_id = W` | delete after all workflow children |
| 58 | `github_issue_watch_tasks`, `github_review_pr_tasks` | parent watch ID in deleted GitHub watch set or `task_id IN S(task)` | delete |
| 59 | `github_task_ci_options`, `github_task_ci_pr_state`, `github_task_pr_automation_options`, `github_task_prs`, `github_pr_watches` | `task_id IN S(task)`, `workspace_id = W`, or `repository_id IN S(repository)` | delete |
| 60 | `github_issue_watches`, `github_review_watches`, `github_action_presets`, `github_workspace_settings`, `github_workspace_connections`, `github_user_connections`, `github_user_connection_versions`, `github_auth_flows` | `workspace_id = W` | delete |
| 61 | `github_app_registration_flows`, `github_app_import_preparations` | `workspace_id = W` | delete; provenance flow only |
| 62 | `gitlab_issue_watch_tasks`, `gitlab_review_mr_tasks` | parent watch ID in deleted GitLab watch set or `task_id IN S(task)` | delete |
| 63 | `gitlab_task_mr_options`, `gitlab_task_mr_state`, `gitlab_task_mr_automation_options`, `gitlab_task_mrs`, `gitlab_mr_watches` | `task_id IN S(task)` or `repository_id IN S(repository)` | delete |
| 64 | `gitlab_issue_watches`, `gitlab_review_watches`, `gitlab_action_presets`, `gitlab_configs`, `gitlab_mention_scopes` | `workspace_id = W` | delete |
| 65 | `azure_devops_work_item_watch_tasks`, `azure_devops_pull_request_watch_tasks` | parent watch ID in deleted Azure watch set or `task_id IN S(task)` | delete |
| 66 | `azure_devops_task_prs`, `azure_devops_task_work_items` | `task_id IN S(task)`, `workspace_id = W`, or `repository_id IN S(repository)` | delete |
| 67 | `azure_devops_work_item_watches`, `azure_devops_pull_request_watches`, `azure_devops_configs` | `workspace_id = W` | delete |
| 68 | `jira_issue_watch_tasks`, `linear_issue_watch_tasks`, `sentry_issue_watch_tasks` | parent watch ID in provider watch set or `task_id IN S(task)` | delete |
| 69 | `jira_issue_watches`, `linear_issue_watches`, `sentry_issue_watches` | `workspace_id = W` | delete |
| 70 | `jira_configs`, `linear_configs`, `sentry_configs` | `workspace_id = W` | delete |
| 71 | `workflow_sync_configs` | `workspace_id = W` | delete |
| 72 | `quick_terminal_tabs`, `quick_terminal_workspace_sequences` | `workspace_id = W` | delete |
| 73 | `secrets` | `workspace_id = W`; user/global/null workspace rows survive | delete matching rows through secret transaction API |
| 74 | `tasks` | `workspace_id = W`; only after task tombstones/outbox are durable | delete |
| 75 | `workspaces` | `id = W`; only after every transactional child | delete last |

Group deletion is conditional, not a wildcard cascade. Before order 21, the
transaction partitions each affected group by member workspace while locking
group, membership, environment, repository, cleanup-marker, and ownership
generation rows. If any member survives outside W, it deletes only W-owned
membership edges, rehomes the group, shared environment, and repositories to
the lexicographically smallest surviving workspace/member, increments every
ownership generation atomically, and removes those resources from deletion
sets. Foreign edges and resources therefore retain valid live FKs. If none
survives, later rows delete the group and owned resources. Reconciliation may
restore stewardship to a newly selected surviving member, never to W or a
dangling group reference; stale cleanup claims fail their generation fence.

## Retained aggregate, recovery, and diagnostic tables

This generated section mirrors the typed registry's retained rows. Each name maps
to one typed `TableEntry`; no alias is live. `Owner predicate` selects rows whose
historical owner is `W`; `retain` means workspace deletion cannot delete the row.
The typed entry, not this Markdown table, owns columns, constraints, retention,
and FK invariants. `Tier` mirrors the global lock tier.

| Exact table | Owner predicate | Tier | Deletion lifecycle and FK invariant |
| --- | --- | ---: | --- |
| `task_creation_operations` | `workspace_id = W` | 4 | retain every state and replay result; no cascading FK to task/workspace |
| `task_creation_steps` | `operation_id IN task_creation_operations(W)` | 7 | retain plan, evidence, and compensation; operation FK is retain/restrict |
| `task_external_id_release_operations` | `workspace_id = W` | 4 | retain result indefinitely for replay; no task/workspace cascade |
| `task_archive_operations` | `root_workspace_id = W` | 4 | retain every phase/result; no task/workspace cascade |
| `task_archive_operation_members` | `workspace_id = W OR operation_id IN task_archive_operations(W)` | 7 | retain immutable membership; operation FK is retain/restrict |
| `task_archive_operation_scopes` | `operation_id IN task_archive_operations(W) OR scope_workspace_id = W` | 4 | retain immutable operation/workspace barriers, generation, phase, and lease; operation FK is retain/restrict; no workspace cascade |
| `task_archive_operation_groups` | `workspace_id = W OR operation_id IN task_archive_operations(W)` | 13 | retain the sole immutable full group/member snapshot; no separate snapshot table exists |
| `task_cleanup_jobs` | `workspace_id = W` | 8 | retain cleanup/restore state and proof; no task/workspace cascade |
| `task_group_cleanup_jobs` | `workspace_id = W` | 13 | retain cleanup/restore state and proof; no group/task/workspace cascade |
| `task_resource_cleanup_jobs` | `workspace_id = W` | 8 | retain resource cleanup/transfer state; no task/workspace cascade |
| `task_resource_cleanup_snapshots` | `workspace_id = W` | 8 | retain immutable runtime/session/environment/repository/worktree handles, ownership/lifecycle generations, HMAC, and Git-registration/path identity; no mutable-resource cascade |
| `task_archive_resource_markers` | `workspace_id = W` | 14 | retain per-resource ownership/CAS marker and proof state; no mutable-resource cascade |
| `task_resume_materialization_claims` | `workspace_id = W OR task_id IN S(task)` | 8 (ordinal 76) | retain states `planned|running|unknown|retry_wait|succeeded|blocked|cancelled|transferred`; workspace deletion cancels unstarted or transfers ambiguous/in-flight with `workspace_deleted`, increments generation, and fences stale I/O; task/workspace FKs are retain/restrict |
| `task_launch_dispatch_claims` | `workspace_id = W OR task_id IN S(task)` | 8 | retain launch claim/lease/generation/runtime handle and unknown evidence; no mutable task/session/workspace cascade |
| `task_resource_cleanup_snapshot_evidence` | `snapshot_id IN task_resource_cleanup_snapshots(W)` | 14 | retain append-only authenticated stop/Git/provider evidence versions; unique evidence key and snapshot FK are retain/restrict |
| `automation_task_cleanup_jobs` | `workspace_id = W` | 8 | retain canonical deletion provenance and result; no automation/task cascade |
| `task_lifecycle_event_outbox` | `workspace_id = W` | 16 | retain immutable task epoch/revision/event/tombstone/payload rows from pending through delivered, including session events as rows with `session_id`, `session_event_id`, terminal session state, and task-tombstone suppression; no task/workspace cascade |
| `task_parent_mutation_results` | `workspace_id = W` | 6 | retain canonical result/replay bytes, request/actor hashes, historical task/lifecycle identity, nullable event identity, and digest; FK to retained `task_lifecycle_revisions(task_id,lifecycle_epoch)` is `RESTRICT`; no mutable-task/workspace cascade |
| `task_detach_mutation_results` | `workspace_id = W` | 6 | retain canonical result/replay bytes, request/actor hashes, historical task/lifecycle identity, nullable event identity, and digest; FK to retained `task_lifecycle_revisions(task_id,lifecycle_epoch)` is `RESTRICT`; no mutable-task/workspace cascade |
| `task_lifecycle_revisions` | `workspace_id = W` | 6 | retain epoch, revision, terminal/tombstone state, and next event identity; no task/workspace cascade |
| `task_lifecycle_delivery_watermarks` | `workspace_id = W` | 6 | retain contiguous `(revision,queue_sequence)` delivery cursor by task epoch; lock/restrict revision FK; no task/workspace cascade |
| `task_lifecycle_historical_gaps` | `workspace_id = W` | 6 | retain immutable pre-epoch gap ranges and provenance; revision FK is retain/restrict |
| `workspace_lifecycle_event_outbox` | `workspace_id = W` | 16 | retain immutable workspace epoch/event/payload rows from pending through delivered; no workspace cascade |
| `workspace_deletion_operations` | `workspace_id = W` | 4 | retain command, result, tombstones, and aftermath state; no workspace cascade |
| `workspace_deletion_steps` | `operation_id IN workspace_deletion_operations(W)` | 16 | retain pending/running/retry_wait/unknown/succeeded steps and evidence; `group_rehome` carries resource snapshot, idempotency key, generation/lease/due state, and `rehome_pending` disposition on retry; operation FK is retain/restrict; no workspace cascade; writer is workspace-deletion aftermath worker and Runtime |
| `workspace_config_path_claims` | `workspace_id = W` | 15 | retain active/deleting/quarantined/released marker history; deletion transitions `active -> deleting -> quarantined -> released`; no FK to mutable workspace and no workspace cascade; writer is `workspaceconfig` claim transition |
| `task_archive_legacy_candidates` | `workspace_id = W` for workspace candidates; `workspace_id IS NULL` for installation candidates | 3 | retain active/dismissed barrier evidence; diagnostic FK is retain/restrict |
| `task_archive_migration_diagnostics` | `id IN task_archive_legacy_candidates(W).diagnostic_id`, or `scope=installation AND workspace_id IS NULL` | 3 | retain blocked/resolved diagnostic; no workspace cascade |
| `task_archive_resolution_audit` | `diagnostic_id IN task_archive_migration_diagnostics(W)`; installation diagnostics use `workspace_id IS NULL` | 17 | immutable authorized-resolution audit; diagnostic FK is retain/restrict |
| `task_archive_security_audit` | `diagnostic_id IN task_archive_migration_diagnostics(W)` when nonnull; nullable/global for pre-auth installation scope | 17 | immutable denial audit; nullable installation-scope rows are global and retained |
| `storage_quarantine_entries` | `workspace_id = W` when nonnull | 14 | retain quarantine identity until separately proven purged; nullable installation rows are global |
| `utility_agent_calls` | `workspace_id = W OR task_id IN S(task)` when nonnull | 8 | retain external-call evidence/result; nullable installation calls are global |

All names above are created by work order 01 and are canonical from their first
migration. A pre-merge/prototype database containing
`task_archive_group_cleanup_jobs`, `task_archive_event_outbox`,
`task_archive_legacy_diagnostics`, `task_archive_blocked_candidates`,
`task_archive_legacy_audit`, or `task_archive_group_snapshots` is incompatible:
the migration atomically copies validated rows into the canonical table, rewrites
declared FKs, drops the alias, and verifies row/hash equality before commit.
Fresh schemas never create aliases; replayed migrations are idempotent. Runtime
startup rejects a database where an alias remains or both forms contain rows.

## Migration-only source aliases

These names/columns are never present in a fresh schema and are not deletion
targets. They are nevertheless part of the canonical migration catalog:

| Source alias | Canonical destination | Ownership/precedence | Cutover and rollback |
| --- | --- | --- | --- |
| `task_session_worktrees` rows (`session_id`, `task_environment_id`, `worktree_id`, path, branch, timestamps) | `task_environment_repos` plus normalized `task_environments` and `task_sessions.task_environment_id` | canonical repository row wins; otherwise surviving flat environment row; otherwise live-session then newest row per `(repository, branch)` slot; deleted/terminal/orphan rows are evidence, active unresolvable rows fail closed | exclusive migration transaction builds shadow tables, copies full identity/hash inventory, validates one owner/FKs, swaps, drops alias; any failpoint or commit-unknown preserves old schema or completes an attested replay |
| `task_environments.repository_id`, `worktree_id`, `worktree_path`, `worktree_branch` | `task_environment_repos` | canonical repository row wins; otherwise flat environment fields win over a legacy session reference for the same physical identity | copied with source precedence and identity hash; dropped only after validation; rollback restores columns and values atomically |
| `task_resource_cleanup_jobs.trigger = session_delete` from preview builds | no destination; invalid preview residue | never an ownership source; task-level cleanup history remains | removed in the same migration before Runtime claims jobs; row-count/hash failpoint rolls back; replay is idempotent |

The migration records demoted legacy physical identities as historical evidence
without deleting directories, Git registrations, or branches. It verifies source
column hashes, destination row hashes, precedence decisions, FK rewrites, and
drop order on SQLite and PostgreSQL. Task 01 implements these catalog entries;
Task 04 independently discovers the aliases/columns and verifies fresh, replay,
copy-fail, FK-fail, drop-fail, rollback, and commit-unknown behavior.

`workspace_config_path_claims` has no migration source alias: it is introduced as
the canonical table and any same-named legacy shape must be upgraded in place or
rejected by schema attestation. Its registry row owns path-claim insert,
transition, and release writers. Task 04 fixtures create an owned claim and an
unrelated claim, pause a tier-15 writer against workspace deletion, and assert
that the released owned claim remains retained and blocks path reuse until the
recorded lifecycle permits it on both dialects.

## Installation-global and user-global tables

These exact live tables are outside workspace ownership and are retained. A
nullable reference is provenance or user scope, not ownership, unless the table
appears in the transactional section:

- infrastructure: `executors`, `executor_profiles`, `environments`,
  `dynamic_installation_keys`, `dynamic_resource_circuits`, `kandev_meta`,
  `workflow_templates`;
- GitHub installation provenance: `github_app_registrations` and
  `github_webhook_deliveries`, including registrations whose nullable
  `created_for_workspace_id = W`;
- auth/user: `users`, `user_agent_profile_recent_use`, `auth_identities`,
  `auth_sessions`, `auth_api_tokens`, `auth_invites`, `auth_hostname_cache`;
- user notifications: `notification_providers`, `notification_subscriptions`,
  `notification_deliveries`, `notification_migrations`;
- installation configuration: `custom_prompts`, `editors`, `settings`,
  `runtime_flag_overrides`, `plugin_marketplace_source`, `plugin_settings`,
  `plugin_state`, `plugin_user_state`;
- operations: `storage_maintenance_runs`, `storage_temp_artifacts`,
  `telemetry_activations`, `utility_agents`, `office_inbox_dismissals`.

## Completeness and race proof

One generated typed catalog drives fresh DDL, replay migrations, Store inventory,
retained-FK validation, and both-dialect tests. The verifier independently
discovers production `CREATE TABLE`, migration rename, FK, and SQL references,
rejects scratch/alias names, and compares the exact name/column set. Every
transactional row supplies executable ownership, lock tier/ordinal, and delete
SQL; every retained row supplies its owner selector and no-cascade invariant.
No separately maintained name list is permitted.

Fixtures seed one owned and one unrelated row for every transactional predicate,
including both blocker directions and every parent-only join. They seed nullable
GitHub registration provenance, its webhook delivery, user/global profiles and
secrets, retained operation/diagnostic rows, and assert exact survival. A race
suite pauses each writer after its ownership read, starts workspace deletion,
and proves the writer either commits before inventory and is deleted or observes
the reservation and performs no write. No wildcard table group or runtime table
name is accepted by the implementation registry.
Fresh-schema and replay-migration tests assert canonical names only, exact
row/hash preservation from each rejected alias, rollback at every copy/FK/drop
failpoint, and startup rejection of mixed aliases. Config-claim tests lock tier
15, prove `active -> deleting -> quarantined -> released`, retain the released
claim after workspace-row deletion, and reject path reuse until that lifecycle
permits it.

Work order 01 implements the typed registry, selectors, locks, and delete SQL.
Work order 04 independently derives the live production schema and SQL-reference
set, compares every name/column to the registry, executes all predicates on both
dialects, and owns GitHub registration/delivery retention proof.
