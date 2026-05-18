---
status: in-progress
created: 2026-04-25
owner: cfl
---

# Office: Cost Tracking & Budget Management

## Why

Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

Office adds per-session cost estimation, aggregated views by agent/project/model, and budget policies that can alert or hard-stop agents when estimated limits are exceeded.

**Important framing**: costs displayed in Office are **estimated costs** based on token usage and published model pricing. They may not reflect the user's actual bill - users on subscription plans (e.g. Claude Max, Copilot Enterprise) pay a flat fee regardless of token consumption. The cost explorer is a visibility and governance tool, not a billing system.

## What

- Every agent session generates cost events as it runs.
- Cost events are aggregated on read into multiple views (by agent, project, task, model, time window).
- Budget policies SHALL enforce spending limits per agent, project, or workspace with `notify_only`, `pause_agent`, or `block_new_tasks` actions.
- Cost is resolved per cost event via a two-layer lookup (provider-reported then `models.dev` cache); no static fallback table exists.
- For providers that emit token telemetry on the ACP wire (claude-acp, opencode-acp, gemini, codex-acp), cost events are populated from the `complete` event at end-of-turn.
- For providers without usable wire telemetry (codex per-turn split, amp), a disk-runner subsystem reads on-disk session files via pinned `@ccusage/*` packages and feeds normalized cost events into the same pipeline.
- The cost explorer UI surfaces aggregations and lets the user manage budget policies.

## Data model

### `office_cost_events`

| Field | Type | Notes |
|---|---|---|
| `id` | string | PK, unique identifier |
| `session_id` | string | FK -> `task_sessions.id` |
| `task_id` | string | FK -> `tasks.id` |
| `agent_instance_id` | string | null for non-office sessions |
| `project_id` | string | null if unassigned |
| `model` | string | e.g. `claude-sonnet-4-20250514` |
| `provider` | string | e.g. `anthropic`, `openai` |
| `tokens_in` | int64 | input token count |
| `tokens_cached_in` | int64 | cached input tokens (prompt caching) |
| `tokens_out` | int64 | output token count |
| `cost_subcents` | int64 | hundredths of a cent (renamed from `cost_cents`). UI divides by 10000 |
| `estimated` | bool | true when token counts were synthesised |
| `provider_event_id` | string | null for wire-side rows; non-null for disk-runner rows. Format `ccusage:<provider>:<session_id>:<model>` |
| `provider_credits` | int64 | amp `credits` field (null elsewhere) |
| `occurred_at` | timestamp | when the cost was incurred |

Disk-runner rows are upserted by `(session_id, provider_event_id)`. For codex, after a successful aggregate row lands, the wire-side `estimated=true` rows for the same session are deleted to avoid double-counting.

### `office_budget_policies`

| Field | Type | Notes |
|---|---|---|
| `id` | string | PK |
| `scope_type` | enum | `agent` \| `project` \| `workspace` |
| `scope_id` | string | the scoped entity ID |
| `limit_cents` | int64 | hundredths of a cent |
| `period` | enum | `monthly` (resets calendar month) \| `total` (lifetime) |
| `alert_threshold_pct` | int | default 80 |
| `action_on_exceed` | enum | `notify_only` \| `pause_agent` \| `block_new_tasks` |

Multiple policies can apply to the same scope (e.g. monthly + total).

### `TaskSession` additions

`cost_subcents`, `tokens_in`, `tokens_out` incrementally updated as cost events arrive, providing quick per-session totals without scanning `office_cost_events`.

### Per-agent budget

Each agent instance has `budget_monthly_cents` set at creation (or inherited from a workspace default). The CEO proposes budgets in hire requests; users adjust via agent instance settings.

### Pricing cache (`models.dev`)

The `models.dev` dataset is fetched once per day to a workspace-local disk cache at `<data-dir>/cache/models-dev.json`. The file lives on disk full-fat; only queried models load into the in-memory map. Refresh runs in a background goroutine; refresh failures fall back to the existing on-disk file. Pricing is recorded per-million-tokens for input, cached read, cached write, and output separately (Anthropic charges different rates for cached read vs. cached write).

### Per-CLI usage shapes (ACP wire)

