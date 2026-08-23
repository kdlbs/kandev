# Legacy Feature Specification Catalog

This catalog locates specifications that predate the system-oriented structure.
Do not add new standalone feature specifications here. New system specifications
follow [`docs/specs/README.md`](README.md) and live under an owning system in
`requirements/` or `system-design/`. Product-wide specifications live under
`docs/specs/product/`.

Each legacy specification remains authoritative until its system `README.md`
names a replacement.

The bar: an agent given only a spec (no source code) should be able to either reimplement the feature or test the existing system for conformance. See `.agents/skills/spec/SKILL.md` for the workflow and template.

**Status:** `draft` (being written) · `approved` (accepted design, ready to build) · `building` (in active development) · `shipped` (implemented, spec matches code) · `archived` (deprecated).

**`needs-upgrade`** in a spec's frontmatter flags template sections that the original sources did not cover and should be filled in from code (Data model, API surface, State machine, Permissions, Failure modes, Persistence guarantees). All office specs have been brought to the implementability bar; this flag is only used for newly-drafted specs that need a code-driven fill-in pass.

---

## office/ — autonomous agent management

The office umbrella covers kandev's autonomous-agent product surface: workspaces of long-running agents that pick up tasks, coordinate via handoffs, and report through a dashboard.

| Spec | Status |
|---|---|
| [overview](office/overview.md) | draft |
| [agents](office/agents.md) | draft |
| [tasks](office/tasks.md) | draft |
| [scheduler](office/scheduler.md) | draft |
| [runtime](office/runtime.md) | draft |
| [routing](office/routing.md) | archived |
| [per-agent + per-role tier selection](office-agent-tier-routing/spec.md) | draft |
| [costs](office/costs.md) | in-progress |
| [dashboard](office/dashboard.md) | draft |
| [live-updates](office/live-updates.md) | draft |
| [inbox](office/inbox.md) | draft |
| [assistant](office/assistant.md) | draft |
| [automation-runs](office/automation-runs.md) | draft |
| [automations-settings](office/automations-settings.md) | draft |
| [testing](office/testing.md) | shipped |
| [unread-divider](office/unread-divider.md) | shipped |

## platform/ — cross-cutting capabilities

Product-wide capabilities that are not tied to a single feature area.

| Spec | Status |
|---|---|
| [startup-configuration-parity](platform/startup-configuration-parity.md) | shipped |
| [agent-runtime-availability](platform/agent-runtime-availability.md) | draft |
| [background-work-liveness](platform/background-work-liveness.md) | shipped |
| [setup-launch-timeout](platform/setup-launch-timeout.md) | approved |
| [task-sleep-inhibition](platform/task-sleep-inhibition.md) | building |
| [i18n](platform/i18n.md) | building |
| [traditional-chinese-locales](platform/traditional-chinese-locales.md) | building |
| [mid-turn-steering](platform/mid-turn-steering.md) | shipped |
| [plugins](plugins/spec.md) | draft |
| [plugins — authoring experience](plugins/authoring-experience.md) | draft |
| [plugins — marketplace](plugins/marketplace.md) | building |
| [plugins — agent tools](plugins/agent-tools.md) | draft |
| [plugins — Voice extraction host prerequisites](plugins/voice-extraction-host.md) | shipped |
| [plugins — Voice Mode leaves core](plugins/voice-extraction.md) | shipped |
| [plugin-nav-sidebar-footer](plugin-nav-sidebar-footer/spec.md) | draft |
| [semantic-notifications](platform/notifications.md) | shipped |
| [workspace-git-status](platform/workspace-git-status.md) | approved |
| [git-subprocess-admission](platform/git-subprocess-admission.md) | building |
| [git-credential-lease-reissue](git-credential-lease-reissue/spec.md) | shipped |
| [bounded-task-status-delivery](platform/bounded-task-status-delivery.md) | approved |
| [diagnostic-logging](platform/diagnostic-logging.md) | approved |
| [expected runtime log severity](platform/expected-runtime-log-severity.md) | building |
| [provider-error-recovery](platform/provider-error-recovery.md) | draft |
| [duplicated-tab-stale-data](fix-duplicated-tab-stale-data/spec.md) | building |
| [health-endpoint-version](health-endpoint-version/spec.md) | building |
| [go-dev-launcher](go-dev-launcher/spec.md) | draft |

## tasks/ — task & workflow model

The task and workflow system has a system index and new authoritative
requirement and design paths in [tasks/README.md](tasks/README.md).
All task and workflow specifications are now indexed under that system
directory; this legacy catalog has no remaining task-owned editable sources.

## agents/ — agent governance

Roles, governance gates, and granular permissions that apply across human users and office agents.

