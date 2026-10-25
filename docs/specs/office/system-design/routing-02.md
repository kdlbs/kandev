---
status: draft
system: office
requirements:
  - REQ-OFFICE-ROUTING-001
created: 2026-05-10
owners:
  - cfl
---
# Office Provider Routing System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## State machine

### Provider route lifecycle (per workspace, per provider)

- A short transient creates a non-degraded route-scoped `short_retry` cooldown.
  One run owns the retry; other matching runs wait and do not switch providers or
  consume the retry budget.
- `eligible` -> `degraded` on a long-horizon or short-budget-exhausted provider outage, quota, rate-limit, or model-capacity failure.
- `eligible` -> `user_action_required` on auth, inactive-subscription, missing-configuration, or invalid-model evidence.
- `degraded` -> `eligible` on any of: scheduled health check success, launch probe success, reconnect, user-triggered retry from the UI.
- `degraded` -> `degraded` (with increased backoff) on a probe / retry failure; the task using the route immediately moves to the next candidate.
- Auto-retryable degraded routes carry a `retry_at` timestamp; user-actionable degraded routes remain blocked indefinitely until the user acts.

### Task resolution outcome

- Short transient -> run schedules the same execution profile and remains out of
  fallback resolution until success or exhaustion.
- All-providers-exhausted, at least one auto-retryable: task is parked; scheduler wakes it at the earliest `retry_at`.
- All-providers-exhausted, all user-actionable: task is blocked until the user reconnects / configures / retries.
- Mixed: task is blocked, UI surfaces both the earliest automatic retry and the user actions needed.

## Permissions

Routing is a workspace-admin surface; agents themselves do not call any routing endpoints.

- Reading workspace routing (`GET /workspaces/:wsId/routing`, `/health`, `/preview`) is open to any caller authenticated to the workspace.
- Writing workspace routing (`PUT /workspaces/:wsId/routing`, `POST /retry`) is workspace-admin only. The dashboard surface assumes admin auth at the layer above; agent JWTs cannot mutate workspace settings.
- Per-agent routing overrides ride on `PATCH /agents/:id`, which the `agents` handler restricts: only the target agent itself or a CEO / admin role caller may mutate another agent's settings (see `isAdminRole` check in `internal/office/agents/handler.go`).
- The runtime action surface (`/runtime/*` in `internal/office/runtime/handler.go`) does NOT expose any routing-mutation capability; there is no `CapabilityModifyRouting` in v1. Agents that need to change tier or provider order escalate through approvals or human admins.

## Failure modes

- **Provider returns a known long quota or rate-limit signal**: route resolution uses the normalized error code rather than provider-specific prose. Provider goes `degraded`; task falls through to the next configured provider.
- **Provider returns auth, inactive-subscription, or configuration evidence**: route becomes `user_action_required`; task may fall through to another configured provider but the failed route receives no timer.
- **Provider fails before session start with an unrecognized provider-owned error**: scheduler records a low-confidence `unknown_provider_error`, preserves evidence, and tries the next configured provider.
- **Task has already started editing or running tools, provider reports an ambiguous failure**: scheduler does not fall back unless the error clearly matches a provider-unavailable class. Post-start fallback is conservative by design.
- **Provider returns a generic provider-unavailable error without a reset time**: same run enters the short same-route phase. Exhaustion marks the route degraded with provider-health backoff and allows configured fallback.
- **Provider returns a short network, overload, availability, or model-capacity error**: same run retries the current execution profile after 5, 10, and 20 seconds. It falls through only after exhaustion.
- **Degraded provider reaches retry time, probe fails again**: task immediately tries the next candidate; provider route stays degraded with an increased backoff.
- **Degraded provider's scheduled health check, launch probe, reconnect, or user-triggered retry succeeds**: route returns to `eligible`; future launches can use it again.
- **All configured providers quota-limited or temporarily unavailable**: task waits for capacity and automatically retries at the earliest route retry time. Logical Office agent assignment is preserved; task is not reassigned automatically.
- **All configured providers require an inactive subscription or other user action**: task is blocked with remediation hints and does not schedule those routes automatically. An explicitly enabled fallback chain is exhausted before reaching this state.
- **All configured providers require user action**: task is blocked until the user reconnects, configures a provider, fixes model mappings, or manually retries.
- **Provider health state changes (workspace level)**: dashboard and inbox show an actionable issue listing affected agents and routes.

## Persistence guarantees