- **claude-acp**: `result.usage` (camelCase, `cachedRead`/`cachedWriteTokens`), plus `usage_update.cost.amount` in USD which is preferred when present. claude-acp uses logical aliases (`sonnet`, `haiku`); provider-reported cost is the only accurate signal.
- **opencode-acp**: `result.usage` with `inputTokens`/`outputTokens`/`thoughtTokens` (no cached tokens). Optional `usage_update.cost.amount` (often `0` on BYOK).
- **gemini**: `result._meta.quota.token_count.{input_tokens, output_tokens}` (snake_case, no cached, no cost).
- **codex-acp**: cumulative `usage_update.used` only; adapter infers per-turn deltas and flags rows `estimated=true`. Input vs output cannot be split on the wire.
- **auggie**, **copilot-acp**: not tracked. `_meta.copilotUsage` is a billing multiplier; Copilot `/usage` would require scraping.

### Disk-runner provider coverage

- **codex** - disk runner is the preferred source. Cost rows promoted from `estimated=true` to `estimated=false` with full token split.
- **amp** - disk runner is the only source. Cost events emitted with `estimated=false`; `credits` captured in `provider_credits`.
- **claude, opencode, gemini** - disk runner NOT used; wire data is authoritative.
- **auggie, copilot** - no `@ccusage/*` package; out of scope.

## API surface

### Cost resolution (two-layer lookup)

1. **Provider-reported cost.** If the CLI emits a USD amount on `usage_update`, store `int64(amount * 10000)` hundredths-of-a-cent and skip pricing lookup. Only accurate path for claude-acp.
2. **`models.dev` lookup.** For CLIs reporting tokens but no cost (gemini, opencode BYOK, codex fallback), resolve pricing against the cached dataset.

When both miss (first-boot, no network, model unknown, proprietary id), the row records `cost_subcents=0` and `estimated=true`. UI shows "pricing unavailable". Users can override pricing per model in workspace settings.

### Disk-runner binary: `cmd/usage-runner`

Standalone Go binary built into the backend Docker image alongside `kandev` and `agentctl`. Invoked after `session/complete` events for relevant providers, and periodically (every 60s) to catch sessions completed during backend downtime.

Spawns `npx -y @ccusage/<provider>@latest session --json`, reads stdout, emits normalized `CostEvent` records.

Join key: `TaskSession.ACPSessionID == ccusage.sessionId`. Verified empirically:
- codex: `session_meta.id` in rollout JSONL equals ACP session id equals rollout filename suffix.
- amp: thread JSON top-level `id` equals agent's session id.

ccusage's `session --json` rolls per-turn events into one summary per `(session_id, model)`. One cost row per session-model pair, not per turn. `provider_event_id` is deterministic from the aggregate row's identity, so re-runs replace rows.

ccusage version policy: track `@latest`. Mitigations: schema-validate ccusage's JSON output at decode time; nightly CI smoke job runs the runner against committed fixture inputs (fake `HOME` with synthetic rollout/threads files) and asserts the JSON contract.

### Cost aggregation queries

Aggregated on read (not pre-computed):
- **By agent instance**: total spend per agent over a time window.
- **By project**: total across all tasks.
- **By task**: total across all sessions.
- **By model**: spend broken down by model.
- **By time**: daily/weekly/monthly trends.

### Cost explorer UI routes

- `/office/company/costs` - summary bar, by-agent/by-project/by-model/by-time views, budget policies CRUD.
- Drill-in: agent rows -> `/office/agents/[id]` (Runs tab shows per-session cost; Overview shows cumulative + gauge); project rows -> project detail.

### Dashboard integration

`/office` shows per-agent budget utilization gauges on agent status cards and a "Spend this month" stat.

## State machine

Budget enforcement transitions (per agent instance):

