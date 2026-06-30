# Decision Log

Architecture Decision Records (ADRs) for the Kandev project. Each decision captures the context, choice, consequences, and alternatives for significant architectural or design decisions.

Read individual ADRs for full context. Create new ones via `/record decision` or manually following the template in `0001-file-based-knowledge-system.md`.

| #    | Title                                                                                                                               | Status     | Area                        | Date       |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------- | ---------- | --------------------------- | ---------- |
| 0001 | [File-based knowledge system](0001-file-based-knowledge-system.md)                                                                  | accepted   | infra                       | 2026-03-28 |
| 0002 | [Host utility agentctl for sessionless ACP flows](0002-host-utility-agentctl-for-sessionless-flows.md)                              | accepted   | backend                     | 2026-04-08 |
| 0003 | [executors_running as the single source of truth for agent_execution_id](0003-executors-running-as-execution-id-source-of-truth.md) | accepted   | backend                     | 2026-05-03 |
| 0004 | [Task model unification — shared base, per-strategy meta, shared kernel](0004-task-model-unification.md)                            | proposed   | backend, frontend           | 2026-05-05 |
| 0005 | [Agent model unification — one `agents` table](0005-agent-model-unification.md)                                                     | proposed   | backend, frontend           | 2026-05-06 |
| 0006 | [Tier routing vs cheap_agent_profile_id coexistence](0006-tier-routing-vs-cheap-agent-profile-coexistence.md)                       | superseded | backend                     | 2026-05-11 |
| 0007 | [profiles.yaml — runtime defaults for prod / dev / e2e](0007-runtime-feature-flags.md)                                              | accepted   | backend, frontend           | 2026-05-16 |
| 0008 | [DB upgrade safety - meta table, pre-migration backup, migration logging](0008-db-upgrade-safety.md)                                | accepted   | backend                     | 2026-05-16 |
| 0009 | [Fail-closed GC semantics for filesystem and container cleanup](0009-fail-closed-gc-semantics.md)                                   | accepted   | backend                     | 2026-05-16 |
| 0010 | [Worktree copy-files — per-repo, idempotent, host-local](0010-worktree-copy-files.md)                                               | accepted   | backend, frontend           | 2026-05-19 |
| 0011 | [Transient provider errors (529 Overloaded) auto-retry with visible backoff](0011-transient-provider-error-retry.md)                | accepted   | backend, frontend           | 2026-05-30 |
| 0012 | [Service-only UI self-update](0012-service-only-self-update.md)                                                                     | accepted   | backend, frontend, cli      | 2026-05-29 |
| 0013 | [Multi-branch task support — N (repo, branch) pairs per task](0013-multi-branch-tasks.md)                                           | accepted   | backend, frontend           | 2026-06-01 |
| 0014 | [Per-CLI MCP server injection for passthrough mode](0014-passthrough-mcp-injection-strategies.md)                                   | accepted   | backend                     | 2026-05-29 |
| 0015 | [Explicit completion signal for auto-advance](0015-explicit-completion-signal-for-auto-advance.md)                                  | proposed   | backend, frontend           | 2026-06-04 |
| 0016 | [Read-only absolute file paths](0016-observed-external-file-reads.md)                                                               | accepted   | backend                     | 2026-06-14 |
| 0017 | [Resource metrics sampling](0017-resource-metrics-sampling.md)                                                                      | accepted   | backend, frontend, protocol | 2026-06-14 |
| 0018 | [Runtime settings overrides](0018-runtime-settings-overrides.md)                                                                    | accepted   | backend, frontend, cli      | 2026-06-14 |
| 0019 | [Restart supervisor owns backend restarts](0019-restart-supervisor.md)                                                              | accepted   | backend, frontend, cli      | 2026-06-14 |
| 0020 | [Pi project MCP config injection](0020-pi-project-mcp-config-injection.md)                                                          | accepted   | backend                     | 2026-06-16 |
| 0021 | [Go-served SPA with boot state](0021-go-served-spa-with-boot-state.md)                                                              | accepted   | backend, frontend, cli      | 2026-06-15 |
| 0022 | [Embedded Vite assets](0022-embedded-vite-assets.md)                                                                                | accepted   | backend, frontend, cli      | 2026-06-17 |
| 0023 | [Active workspace cookie for boot state](0023-active-workspace-cookie.md)                                                           | accepted   | backend, frontend           | 2026-06-18 |
| 0024 | [Go-fronted Vite dev mode](0024-go-fronted-vite-dev-mode.md)                                                                        | accepted   | backend, frontend, cli      | 2026-06-18 |
| 0025 | [Runtime cleanup uses `executors_running`](0025-runtime-cleanup-uses-executors-running.md)                                          | accepted   | backend                     | 2026-06-22 |
| 0026 | [Tauri desktop shell over native runtime](0026-tauri-desktop-shell.md)                                                              | accepted   | frontend, backend, cli, infra | 2026-06-23 |
| 0027 | [Replayable schema migrations across SQLite and Postgres](0027-replayable-schema-migrations.md)                                     | accepted   | backend                     | 2026-06-24 |
| 0028 | [Backend-owned task-create last-used preferences](0028-task-create-last-used-source-of-truth.md)                                    | accepted   | backend, frontend           | 2026-06-29 |
