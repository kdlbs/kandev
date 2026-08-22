# Feature Specs

Specs for kandev product features, grouped by umbrella. Each spec describes a user-invocable capability and is the source of truth for "is this feature done?"

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
| [provider-error-recovery](platform/provider-error-recovery.md) | draft |
| [duplicated-tab-stale-data](fix-duplicated-tab-stale-data/spec.md) | building |
| [health-endpoint-version](health-endpoint-version/spec.md) | building |
| [go-dev-launcher](go-dev-launcher/spec.md) | draft |

## tasks/ — task & workflow model

Kandev's task model: documents, execution stages, labels, blocker escalation, subtask checklists, subtree controls, and the unification with the workflow engine.

| Spec | Status |
|---|---|
| [documents](tasks/documents.md) | shipped |
| [execution-stages](tasks/execution-stages.md) | shipped |
| [interrupted-task-indicator](tasks/interrupted-task-indicator.md) | complete |
| [labels](tasks/labels.md) | shipped |
| [title-length-limit](tasks/title-length-limit.md) | complete |
| [rich-task-title-previews](tasks/rich-task-title-previews.md) | building |
| [active clarification lifecycle](clarification-active-lifecycle/spec.md) | approved |
| [model-unification](tasks/model-unification.md) | draft |
| [run-scheduling](tasks/run-scheduling.md) | building |
| [without-repositories](tasks/without-repositories.md) | draft |
| [attach-workspace-sources](tasks/attach-workspace-sources.md) | building |
| [remote-contribution-tasks](tasks/remote-contribution-tasks.md) | approved |
| [subtask-checklist](tasks/subtask-checklist.md) | shipped |
| [subtask-detachment](tasks/subtask-detachment.md) | shipped |
| [subtask-reparenting-drag-drop](tasks/subtask-reparenting-drag-drop.md) | building |
| [subtask-completion-trigger](tasks/subtask-completion-trigger.md) | draft |
| [task-dependencies](task-dependencies/spec.md) | draft |
| [task-dependencies - create dialog selector](task-dependencies/create-dialog-dependency-selector.md) | implemented |
| [task-dependencies - create dialog advanced settings](task-dependencies/create-dialog-advanced-settings.md) | shipped |
| [subtree-controls](tasks/subtree-controls.md) | shipped |
| [blocked-task-escalation](tasks/blocked-task-escalation.md) | draft |
| [runtime-cleanup](tasks/runtime-cleanup.md) | draft |
| [session-delete-resource-cleanup](session-delete-resource-cleanup/spec.md) | draft |
| [archive-confirmation](tasks/archive-confirmation.md) | shipped |
| [link-existing-task-github-issue](tasks/link-existing-task-github-issue.md) | building |
| [wip-limit-pull-system](tasks/wip-limit-pull-system.md) | draft |
| [multi-branch](tasks/multi-branch/spec.md) | shipped |
| [quick-chat-sessions](tasks/quick-chat-expiration.md) | shipped |
| [quick-chat-repository-context](tasks/quick-chat-repository-context.md) | shipped |
| [parent-child-message-interrupt](tasks/parent-child-message-interrupt.md) | shipped |
| [parent-child-task-stop](tasks/parent-child-task-stop.md) | shipped |
| [mcp-task-agent-profile-default](tasks/mcp-task-agent-profile-default/spec.md) | shipped |
| [spawn-session-effective-profile](spawn-session-effective-profile/spec.md) | shipped |
| [runtime-state-publication-order](tasks/runtime-state-publication-order.md) | shipped |
| [agent-generated-titles](tasks/agent-generated-titles.md) | approved |
| [task-create-executor-default](tasks/task-create-executor-default.md) | approved |
| [task-create-workflow-memory](tasks/task-create-workflow-memory.md) | approved |
| [task-create-escape-dismissal](tasks/task-create-escape-dismissal.md) | complete |
| [repository-sets](repository-sets/spec.md) | building |
| [external-id-idempotency](tasks/external-id-idempotency/spec.md) | draft |
| [prompt-attachments](tasks/prompt-attachments.md) | draft |
| [sidebar-task-edit](tasks/sidebar-task-edit.md) | approved |
| [autopilot-mode](tasks/autopilot-mode.md) | draft |
| [explicit-completion-signal](workflow/explicit-completion-signal/spec.md) | shipped |
| [cancelled-turn-completion](workflow/cancelled-turn-completion/spec.md) | building |
| [task-step-transition-ledger](workflow/task-step-transition-ledger/spec.md) | draft |
| [conditional-session-settings](workflow-session-settings/spec.md) | approved |
| [prevent-agent-autostart-on-open](prevent-agent-autostart-on-open/spec.md) | draft |
| [workflow-duplication](workflow-duplication/spec.md) | draft |

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