| Spec | Status |
|---|---|
| [runtime-updates](agents/runtime-updates.md) | approved |
| [profile-disable](agents/profile-disable.md) | draft |
| [settings-profile-layout](agents/settings-profile-layout.md) | shipped |
| [dynamic-provider-options](agents/dynamic-provider-options.md) | shipped |
| [dynamic-agent-routing](agents/dynamic-agent-routing.md) | draft |
| [dynamic-agent-routing-rollout-blockers](agents/dynamic-agent-routing-rollout-blockers.md) | draft |
| [dynamic-agent-telemetry-routing](agents/dynamic-agent-telemetry-routing.md) | draft |
| [utility-agent-profiles](agents/utility-agent-profiles.md) | approved |
| [collapsible-agent-blocks](agents/collapsible-agent-blocks.md) | draft |
| [roles](agents/roles.md) | shipped |
| [governance](agents/governance.md) | shipped |
| [granular-permissions](agents/granular-permissions.md) | draft |
| [external-permission-resolution](agents/external-permission-resolution.md) | draft |
| [git-operations-permission-boundary](agents/git-operations-permission-boundary.md) | shipped |

## integrations/ — external service integrations

Per-workspace credentials and triage triggers for external services.

| Spec | Status |
|---|---|
| [azure-devops-integration](azure-devops-integration/spec.md) | shipped |
| [bitbucket-plugin](bitbucket-plugin/spec.md) | approved |
| [slack](integrations/slack.md) | archived — moved to `kandev-plugin-slack` |
| [external-mcp](integrations/external-mcp.md) | draft |
| [mcp-tool-argument-validation](integrations/mcp-tool-argument-validation.md) | shipped |
| [provider-aware-review-automation](integrations/provider-aware-review-automation.md) | approved |
| [github-authentication](integrations/github-authentication.md) | draft |
| [gitlab-integration](gitlab-integration/spec.md) | shipped |
| [gitlab-mr-status-chip](gitlab-mr-status-chip/spec.md) | draft |
| [gitlab-mr-task-list-badges](gitlab-mr-task-list-badges/spec.md) | draft |
| [gitlab-workflow-sync](gitlab-workflow-sync/spec.md) | shipped |
| [jira-status-filter](jira-status-filter/spec.md) | shipped |
| [pr-outcome-attribution](pr-outcome-attribution/spec.md) | shipped |
| [enable-disable-toggle](integrations/enable-disable-toggle.md) | shipped |
| [clickable-integration-cards](integrations/clickable-integration-cards.md) | shipped |

## workspaces/ — workspace lifecycle

| Spec | Status |
|---|---|
| [creation](workspaces/creation.md) | building |
| [deletion](workspaces/deletion.md) | shipped |
| [local-repositories](workspaces/local-repositories.md) | shipped |
| [worktree-branch-templates](workspaces/worktree-branch-templates.md) | building |
| [repository-secrets](workspaces/repository-secrets.md) | shipped |
| [secret-scope-transfer](workspaces/secret-scope-transfer.md) | shipped |

## costs/ — cost tracking & budgets

Subscription quota tracking and per-agent cheap-model profile routing.

| Spec | Status |
|---|---|
| [subscription-usage](costs/subscription-usage.md) | draft |
| [cheap-model-profiles](costs/cheap-model-profiles.md) | shipped |

## ui/ — cross-cutting UI features

