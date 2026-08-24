# Office — autonomous agent management

`internal/office/` implements Office: workspaces of long-running agents that pick up tasks, coordinate via handoffs, and report through a dashboard. This file routes you to the specs and records local traps; it does not restate either.

## Spec authority

`docs/specs/office/` (15 files) is the authority for what Office is and why — it outranks any card, register, or comment. **Where code and spec disagree, that is a defect in one of them; do not silently follow the code.** A sixteenth office-tagged spec, [per-agent + per-role tier selection](../../../../docs/specs/office-agent-tier-routing/spec.md), lives in a sibling directory, not under `docs/specs/office/`.

| Spec | Status | Covers |
|---|---|---|
| `overview.md` | draft | Why Office exists: autonomy layer on top of kandev's task system |
| `agents.md` | draft | Persistent Office agent identity vs. execution profiles |
| `tasks.md` | draft | Office task model: assignee, reviewers/approvers, blockers, subtasks, per-agent working memory |
| `scheduler.md` | draft | Autonomous wakeup pipeline: assignments/comments/approvals, routines, idle skip, retry/backoff |
| `runtime.md` | draft | Error-handling contract for the agent runtime; **2026-08-17 amendment**: provider classification/recovery is superseded by `../platform/provider-error-recovery.md` and `../agents/dynamic-agent-routing.md` — read the amendment banner before the body |
| `routing.md` | **archived** | Provider routing; superseded by `../agents/dynamic-agent-routing.md` — do not treat as current |
| `costs.md` | in-progress | Cost tracking and budget management |
| `dashboard.md` | draft | Workspace-health landing page: agents, run trends, recent activity, spend |
| `live-updates.md` | draft | Real-time refresh so concurrent agent fan-out is visible without a manual reload |
| `inbox.md` | draft | Centralized inbox, approvals, and activity log for actions needing human judgment |
| `assistant.md` | draft | Personal-assistant agent, external channels (Telegram/Slack/email), and agent memory |
| `automation-runs.md` | draft | Reading an automation's own output without opening its edit form |
| `automations-settings.md` | draft | Scheduling an automation (cron / PR event / webhook) from Settings, not the board |
| `testing.md` | shipped | E2E mock harness for task sessions/messages — Playwright can't launch real executors in CI |
| `unread-divider.md` | shipped | Slack-style unread divider for background-running task sessions |

`office-agent-tier-routing/spec.md`'s own front matter still calls `routing.md` "authoritative" for tiers, provider order, execution profiles, provider health, and wake-reason policy — that predates `routing.md`'s archival in `docs/specs/INDEX.md` and is now stale; trust the INDEX status over the sibling spec's own text.

## Traps

- **A run is not a task start.** The `runs` table carries provider routing (`resolved_provider_id`, `logical_provider_order`, `requested_tier`, plus the `office_run_route_attempts` ledger), the per-reason `assembled_prompt`, the `AppendRunEvent` timeline, and retry/backoff/coalescing (`retry_count`, `earliest_retry_at`, `coalesced_count`, `idempotency_key` with unique index `idx_run_idempotency`). `Service.QueueRun` (`service/run.go`) and `SchedulerService.QueueRun` (`scheduler/run.go`) are the entry points that populate all of it; `TaskStarter` (the `StartTask` seam office calls, `service/service.go`) runs only *after*. Calling `StartTask` directly, or any ordinary kanban start path, skips all of the above. See `scheduler.md`.
- **`heartbeat` as a wake source is retired** (`scheduler.md`: routines are "the only mechanism for periodic wakes"; `wakeup/payloads.go` states `"heartbeat"` is intentionally absent from the `Source` enum) — but three literal `"heartbeat"` constants still exist in code (`scheduler/run.go`'s and `service/run.go`'s duplicate `RunReasonHeartbeat`, `routing/types.go`'s `WakeReasonHeartbeat`, still listed in `AllWakeReasons` and used to seed a `TierPerReason` policy in `onboarding/service.go`). `checkIdleSkip` (`service/scheduler_integration.go`) only applies when `run.Reason == RunReasonHeartbeat`, while the lightweight routine path sets `Reason: "routine_dispatch"` (`routines/service.go`, copied onto the run by `wakeup/dispatcher.go`). Check the reason string before assuming an idle-skip gate fires for a given run.
- **Continuation summary scope must come from `summaryScopeForRun`** (`service/event_subscribers.go`), never a literal string. Its sole caller, `refreshContinuationSummary`, upserts under the same value it reads, so reader and writer can't drift. Two scope forms exist: `routine:<id>` and `agent:<id>`; the legacy `"heartbeat"` scope is retired.
- **Participant slate: one builder, not office's own.** The AC-50 slate construction lives in `internal/workflow/engine` — `(*Engine).requiredSeats` / `requiredSeatsForWorkflow` (`workflow/engine/quorum.go`) — and `ResolveParticipantRole` (same file) reuses it so role resolution and slate membership can never disagree about who participates. Office reaches it through `engine_dispatcher.Dispatcher.ResolveParticipantRole`, which forwards verbatim. Do not add an office-local "who are this task's reviewers" query; that is a fourth answer to a question that already has exactly one.
- **Lightweight vs. heavy routines fail differently.** A routine with an empty `task_template` (`routines/service.go`; `CreateDefaultCoordinatorRoutine` sets `TaskTemplate: ""`, cron `*/5 * * * *`) is lightweight. Its **idle skip** — firing with no actionable task, launching nothing — is specified behavior (`scheduler.md`, telemetry `wakeup_idle_skipped`), not a bug; don't "fix" it. `checkIdleSkip` fails **open** on a DB error.
- **Advancement has two signals.** An explicit `step_complete_kandev` MCP call (registered `mcp/server/server.go`, ADR 0015) and the turn-end path (`handleAgentReady`, `orchestrator/event_handlers_agent.go`), reconciled for late signals in `orchestrator/event_handlers_step_completion.go`. The per-step toggle `auto_advance_requires_signal` (`mcp/server/config_handlers.go`) couples them — know which one a step relies on before changing either.

## Package map

- `scheduler/` — wakeup queueing, run dispatch, idle skip, retry/backoff
- `service/` — core Office service: run lifecycle, event subscribers, continuation summaries
- `routines/` — routine (cron) definitions and dispatch, including the default coordinator routine
- `wakeup/` — wake payload/source/reason types and the dispatcher that turns them into runs
- `repository/sqlite/` — Office's SQLite tables (`runs`, route-attempt ledger, etc.)
- `routing/` — provider routing types; **spec archived**, package still live — exactly the kind of code/spec mismatch the precedence rule above exists for; readers wanting current routing behavior should start from `../agents/dynamic-agent-routing.md`
- `engine_dispatcher/` — office's bridge into `internal/workflow/engine` (participant roles, transitions)
- `engine_adapters/` — concrete adapters office supplies to the workflow engine (CEO, child-task creation, workflow switching); consumed from `internal/backendapp`
- `runtime/` — Office agent runtime wiring
- `summary/` — continuation summary storage
- `configloader/` — loads per-role agent instructions; `configloader/instructions/{ceo,devops,qa,reviewer,security,worker}/AGENTS.md` are the role-specific system prompts fed to each Office agent type, not developer docs. `ceo/` additionally has `HEARTBEAT.md`.
- `agents/`, `approvals/`, `channels/`, `config/`, `costs/`, `dashboard/`, `infra/`, `labels/`, `models/`, `onboarding/`, `projects/`, `shared/`, `skills/`, `testharness/`, `tree_controls/`, `truncate/`, `workspaces/` — one subpackage per dashboard/domain concept; names match the spec files above.