| From | Trigger | To | Effect |
|---|---|---|---|
| `idle`/`running` | spend crosses `alert_threshold_pct` | (unchanged) | inbox item + `budget_alert` wakeup for CEO |
| `idle`/`running` | spend exceeds `limit_cents` with `action_on_exceed=pause_agent` | `paused` (`pause_reason=budget_exceeded`) | pending wakeups marked `finished`; current turn completes; no further prompts |
| `paused` (budget) | user increases budget | `idle` | wakeup processing resumes |
| `paused` (budget) | monthly reset on 1st UTC | `idle` (if new month's spend within limits) | counters reset |

Monthly reset is idempotent: backend restart mid-month does not refire.

## Permissions

Cost data and budget policies are workspace-scoped. The same auth gate as the rest of the office HTTP surface applies (see [agents.md](./agents.md#permissions)): UI requests authenticated via the user session bypass as admin; agent JWT requests run through the office permission middleware.

- **Read cost data** (`GET /workspaces/:wsId/costs*`): any caller with workspace access. CEO, worker, specialist, assistant, and reviewer agents can all read their workspace's cost rollups. There is no per-agent or per-project read scoping today - any agent in the workspace sees workspace-wide totals.
- **Manage budget policies** (`POST /workspaces/:wsId/budgets`, `PATCH /budgets/:id`, `DELETE /budgets/:id`): UI / admin only. The CEO's `kandev-budget` skill is the agent-facing surface for budget proposals; CEO agents do not call the budget HTTP endpoints directly, they raise proposals that the user actions through the inbox.
- **Override pricing** (per-model overrides written into workspace settings): user only. Agents cannot mutate the pricing table.
- **Trigger pause** (the side effect of `CheckBudget` when `action_on_exceed=pause_agent`): performed by the `system` actor inside the cost subscriber, not by any caller. The agent pause it produces is identical to a user-initiated pause; only the user can resume by raising the budget or waiting for the monthly reset.

There is no per-field permission model. Conformance tests should assert that cost-read endpoints accept any authenticated workspace member, and that mutating budget endpoints either accept the admin / UI session or follow the same JWT-bypass rules used by the rest of the office API.

## Failure modes

- **models.dev miss**: row recorded with `cost_subcents=0` and `estimated=true`; UI shows "pricing unavailable".
- **models.dev fetch fails**: background refresh falls back to existing on-disk file; no crash.
- **Disk-runner `npx` unavailable**: log one warning per provider per process lifetime, mark run skipped, backend continues. Codex sessions retain wire-side `estimated=true` rows; amp sessions remain untracked.
- **ccusage JSON schema drift**: schema validator returns decode error; office subscriber treats run as no-op (no rows touched). Codex falls back to wire-side estimated path; amp absent. Nightly fixture-smoke CI alerts maintainers.
- **`@ccusage/<provider>@latest` yanked**: next runner invocation fails; coverage degrades the same as parse failure.
- **Coalescing**: if `session/complete` fires while a runner is already executing for that provider, the second invocation is dropped. The 60s sweep catches sessions in the gap.

## Persistence guarantees

**Survives restart:**

- `office_cost_events` rows (full history, never trimmed). Disk-runner rows keyed by `(session_id, provider_event_id)` survive re-ingestion without duplicating; the wire-side `estimated=true` rows for codex are deleted once the matching aggregate row lands.
- `office_budget_policies` rows.
- `TaskSession.cost_subcents` / `tokens_in` / `tokens_out` running totals, kept in sync with `office_cost_events` so per-session totals are correct without a re-scan.
- Per-agent `budget_monthly_cents` stored on the agent instance row.
- Per-model pricing overrides stored in workspace settings.
- The on-disk `models.dev` cache at `<data-dir>/cache/models-dev.json`. Recovery on next boot: the in-memory pricing map is empty until first query; queries fall back to the on-disk file when the background refresh has not yet completed.
- Activity log entries `budget.alert` and `budget.exceeded` (workspace-scoped, included in the standard office backup as part of normal SQLite persistence - see `persistence.Provide` for the snapshot policy).

**Does NOT survive restart:**

- In-memory pricing map inside the models.dev client. Rebuilt lazily on first lookup from the on-disk cache file.
- The disk-runner coalescing set (the in-process record of "already running for provider X"). After restart the 60s periodic sweep is the catch-up path - no in-flight session state is replayed.
- Wire-side `estimated=true` rows for codex sessions that completed during downtime: those are reconciled when the next disk-runner sweep promotes them to the aggregate row.
- The "monthly reset already ran for month M" guard is durable (idempotent by month boundary) so a restart on the 1st UTC does not refire the reset for agents that already returned to `idle`.
- Cached aggregation results in the frontend store (`office.costSummary`, `office.budgetPolicies`) - rehydrated from the API on next page load.

No TTL or retention is applied to `office_cost_events`; rows accumulate for the lifetime of the workspace. Cleanup happens only through workspace deletion (cascade via task workspace foreign key) and agent / project deletion garbage collection (see `office/repository/sqlite/runtime.go`).

## Scenarios

- **GIVEN** an ACP agent session processing a turn, **WHEN** the `context_window` event arrives with token counts, **THEN** a cost event is created with the estimated cost looked up from models.dev pricing. The session's cumulative `cost_subcents` is updated.

- **GIVEN** a model not found in the models.dev dataset and no user override configured, **WHEN** a cost event is recorded, **THEN** token counts are stored but `cost_subcents` is zero. The cost explorer shows "pricing unavailable" for that model.

- **GIVEN** an agent instance with a monthly budget of $10 and 80% alert threshold, **WHEN** spending reaches $8, **THEN** an alert appears in the user's inbox and a `budget_alert` wakeup is queued for the CEO.

- **GIVEN** an agent instance with a monthly budget of $10 and `action_on_exceed=pause_agent`, **WHEN** spending exceeds $10, **THEN** the agent is paused, pending wakeups are cancelled, and an activity log entry records the auto-pause.

- **GIVEN** a paused agent (budget exceeded), **WHEN** the user increases the budget via the agent settings UI, **THEN** the agent's status returns to `idle` and wakeup processing resumes.

- **GIVEN** it is the 1st of a new month, **WHEN** the scheduler runs its monthly reset, **THEN** all monthly budget spend counters reset to zero. Previously paused agents (budget-exceeded) return to `idle` if their new month's spend is within limits.

- **GIVEN** a user on the cost explorer page, **WHEN** they view "By agent", **THEN** they see each agent instance with its monthly spend, budget limit, utilization percentage, and a visual gauge.

- **GIVEN** a completed codex-acp session that used a single model, **WHEN** the disk runner executes, **THEN** one cost row exists for that session with input/cached_input/output/reasoning summed, `estimated=false`, `provider_event_id="ccusage:codex:<sessionId>:<model>"`. The wire-side `estimated=true` rows previously emitted for that session are deleted. Re-running replaces the row; row count remains 1.

- **GIVEN** a codex session that used two models (model switched mid-session), **WHEN** the disk runner executes, **THEN** two rows exist - one per `(session, model)` pair - each with its own totals and `provider_event_id`.

- **GIVEN** a completed amp-acp session, **WHEN** the disk runner executes, **THEN** one cost row per model exists for the session including the amp `credits` value alongside USD cost. The agent appears in the cost explorer where it was previously absent.

- **GIVEN** Node/`npx` is not available on the host, **WHEN** the disk runner attempts to spawn ccusage, **THEN** it logs one warning per provider per process lifetime, marks the run as skipped, and the backend continues normally.

- **GIVEN** a pinned `@ccusage/codex` version is bumped, **WHEN** CI runs, **THEN** the recorded-fixture test asserts the new version's `--json` output still matches the expected shape. If the shape changed, CI fails before merge.

- **GIVEN** a codex session that completed during a backend restart, **WHEN** the 60s periodic sweep runs after startup, **THEN** the runner discovers the rollout file via ccusage's normal scan, emits cost rows, and the session shows accurate cost in the explorer.

## Out of scope

- Actual billing integration or payment processing (costs are estimates, not invoices).
- Tracking real spend for subscription-based plans (flat-fee subscriptions are not modeled).
- Per-turn cost limits (budgets are per-period, not per-turn).
- Cost allocation across multiple users (single-user workspace model).
- Cost forecasting or spend predictions.
- Auggie support - no `@ccusage/*` package exists.
- Copilot support - billing is request-multiplier-based, not token-based.
- Retroactive ingestion of sessions from before this feature ships.
- Replacing the wire-side ingestion for claude-acp / opencode-acp / gemini (their wire data is authoritative).
- A user-facing UI surface for the disk-runner binary; visibility flows through the existing cost explorer.

## Open questions

- **Node/npx in the backend container**: the backend image currently does not include Node. Options: `apt-get install nodejs npm` (~50MB), or bundle ccusage as a single-file `bun build --compile` per provider, eliminating runtime Node dependency.
- **Future providers**: when a new agent CLI lands (hypothetical `@ccusage/auggie`, new `@ccusage/copilot`), the runner should accept provider plugins via a registry, not require new code per provider.
- **Tokscale as alternative wrapper** (considered, deferred): single Rust binary, 20+ CLIs, but lacks per-session output today. Decision for v1: stay with `@ccusage/*` because per-session output ships. Revisit when (a) we need a provider tokscale supports natively that ccusage doesn't, or (b) the session-grouping contribution lands upstream.
