# Feature Specs

Specs for kandev product features, grouped by umbrella. Each spec describes a user-invocable capability and is the source of truth for "is this feature done?"

The bar: an agent given only a spec (no source code) should be able to either reimplement the feature or test the existing system for conformance. See `.agents/skills/spec/SKILL.md` for the workflow and template.

**Status:** `draft` (being written) · `building` (in active development) · `shipped` (implemented, spec matches code) · `archived` (deprecated).

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
| [routing](office/routing.md) | draft |
| [costs](office/costs.md) | in-progress |
| [dashboard](office/dashboard.md) | draft |
| [live-updates](office/live-updates.md) | draft |
| [inbox](office/inbox.md) | draft |
| [assistant](office/assistant.md) | draft |
| [automations-settings](office/automations-settings.md) | draft |
| [testing](office/testing.md) | shipped |

## platform/ — cross-cutting capabilities

Product-wide capabilities that are not tied to a single feature area.

| Spec | Status |
|---|---|
| [plugins](plugins/spec.md) | draft |

## tasks/ — task & workflow model

Kandev's task model: documents, execution stages, labels, blocker escalation, subtask checklists, subtree controls, and the unification with the workflow engine.

| Spec | Status |
|---|---|
| [documents](tasks/documents.md) | shipped |
| [execution-stages](tasks/execution-stages.md) | shipped |
| [labels](tasks/labels.md) | shipped |
| [model-unification](tasks/model-unification.md) | draft |
| [without-repositories](tasks/without-repositories.md) | draft |
| [subtask-checklist](tasks/subtask-checklist.md) | shipped |
| [subtask-completion-trigger](tasks/subtask-completion-trigger.md) | draft |
| [subtree-controls](tasks/subtree-controls.md) | shipped |
| [blocked-task-escalation](tasks/blocked-task-escalation.md) | draft |
| [runtime-cleanup](tasks/runtime-cleanup.md) | draft |
| [archive-confirmation](tasks/archive-confirmation.md) | shipped |
| [link-existing-task-github-issue](tasks/link-existing-task-github-issue.md) | building |
| [wip-limit-pull-system](tasks/wip-limit-pull-system.md) | building |
| [multi-branch](tasks/multi-branch/spec.md) | shipped |
| [quick-chat-expiration](tasks/quick-chat-expiration.md) | draft |
| [quick-chat-repository-context](tasks/quick-chat-repository-context.md) | shipped |
| [parent-child-message-interrupt](tasks/parent-child-message-interrupt.md) | shipped |

## agents/ — agent governance

Roles, governance gates, and granular permissions that apply across human users and office agents.

| Spec | Status |
|---|---|
| [roles](agents/roles.md) | shipped |
| [governance](agents/governance.md) | shipped |
| [granular-permissions](agents/granular-permissions.md) | draft |

## integrations/ — external service integrations

Per-workspace credentials and triage triggers for external services.

| Spec | Status |
|---|---|
| [slack](integrations/slack.md) | shipped |
| [external-mcp](integrations/external-mcp.md) | draft |
| [gitlab-integration](gitlab-integration/spec.md) | shipped |
| [jira-status-filter](jira-status-filter/spec.md) | shipped |

## workspaces/ — workspace lifecycle

| Spec | Status |
|---|---|
| [deletion](workspaces/deletion.md) | shipped |

## costs/ — cost tracking & budgets

Subscription quota tracking and per-agent cheap-model profile routing.

| Spec | Status |
|---|---|
| [subscription-usage](costs/subscription-usage.md) | draft |
| [cheap-model-profiles](costs/cheap-model-profiles.md) | shipped |

## ui/ — cross-cutting UI features

| Spec | Status |
|---|---|
| [ci-pr-automation](ui/ci-pr-automation.md) | draft |
| [comment-markdown](ui/comment-markdown.md) | shipped |
| [empty-turn-notice](ui/empty-turn-notice.md) | shipped |
| [acp-shell-command-output](ui/acp-shell-command-output.md) | shipped |
| [review-file-status](ui/review-file-status.md) | building |
| [slash-command-composer](ui/slash-command-composer.md) | shipped |

## system-page/ — operational diagnostics & maintenance UI

System pages (Radarr/Sonarr-style) for status, disk usage, database maintenance, backups, logs, updates, OSS licenses, and about.

| Spec | Status |
|---|---|
| [system-page](system-page/spec.md) | draft |
| [feature-toggles](feature-toggles/spec.md) | draft |

---

## Standalone

| Spec | Status |
|---|---|
| [workflow-cycle-guardrails](workflow-cycle-guardrails/spec.md) | building |
| [improve-kandev](improve-kandev/spec.md) | draft |
| [homebrew-core](homebrew-core/spec.md) | draft |
| [native-kandev-cli](native-kandev-cli/spec.md) | draft |
| [desktop-tauri-app](desktop-tauri-app/spec.md) | shipped |
| [public-share-links](public-share-links/spec.md) | draft |
| [ssh-executor](ssh-executor/spec.md) | draft |
| [cli-mode-parity](cli-mode-parity/spec.md) | draft |

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
