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
| [plugins](office/plugins.md) | draft |
| [testing](office/testing.md) | shipped |

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
| [subtree-controls](tasks/subtree-controls.md) | shipped |
| [blocked-task-escalation](tasks/blocked-task-escalation.md) | draft |

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
| [comment-markdown](ui/comment-markdown.md) | shipped |

---

## Standalone

| Spec | Status |
|---|---|
| [improve-kandev](improve-kandev/spec.md) | draft |

---

## Conventions

- **Spec layout.** Umbrella specs live as flat `.md` files under the umbrella directory (`docs/specs/office/agents.md`). Standalone specs use a folder (`docs/specs/improve-kandev/spec.md`).
- **Plans are not specs.** Implementation plans (`plan.md`) are working files, gitignored. Specs are the durable requirements.
- **Bug fixes are not specs.** Bugs produce a regression test plus an ADR if they encoded a new convention. See `/fix` skill.
- **Architecture decisions are not specs.** ADRs live under `docs/decisions/`. See `/record decision`.

## Cross-references

- ADRs: [`../decisions/INDEX.md`](../decisions/INDEX.md)
- Spec workflow: [`.agents/skills/spec/SKILL.md`](../../.agents/skills/spec/SKILL.md)
- Bug-fix workflow: [`.agents/skills/fix/SKILL.md`](../../.agents/skills/fix/SKILL.md)