## workspaces/ — workspace lifecycle

| Spec | Status |
|---|---|
| [creation](workspaces/creation.md) | building |
| [deletion](workspaces/deletion.md) | shipped |
| [local-repositories](workspaces/local-repositories.md) | shipped |
| [worktree-branch-templates](workspaces/worktree-branch-templates.md) | building |
| [repository-secrets](workspaces/repository-secrets.md) | shipped |
| [secret-scope-transfer](workspaces/secret-scope-transfer.md) | shipped |
| [visibility-and-membership](workspaces/membership.md) | draft |

## costs/ — cost tracking & budgets

Subscription quota tracking and per-agent cheap-model profile routing.

| Spec | Status |
|---|---|
| [subscription-usage](costs/subscription-usage.md) | draft |
| [cheap-model-profiles](costs/cheap-model-profiles.md) | shipped |

## ui/ — cross-cutting UI features

| Spec | Status |
|---|---|
| [workspace-active-first-order](ui/workspace-active-first-order.md) | shipped |
| [ci-pr-automation](ui/ci-pr-automation.md) | building |
| [github-pr-review-actions](ui/github-pr-review-actions.md) | shipped |
| [github-saved-query-defaults](ui/github-saved-query-defaults.md) | shipped |
| [pr-detail-header-width](pr-detail-header-width/spec.md) | shipped |
| [pr-task-status-summary](ui/pr-task-status-summary.md) | shipped |
| [comment-markdown](ui/comment-markdown.md) | shipped |
| [resizable-markdown-tables](ui/resizable-markdown-tables.md) | building |
| [transcript-auto-scroll](ui/transcript-auto-scroll.md) | building |
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
| [auth](auth/spec.md) | building |
| [auth — roles and scopes](auth/roles-and-scopes.md) | draft |
| [multi-tenancy](multi-tenancy/spec.md) | draft |
| [create-local-repository](create-local-repository/spec.md) | shipped |
| [workflow-cycle-guardrails](workflow-cycle-guardrails/spec.md) | building |
| [workflow-quorum-decision-recording](workflow-quorum-decision-recording/spec.md) | draft |
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
| [workflow-settings-autosave](workflow-settings-autosave/spec.md) | archived; superseded by settings-manual-save |
| [mobile-quick-chat-topbar](mobile-quick-chat-topbar/spec.md) | building |
| [quick-chat-idle-dot](quick-chat-idle-dot/spec.md) | draft |
| [native-code-review](native-code-review/spec.md) | building |
| [missing-task-route-recovery](missing-task-route-recovery/spec.md) | draft |
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
| [task-launch-failure-recovery](task-launch-failure-recovery/spec.md) | draft |

---

## Conventions

- **Spec layout.** Umbrella specs live as flat `.md` files under the umbrella directory (`docs/specs/office/agents.md`). Standalone specs use a folder (`docs/specs/improve-kandev/spec.md`).
- **Plans are not specs.** Implementation plans are committed under `docs/plans/<feature>/` with individual sibling task files named `task-<NN>-<short-slug>.md`. Specs are the durable requirements; plans and task files are implementation records for the current buildout.
- **Bug fixes are not specs.** Bugs produce a regression test plus an ADR if they encoded a new convention. See `/fix` skill.
- **Architecture decisions are not specs.** ADRs live under `docs/decisions/`. See `/record decision`.

## Cross-references

- ADRs: [`../decisions/INDEX.md`](../decisions/INDEX.md)
- Spec workflow: [`.agents/skills/spec/SKILL.md`](../../.agents/skills/spec/SKILL.md)
- Bug-fix workflow: [`.agents/skills/fix/SKILL.md`](../../.agents/skills/fix/SKILL.md)
