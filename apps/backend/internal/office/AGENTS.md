# Office — autonomous agent management

`internal/office/` implements Office: workspaces of long-running agents that pick up tasks, coordinate via handoffs, and report through a dashboard. This file routes you to the specs and records local traps; it does not restate either.

## Spec authority

For Office requirements and design authority, read [specification authority](SPEC-AUTHORITY.md) before changing Office behavior.

## Traps

Before changing Office scheduling, routing, security, or task behavior, read the relevant [operational invariants](OPERATIONAL-INVARIANTS.md).

## Package map

- `scheduler/` — wakeup queueing, run dispatch, idle skip, retry/backoff
- `service/` — core Office service: run lifecycle, event subscribers, continuation summaries
- `configsync/` — per-workspace config sync: pulls agent/skill/project/routine definitions from a GitHub or GitLab repository on a schedule (`service.go`, `poller.go`, `provider.go`), reconciles them against Office's tables (`reconcile*.go`), and exposes the HTTP config/force-sync routes (`handlers.go`, `models.go`). Modeled on `internal/workflowsync` but shares no code with it. `Service` also satisfies the `ActiveSourceChecker` guard consumed by `office/config` and `office/dashboard` — see the trap above.
- `routines/` — routine (cron) definitions and dispatch, including the default coordinator routine
- `wakeup/` — wake payload/source/reason types and the dispatcher that turns them into runs
- `repository/sqlite/` — Office's SQLite tables (`runs`, route-attempt ledger, etc.)
- `routing/` — provider routing types; **spec deprecated**, package still live — tier precedence is in `docs/specs/office/requirements/office-agent-tier-routing.md`; the provider-routing migration target is `docs/specs/agents/system-design/dynamic-agent-routing-01.md`
- `engine_dispatcher/` — office's bridge into `internal/workflow/engine` (participant roles, transitions)
- `engine_adapters/` — concrete adapters office supplies to the workflow engine (CEO, child-task creation, workflow switching); consumed from `internal/backendapp`
- `runtime/` — Office agent runtime wiring
- `summary/` — continuation summary storage
- `configloader/` — loads per-role agent instructions; `configloader/instructions/{ceo,devops,qa,reviewer,security,worker}/AGENTS.md` are the role-specific system prompts fed to each Office agent type, not developer docs. `ceo/` additionally has `HEARTBEAT.md`.
- `agents/`, `approvals/`, `channels/`, `config/`, `costs/`, `dashboard/`, `infra/`, `labels/`, `models/`, `onboarding/`, `projects/`, `shared/`, `skills/`, `testharness/`, `tree_controls/`, `truncate/`, `workspaces/` — one subpackage per dashboard/domain concept; names match the spec files above.
