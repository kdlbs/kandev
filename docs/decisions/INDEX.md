# Decision Log

Architecture Decision Records (ADRs) for the Kandev project. Each decision captures the context, choice, consequences, and alternatives for significant architectural or design decisions.

Read individual ADRs for full context. Create new ones via `/record decision` or manually following the template in `0001-file-based-knowledge-system.md`.

| ID   | Title                                                                                                                               | Status     | Area                        | Date       |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------- | ---------- | --------------------------- | ---------- |
| 0001 | [File-based knowledge system](0001-file-based-knowledge-system.md)                                                                  | accepted (amended 2026-07-16) | infra             | 2026-03-28 |
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
| 0025 | [Runtime cleanup uses `executors_running`](0025-runtime-cleanup-uses-executors-running.md)                                          | accepted (amended 2026-07-06) | backend            | 2026-06-22 |
| 0026 | [Tauri desktop shell over native runtime](0026-tauri-desktop-shell.md)                                                              | accepted (amended 2026-07-15) | frontend, backend, cli, infra | 2026-06-23 |
| 0027 | [Replayable schema migrations across SQLite and Postgres](0027-replayable-schema-migrations.md)                                     | accepted   | backend                     | 2026-06-24 |
| 0028 | [Backend-owned task-create last-used preferences](0028-task-create-last-used-source-of-truth.md)                                    | accepted (amended by 0041) | backend, frontend | 2026-06-29 |
| 0029 | [Release backfill and desktop diagnostics](0029-release-backfill-and-desktop-diagnostics.md)                                        | accepted (amended 2026-07-16) | infra, workflow             | 2026-07-01 |
| 0030 | [Workspace-scoped integration settings](0030-workspace-scoped-integration-settings.md)                                             | accepted   | backend, frontend           | 2026-07-01 |
| 0031 | [Office skill reference files](0031-office-skill-reference-files.md)                                                                | accepted   | backend                     | 2026-07-06 |
| 0032 | [Configurable worktree branch names](0032-configurable-worktree-branch-names.md)                                                    | accepted   | backend, frontend           | 2026-07-07 |
| 0033 | [Durable plan implementation start marker](0033-durable-plan-implementation-start.md)                                               | accepted   | backend, frontend           | 2026-07-09 |
| 0034 | [Agent Client Protocol Codex ACP Bridge](0034-agentclientprotocol-codex-acp.md)                                                     | accepted   | backend, protocol           | 2026-07-10 |
| 0035 | [Version AgentReady events by prompt generation](0035-version-agent-ready-events-by-prompt-generation.md)                          | accepted   | backend                     | 2026-07-14 |
| 0036 | [Normalize ACP shell output at the adapter boundary](0036-normalize-acp-shell-output-at-adapter-boundary.md)                        | accepted   | backend, frontend, protocol | 2026-07-14 |
| 0037 | [Resource-aware frontend unit tests](0037-resource-aware-frontend-unit-tests.md)                                                     | accepted   | frontend, infra             | 2026-07-14 |
| 0038 | [Quick Chat Repository Isolation](0038-quick-chat-repository-isolation.md)                                                           | accepted   | backend, frontend           | 2026-07-14 |
| 0039 | [Native desktop integration boundary](0039-native-desktop-integration-boundary.md)                                                  | accepted   | desktop, frontend, backend, infra | 2026-07-15 |
| 0040 | [Separate updater integrity from OS publisher identity](0040-separate-updater-integrity-from-os-publisher-identity.md)              | accepted   | desktop, infra, workflow    | 2026-07-15 |
| 0041 | [Backend-owned portable user settings](0041-backend-owned-portable-user-settings.md)                                               | accepted   | backend, frontend           | 2026-07-15 |
| 0042 | [Project shell output and fetch it on demand](0042-project-shell-output-and-fetch-on-demand.md)                                    | accepted   | backend, frontend, protocol | 2026-07-16 |
| 0043 | [Plugins read/write kandev data via capability-gated Host gRPC RPCs](0043-plugin-host-data-api.md)                                  | accepted   | backend, protocol           | 2026-07-17 |
| 2026-07-14-typed-utility-chat-sessions | [Typed Utility Chats Share the Quick Chat Session Model](2026-07-14-typed-utility-chat-sessions.md) | accepted   | backend, frontend           | 2026-07-14 |
| 2026-07-15-office-agent-execution-profile-routing | [Separate Office identity from routed execution profiles](2026-07-15-office-agent-execution-profile-routing.md) | proposed | backend, frontend | 2026-07-15 |
| 0044 | [ACP agent compatibility dialects](0044-acp-agent-compatibility-dialects.md)                                                        | accepted   | backend, protocol           | 2026-07-16 |
| 0045 | [Install-wide storage maintenance uses typed ownership providers and quarantine](0045-install-wide-storage-maintenance.md)          | accepted (amended 2026-07-22) | backend, frontend, infra | 2026-07-14 |
| 0046 | [Settings route save coordinator](0046-settings-route-save-coordinator.md)                                                          | accepted   | frontend                    | 2026-07-14 |
| 0047 | [Plugins read conversation content via a capability-gated Host RPC](0047-plugin-host-conversation-reads.md)                          | accepted   | backend, protocol           | 2026-07-21 |
| 0048 | [Plugins invoke a settings-selectable utility agent](0048-plugin-host-utility-agent-invoke.md)                                       | accepted   | backend, frontend, protocol | 2026-07-21 |
| 2026-07-23-opencode-review-evidence-trust | [Trusted OpenCode Review Evidence](2026-07-23-opencode-review-evidence-trust.md) | accepted | workflow, infra | 2026-07-23 |
| 2026-07-23-planner-direct-small-work | [Planner Direct Small Work](2026-07-23-planner-direct-small-work.md) | accepted | workflow | 2026-07-23 |
| 2026-07-23-post-commit-hook-aware-verification | [Post-Commit Hook-Aware Verification](2026-07-23-post-commit-hook-aware-verification.md) | accepted | workflow | 2026-07-23 |
| 2026-07-24-operator-owned-agent-launcher-settings | [Operator-Owned Agent Launcher Settings](2026-07-24-operator-owned-agent-launcher-settings.md) | accepted | backend, frontend, protocol | 2026-07-24 |
| 2026-07-18-turn-configuration-snapshots | [Attribute runtime configuration to turns](2026-07-18-turn-configuration-snapshots.md) | accepted | backend, frontend | 2026-07-18 |
| 2026-07-22-gpg-signed-release-tags | [GPG-signed release tags](2026-07-22-gpg-signed-release-tags.md) | accepted | infra, workflow | 2026-07-22 |
| 2026-07-19-reject-mcp-actions-on-raw-websocket | [Reject MCP Actions on the Raw WebSocket](2026-07-19-reject-mcp-actions-on-raw-websocket.md) | accepted | backend, protocol | 2026-07-19 |
| 2026-07-19-workspace-symlink-entries | [Treat Nested Workspace Symlinks as Entries](2026-07-19-workspace-symlink-entries.md) | accepted | backend, infra | 2026-07-19 |
| 2026-07-20-explicit-local-repository-trust | [Explicit Local Repository Trust](2026-07-20-explicit-local-repository-trust.md) | accepted | backend, frontend | 2026-07-20 |
| 2026-07-20-provider-neutral-remote-repositories | [Provider-neutral remote repositories](2026-07-20-provider-neutral-remote-repositories.md) | accepted | backend, frontend, protocol | 2026-07-20 |
| 2026-07-20-repository-provider-origin-identity | [Persist Provider Origin In Repository Identity](2026-07-20-repository-provider-origin-identity.md) | accepted | backend, frontend | 2026-07-20 |
| 2026-07-21-portable-status-bar-order | [Portable Status Bar Order](2026-07-21-portable-status-bar-order.md) | accepted | backend, frontend, protocol | 2026-07-21 |
| 2026-07-21-work-item-reference-search | [Backend-normalized work-item references](2026-07-21-work-item-reference-search.md) | accepted | backend, frontend, protocol | 2026-07-21 |