| Spec | Status |
|---|---|
| [kanban-auto-hide-empty-columns](kanban-auto-hide-empty-columns/spec.md) | shipped |
| [workspace-active-first-order](ui/workspace-active-first-order.md) | shipped |
| [ci-pr-automation](ui/ci-pr-automation.md) | building |
| [github-pr-review-actions](ui/github-pr-review-actions.md) | shipped |
| [github-saved-query-defaults](ui/github-saved-query-defaults.md) | shipped |
| [pr-detail-header-width](pr-detail-header-width/spec.md) | shipped |
| [pr-task-status-summary](ui/pr-task-status-summary.md) | shipped |
| [comment-markdown](ui/comment-markdown.md) | shipped |
| [resizable-markdown-tables](ui/resizable-markdown-tables.md) | building |
| [transcript-auto-scroll](ui/transcript-auto-scroll.md) | building |
| [task-transcript-history-visibility](ui/task-prompt-transcript-visibility.md) | implemented |
| [clarification-context](ui/clarification-context.md) | shipped |
| [empty-turn-notice](ui/empty-turn-notice.md) | shipped |
| [acp-shell-command-output](ui/acp-shell-command-output.md) | shipped |
| [acp-model-configuration-summary](ui/acp-model-configuration-summary.md) | shipped |
| [context-window-unmeasured-state](ui/context-window-unmeasured-state.md) | building |
| [review-file-status](ui/review-file-status.md) | building |
| [submodule-review](ui/submodule-review.md) | shipped |
| [review-markdown-preview](ui/review-markdown-preview.md) | draft |
| [sidebar-view-creation](ui/sidebar-view-creation.md) | shipped |
| [command-panel sidebar task reveal](ui/command-panel-sidebar-task-reveal.md) | draft |
| [sidebar-empty-task-alignment](ui/sidebar-empty-task-alignment.md) | building |
| [sidebar-task-completion-icons](ui/sidebar-task-completion-icons.md) | shipped |
| [sidebar-queued-prompt-count](ui/sidebar-queued-prompt-count.md) | shipped |
| [session-tab-delete-feedback](ui/session-tab-delete-feedback.md) | shipped |
| [terminal-close-feedback](ui/terminal-close-feedback.md) | shipped |
| [message-favorite-star-mobile-size](ui/message-favorite-star-mobile-size.md) | shipped |
| [message-metadata-overflow](ui/message-metadata-overflow.md) | shipped |
| [slash-command-composer](ui/slash-command-composer.md) | shipped |
| [subagent-observability](ui/subagent-observability.md) | building |
| [entity-reference-composer](ui/entity-reference-composer.md) | draft |
| [agent-launch-prompt-composer](ui/agent-launch-prompt-composer.md) | shipped |
| [mermaid-rendering](ui/mermaid-rendering.md) | shipped |
| [agent-rich-output](agent-rich-output/spec.md) | shipped |
| [message-queue-auto-merge](ui/message-queue-auto-merge.md) | shipped |
| [message-queue-management](ui/message-queue-management.md) | shipped |
| [message-queue-merge](ui/message-queue-merge.md) | shipped |
| [message-queue-reorder](ui/message-queue-reorder.md) | building |
| [message-queue-run](ui/message-queue-run.md) | shipped |
| [message-queue-send-now](ui/message-queue-send-now.md) | shipped |
| [settings-manual-save](ui/settings-manual-save.md) | shipped |
| [settings-discovery](ui/settings-discovery.md) | shipped |
| [settings-prompt-editor](ui/settings-prompt-editor.md) | shipped |
| [settings-typography](settings-typography/spec.md) | draft |
| [executor-settings-card-spacing](ui/executor-settings-card-spacing.md) | shipped |
| [quick-chat-elevation](ui/quick-chat-elevation.md) | building |
| [transcript-navigation-settings](ui/transcript-navigation-settings.md) | shipped |
| [voice-mode-task-behavior](ui/voice-mode-task-behavior.md) | archived |
| [app-status-bar](ui/app-status-bar.md) | shipped |
| [quick-terminal](quick-terminal/spec.md) | shipped |
| [mobile-task-navigation](ui/mobile-task-navigation.md) | shipped |
| [adaptive-kanban](ui/adaptive-kanban.md) | shipped |
| [task-layout-profiles](ui/task-layout-profiles.md) | draft |
| [port-forwarding-discovery](ui/port-forwarding-discovery.md) | building |
| [port-proxy-browser-panel](port-proxy-browser-panel/spec.md) | shipped |
| [task-surface-refresh](ui/task-surface-refresh.md) | draft |
| [walkthrough-navigation-layout](walkthrough-navigation-layout/spec.md) | shipped |
| [walkthrough-feedback-controls](walkthrough-feedback-controls/spec.md) | shipped |
| [changes-walkthrough-toolbar-width](changes-walkthrough-toolbar-width/spec.md) | shipped |
| [agent-message-comments](ui/agent-message-comments.md) | shipped |
| [external-vcs-file-links](ui/external-vcs-file-links.md) | shipped |
| [task-listing-display-preferences](ui/task-listing-display-preferences.md) | shipped |
| [sidebar-archived-filter](ui/sidebar-archived-filter.md) | draft |
| [sidebar-last-activity-sort](ui/sidebar-last-activity-sort.md) | draft |
| [task-workspace-content-search](ui/task-workspace-content-search.md) | shipped |
| [file-tree-chat-context](ui/file-tree-chat-context.md) | shipped |
| [task-review-shortcut](ui/task-review-shortcut.md) | approved |
| [embedded-vscode-executor-availability](ui/embedded-vscode-executor-availability.md) | approved |
| [embedded-vscode-windows-availability](ui/embedded-vscode-windows-availability.md) | archived; superseded by embedded-vscode-executor-availability |
| [ws-connectivity-warning](ui/ws-connectivity-warning.md) | approved |
| [context-compaction-count](context-compaction-count/spec.md) | approved |
| [context-window reset freshness](context-window-reset-freshness/spec.md) | shipped |
| [cancel-turn-progress](ui/cancel-turn-progress.md) | approved |
| [agent-todo-list-panel](ui/agent-todo-list-panel.md) | shipped |
| [prompt-history-panel](ui/prompt-history-panel.md) | draft |
| [prompt-turn-duration](ui/prompt-turn-duration.md) | draft |