- **Workspace routing config (`office_workspace_routing`)**: durable. Survives restart. When the row is missing, `GetWorkspaceRouting` synthesises the spec defaults in-memory and the caller may upsert later.
- **Provider health (`office_provider_health`)**: durable. `state`, `retry_at`, `backoff_step`, `last_failure`, `last_success`, and the sanitized `raw_excerpt` survive restart, so a cooldown or degraded provider retains its scheduler meaning across crashes. Healthy rows are not persisted (they are pruned to the healthy state, and `ListProviderHealth` filters them out at the SQL layer).
- **Run routing decision**: `runs.logical_provider_order`, `requested_tier`, `resolved_execution_profile_id`, `resolved_provider_id`, `resolved_model`, `current_route_attempt_seq`, `route_cycle_baseline_seq`, `routing_blocked_status`, `earliest_retry_at`, and `scheduled_retry_at` are durable. The `retry_scheduled` route attempt identifies a pending short phase; a short retry or parked run still re-dispatches after a restart because `scheduled_retry_at` re-enters the scheduler's eligibility filter.
- **Route attempts (`office_run_route_attempts`)**: durable. Every attempt records `execution_profile_id` before the next candidate is tried, so post-start fallback reasoning and the exact CLI profile used survive restart.
- **Session/executor binding**: `task_sessions.execution_profile_id` and `executors_running.execution_profile_id` bind process state and provider-native resume tokens to the concrete profile that created them. A profile change clears or ignores incompatible resume state.
- **Agent overrides**: durable via `agent_profiles.settings` JSON. Round-trips other settings keys untouched.
- **Backoff timing**: short same-route delays use the fixed 5/10/20-second ladder; long-horizon provider health uses the existing durable backoff step and retry deadline.
- **Prober registry (`routingerr.RegisterProber`)**: process-local, in-memory only. Probers must re-register on every boot via package-init or DI wiring. There is no on-disk record of which providers have probers.
- **Provider order / tier-mapping changes that materially alter routing decisions trigger `ClearAllParkedRoutingForWorkspace`**: parked runs are re-queued and resolution runs fresh against the new config. False-positive clears (changes that could not have affected the block reason) are harmless because runs simply re-park with the latest verdict.
- **Workspace flips enabled `true -> false`**: `ClearAllParkedRoutingForWorkspace` re-queues every parked run. The next dispatch still resolves the effective tier but tries only the first provider's execution profile.

See also: [`office/runtime.md`](../requirements/runtime.md) for how the runtime classifies and surfaces individual errors, and the office scheduler (`internal/office/scheduler/`) for the dispatcher that consumes `Resolver.Resolve`.

## Scenarios

- **GIVEN** automatic provider routing is disabled, **WHEN** the coordinator agent launches, **THEN** it resolves the effective tier and launches only the first provider's referenced execution profile without trying a fallback.
- **GIVEN** a custom CTO Office agent has five skills and custom instructions and its Frontier order is `codex -> claude`, with Codex GPT-5.6 and Claude Opus execution profiles, **WHEN** Codex is healthy, **THEN** the CTO launches through the full Codex profile while retaining its Office identity configuration.
- **GIVEN** that Codex execution hits a classified five-hour usage limit after starting work, **WHEN** routing falls back to Claude, **THEN** Kandev starts a fresh Claude-native session in the same task and worktree using the full Claude profile, reapplies the CTO instructions and skills, and tells Claude to inspect durable task and git state before continuing.
- **GIVEN** that cross-provider fallback, **WHEN** the Claude process starts, **THEN** it does not receive the Codex ACP resume token or Codex-specific environment, flags, permissions, config options, passthrough setting, or MCP configuration.
- **GIVEN** workspace routing is enabled with `claude -> codex -> opencode` and tier `balanced`, **WHEN** an agent without overrides launches, **THEN** it first tries the Claude balanced model and may fall back through the remaining providers on provider-limit errors.
- **GIVEN** the CEO agent overrides provider order to `claude` and tier `frontier`, **WHEN** Claude is rate-limited, **THEN** the CEO run does not try Codex or OpenCode; a short limit retries Claude first, while a long limit or exhausted short budget parks for provider capacity.
- **GIVEN** a worker agent inherits workspace routing, **WHEN** the workspace default tier changes from `frontier` to `balanced`, **THEN** future worker runs use the balanced tier without editing the worker.
- **GIVEN** routing is enabled after several agents already exist, **WHEN** those agents still inherit workspace routing, **THEN** their future runs use the workspace default tier and provider order without requiring per-agent edits.
- **GIVEN** a task run falls back from Claude to Codex because Claude is quota-limited, **WHEN** the user opens the task, run history, agent detail, or dashboard, **THEN** the UI shows the intended primary route, the actual provider/model, and the quota-limit reason.
- **GIVEN** Claude credentials expire, **WHEN** an agent with fallback providers launches, **THEN** the scheduler records the auth failure, marks Claude user-action-required for the workspace, and tries the next configured provider.
- **GIVEN** a provider adapter returns a known quota, auth, or rate-limit signal, **WHEN** route resolution handles the failure, **THEN** the scheduler uses the normalized error code rather than provider-specific prose.
- **GIVEN** a provider fails before session start with an unrecognized provider-owned error, **WHEN** no classifier rule matches, **THEN** the scheduler records a low-confidence `unknown_provider_error`, preserves evidence, and tries the next configured provider.
- **GIVEN** a task has already started editing or running tools, **WHEN** the provider reports an ambiguous failure, **THEN** the scheduler does not fall back unless the error clearly matches a provider-unavailable class.
- **GIVEN** a provider returns a generic provider-unavailable error without a reset time, **WHEN** the route fails, **THEN** the scheduler first retries the same execution profile on the short schedule and only degrades/falls through after exhaustion.
- **GIVEN** a provider returns a network reset, overload, or model-capacity error,
  **WHEN** the Office run has short retries remaining, **THEN** the scheduler
  retries the same execution profile after a few seconds and does not select the
  next provider.
- **GIVEN** the same transient survives three short retries, **WHEN** the last
  attempt fails, **THEN** the scheduler degrades the provider route and continues
  through the configured fallback chain.
- **GIVEN** multiple Office runs encounter the same scoped transient, **WHEN** a
  short retry cooldown is active, **THEN** the other runs wait on the shared
  cooldown without switching providers or consuming additional attempts.
- **GIVEN** a degraded provider reaches its retry time, **WHEN** a health probe or task-launch probe fails again, **THEN** the task immediately tries the next candidate and the provider route receives a longer backoff.
- **GIVEN** a provider is marked degraded, **WHEN** a scheduled health check, launch probe, reconnect, or user-triggered retry succeeds, **THEN** future launches can use that provider again.
- **GIVEN** all configured providers are quota-limited or temporarily unavailable, **WHEN** a task exhausts the route list, **THEN** the task waits for provider capacity and automatically retries at the earliest route retry time.
- **GIVEN** all configured providers require user action, **WHEN** a task exhausts the route list, **THEN** the task is blocked until the user reconnects, configures a provider, fixes model mappings, or manually retries.
- **GIVEN** exhausted provider routes include both auto-retryable and user-actionable failures, **WHEN** the task is blocked, **THEN** the UI shows the earliest automatic retry and the user actions needed for the blocked routes.
- **GIVEN** a provider is unavailable for a workspace, **WHEN** the provider health state changes, **THEN** the dashboard and inbox show an actionable issue listing affected agents and routes.
- **GIVEN** onboarding creates a new workspace, **WHEN** setup completes, **THEN** automatic fallback is disabled but the selected Frontier / Balanced / Economy execution profile IDs are stored in the workspace routing seed.
- **GIVEN** those execution profiles reference provider rows by database UUID, **WHEN** onboarding writes the routing seed, **THEN** provider order is keyed by each provider's logical registry name and the execution profile IDs remain the authoritative tier mappings.
- **GIVEN** an agent profile is referenced by a workspace tier, **WHEN** the user tries to delete that profile, **THEN** deletion returns an in-use error until the tier mapping is changed.
- **GIVEN** routing is enabled and the workspace's `tier_per_reason.heartbeat = economy`, **WHEN** a heartbeat run launches, **THEN** the resolver picks the Economy tier model regardless of the agent's default tier.
- **GIVEN** the security agent overrides wake-reason tiers with `heartbeat = frontier`, **WHEN** its heartbeat fires, **THEN** it uses Frontier even though the workspace policy says Economy.
- **GIVEN** a run reason has no policy (e.g. `task_assigned`), **WHEN** that run launches, **THEN** it uses the agent's effective tier as before, ignoring the wake-reason policy entirely.

## Out of scope

- Creating a new Office identity table or reversing ADR 0005's physical table unification.
- Routing non-Office kanban sessions.
- Transferring provider-native conversation history between different CLIs.
- Shipping recommended provider/model tier presets.
- Cost optimization beyond user-selected tiers and provider order.
- Per-wake-reason policy for reasons outside `{heartbeat, routine_trigger, budget_alert}` in v1. Future work could extend the set.