## system-page/ — operational diagnostics & maintenance UI

System pages (Radarr/Sonarr-style) for status, disk usage, database maintenance, backups, logs, updates, OSS licenses, and about.

| Spec | Status |
|---|---|
| [system-page](system-page/spec.md) | draft |
| [storage-maintenance](system-page/storage-maintenance.md) | building |
| [storage-overview-parallel-scan](system-page/storage-overview-parallel-scan.md) | shipped |
| [feature-toggles](feature-toggles/spec.md) | draft |

---

## Standalone

| Spec | Status |
|---|---|
| [mock-agent-slow-duration](mock-agent-slow-duration/spec.md) | shipped |
| [session-subscription-recovery](session-subscription-recovery/spec.md) | draft |
| [npm-nightly-channel](npm-nightly-channel/spec.md) | shipped |
| [scoop-release-automation](scoop-release-automation/spec.md) | shipped |
| [release-pr-queue-bypass](release-pr-queue-bypass/spec.md) | shipped |
| [agent-resume-runtime-recovery](agent-resume-runtime-recovery/spec.md) | shipped |
| [agent-stall-recovery](agent-stall-recovery/spec.md) | approved |
| [mcp-session-observability](mcp-session-observability/spec.md) | approved |
| [subagent-context-persistence](subagent-context-persistence/spec.md) | draft |
| [external-question-answering](external-question-answering/spec.md) | draft |
| [auth](auth/spec.md) | building |
| [create-local-repository](create-local-repository/spec.md) | shipped |
| [improve-kandev](improve-kandev/spec.md) | building |
| [homebrew-core](homebrew-core/spec.md) | building |
| [native-kandev-cli](native-kandev-cli/spec.md) | draft |
| [desktop-tauri-app](desktop-tauri-app/spec.md) | shipped |
| [port-collision-safety](port-collision-safety/spec.md) | building |
| [lsp-file-intelligence](lsp-file-intelligence/spec.md) | building |
| [public-share-links](public-share-links/spec.md) | draft |
| [ssh-executor](ssh-executor/spec.md) | draft |
| [cli-mode-parity](cli-mode-parity/spec.md) | draft |
| [claude-fork-review-allowlist](claude-fork-review-allowlist/spec.md) | building |
| [mobile-quick-chat-topbar](mobile-quick-chat-topbar/spec.md) | building |
| [quick-chat-activity-indicators](quick-chat-idle-dot/spec.md) | draft |
| [native-code-review](native-code-review/spec.md) | building |
| [missing-task-route-recovery](missing-task-route-recovery/spec.md) | draft |
| [kanban-task-executor-cache-staleness](kanban-task-executor-cache-staleness/spec.md) | draft |
| [browser-inspect-annotations-save](browser-inspect-annotations-save/spec.md) | shipped |
| [automations-pr-merged-trigger](automations-pr-merged-trigger/spec.md) | draft |
| [automation-runs-delete-all-by-status](automation-runs-delete-all-by-status/spec.md) | draft |
| [no-silent-model-fallback](no-silent-model-fallback/spec.md) | approved |
| [portable-agent-configuration](portable-agent-configuration/spec.md) | draft |
| [e2e-duration-aware-sharding](e2e-duration-aware-sharding/spec.md) | shipped |
| [board-step-visibility-filter](board-step-visibility-filter/spec.md) | draft |
| [shutdown-turn-failure-suppression](shutdown-turn-failure-suppression/spec.md) | draft |
| [executor-profile-env-precedence](executor-profile-env-precedence/spec.md) | building |
| [automations-yaml-export](automations-yaml-export/spec.md) | building |
| [pr-walkthrough](pr-walkthrough/spec.md) | building |

---

## Legacy conventions

- **Spec layout.** These two layouts remain valid only during migration.
  Umbrella specs use flat files. Standalone specs use a `spec.md` file.
- **Plans are not specifications.** Implementation plans remain under
  `docs/plans/<initiative>/`. Their task files are work orders.
- **Bug fixes are not specs.** Bugs produce a regression test plus an ADR if they encoded a new convention. See `/fix` skill.
- **Architecture decisions are not specs.** ADRs live under `docs/decisions/`. See `/record decision`.

## Cross-references

- ADRs: [`../decisions/INDEX.md`](../decisions/INDEX.md)
- Specification system: [`README.md`](README.md)
- Spec workflow: [`.agents/skills/spec/SKILL.md`](../../.agents/skills/spec/SKILL.md)
- Bug-fix workflow: [`.agents/skills/fix/SKILL.md`](../../.agents/skills/fix/SKILL.md)
