---
status: in-progress
created: 2026-04-25
updated: 2026-08-18
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
- Stale `models.dev` pricing remains readable while one coalesced refresh runs per
  client. Concurrent background lookups and explicit refresh calls do not start
  duplicate network fetches.
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
| `project_id` | string | write-time project snapshot, null if unassigned at event time |
| `model` | string | e.g. `claude-sonnet-4-20250514` |
| `provider` | string | e.g. `anthropic`, `openai` |
| `tokens_in` | int64 | input token count |
| `tokens_cached_in` | int64 | cached input tokens (prompt caching). Always `= tokens_cached_read + tokens_cached_write` when the split is known; kept for existing rollup consumers (see `TaskSession additions` below) |
| `tokens_cached_read` | int64, nullable | cached-read portion of `tokens_cached_in`. NULL for legacy rows (pre-contract-v2) and when no cache data was reported - never `0`, since a missing split is not proof of zero cache usage |
| `tokens_cached_write` | int64, nullable | cached-write portion of `tokens_cached_in`. Same NULL rule as `tokens_cached_read` - cache write and cache read price at different per-million rates, so the split matters |
| `tokens_out` | int64, nullable | output token count. NULL means "never observed". New normalized prompt usage always carries `output_tokens_present` separately from the numeric value. Thus, an observed zero is stored as `0`, while the adapter_prompt.go context-occupancy fallback stores NULL because it cannot observe output tokens. For a legacy event without the presence field, the subscriber keeps the old rule: a non-estimated value or a nonzero value is observed. A downstream per-output-token measure can distinguish both states (contract v3) |
| `cost_subcents` | int64 | hundredths of a cent (renamed from `cost_cents`). UI divides by 10000 |
| `estimated` | bool | true when token counts are not authoritative for a complete turn: the adapter synthesised them (for example, codex-acp cumulative-delta inference), or the provider frame covers only part of the turn (for example, Codex's last request). Independent of `cost_source` |
| `turn_id` | string, nullable | the turn this event was recorded at completion of. NULL when no turn could be resolved (e.g. an error/interrupt completion outside the normal ready path) |
| `usage_event_id` | string, nullable | idempotency key derived from `(session_id, execution_id, prompt_generation)` at the publish site; unique when non-NULL (partial index) so a redelivered prompt-usage event does not double-insert or double-count the session rollup. NULL for legacy rows. Generation-less transports receive a random per-publish key, so those rows are intentionally not deduplicated |
| `cost_source` | enum, nullable | `provider_reported` \| `models_dev_list` \| `unpriced`. Records which layer of the two-layer lookup below actually produced `cost_subcents` - distinct from `estimated`. NULL for legacy rows |
| `rate_input_per_million` / `rate_cached_read_per_million` / `rate_cached_write_per_million` / `rate_output_per_million` | int64, nullable | the four models.dev rates actually applied, in subcents-per-million-tokens. Populated only when `cost_source = models_dev_list`; NULL otherwise (including `provider_reported`, where no rate table applies) |
| `pricing_catalog_version` | string, nullable | the models.dev cache's "as-of" identifier (RFC3339 load/fetch time - the dataset has no version field of its own) that produced the applied rates. Populated only when `cost_source = models_dev_list` |
| `cost_contract_version` | int64, nullable | in-band activation marker for this row's column semantics, since the point-in-time Rill extract has no schema versioning of its own. Current value `3` (v2 -> v3: `tokens_out` became nullable, normalized usage gained an explicit output-token presence signal, and partial-turn typed usage became estimated; v1 -> v2: the unpriced path stopped forcing `estimated=true`, see the `estimated` row above). NULL for legacy pre-contract rows - never backfilled |
| `provider_event_id` | string | null for wire-side rows; non-null for disk-runner rows. Format `ccusage:<provider>:<session_id>:<model>` |
| `provider_credits` | int64 | amp `credits` field (null elsewhere) |
| `occurred_at` | timestamp | when the cost was incurred |

Disk-runner rows are upserted by `(session_id, provider_event_id)`. For codex, after a successful aggregate row lands, the wire-side `estimated=true` rows for the same session are deleted to avoid double-counting.

Project cost attribution uses the current task assignment:

- `office_cost_events.project_id` is a write-time snapshot retained for cleanup and historical inspection.
- `tasks.project_id` is the current source of truth for project cost views and project budget totals.

**Contract v2 columns** (`tokens_cached_read`, `tokens_cached_write`, `turn_id`, `usage_event_id`, `cost_source`, the four `rate_*_per_million` columns, `pricing_catalog_version`, `cost_contract_version`) were added via an idempotent nullable-column migration (`migrateCostEventContract`); every legacy row reads NULL for all of them, never `0` - see the cross-cutting rule in `Persistence guarantees` below. Postgres parity follows the same migration shape per ADR 0027.

**Contract v3 semantics** keep the v2 column layout and pricing-source rules. They
define `estimated=true` for both synthesised totals and typed usage frames that
cover only part of a turn.

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

`cost_subcents`, `tokens_in`, `tokens_out`, and `tokens_cached_in` are maintained by the task cost ledger writer, not this Office subscriber ([task-cost-ledger](../task-cost-ledger/spec.md) AC-10/AC-21). They provide quick per-session totals without a scan of `task_usage_events`. An unknown ledger `tokens_out` value contributes zero to the numeric rollup, so the session output total is a lower bound when at least one ledger row has `tokens_out=NULL`. Consumers needing completeness must read the raw ledger. `tokens_cached_in` is in TaskSession and DTO, without a reader, so the rollup reconciles column-for-column against that ledger. The rollup consumes only the merged `tokens_cached_in` sum, not the `tokens_cached_read`/`tokens_cached_write` split, which exists solely on the ledger row for cost-per-token-type analysis (contract v2).

### Per-agent budget

Each agent instance has `budget_monthly_cents` set at creation (or inherited from a workspace default). The CEO proposes budgets in hire requests; users adjust via agent instance settings.

### Pricing cache (`models.dev`)

The `models.dev` dataset is fetched once per day to a workspace-local disk cache at `<data-dir>/cache/models-dev.json`. The file lives on disk full-fat; only queried models load into the in-memory map. Refresh runs in a background goroutine; refresh failures fall back to the existing on-disk file. Pricing is recorded per-million-tokens for input, cached read, cached write, and output separately (Anthropic charges different rates for cached read vs. cached write).

Each client permits only one physical refresh at a time. Stale lookups return
current data without waiting, while concurrent explicit `Refresh` callers wait
for and share the same result. A failed or canceled refresh releases the guard
so a later request can retry. Cache replacement uses a unique temporary file in
the cache directory and an atomic rename. Failure removes that temporary file
and leaves the last valid disk and memory data unchanged. The client marks the
cache fresh only after the new file is committed and the in-memory indexes are
replaced successfully.

### Per-CLI usage shapes (ACP wire)

- **claude-acp**: `result.usage` (camelCase, `cachedRead`/`cachedWriteTokens`), plus cumulative `usage_update.cost.amount` in USD; the adapter emits its nonnegative turn delta when present. claude-acp uses logical aliases (`sonnet`, `haiku`); provider-reported cost is the only accurate signal.
- **opencode-acp**: `result.usage` with `inputTokens`/`outputTokens`/`thoughtTokens` (no cached tokens). Optional `usage_update.cost.amount` (often `0` on BYOK).
- **gemini**: `result._meta.quota.token_count.{input_tokens, output_tokens}` (snake_case, no cached, no cost).
- **codex-acp**: 1.4.0+ emits a typed `result.usage` block on the prompt response, but its three response-construction sites (normal end_turn, cancelled, terminal failure) all hardcode it to the session's last model request, not a per-turn total — a turn making N requests reports only request N's counts. The adapter accepts this typed frame (input/output/cache split, when present) but always flags rows `estimated=true` until codex-acp emits a genuine per-turn total upstream. Prior to 1.4.0 (or if the frame is absent), the adapter falls back to nonnegative context-occupancy growth from `usage_update.used` as a per-turn estimate; that fallback cannot split input vs output and compaction or other decreases reset its baseline.
- **auggie**, **copilot-acp**: not tracked. `_meta.copilotUsage` is a billing multiplier; Copilot `/usage` would require scraping.

### Disk-runner provider coverage

- **codex** - disk runner is the preferred source. Cost rows promoted from `estimated=true` to `estimated=false` with full token split.
- **amp** - disk runner is the only source. Cost events emitted with `estimated=false`; `credits` captured in `provider_credits`.
- **claude, opencode, gemini** - disk runner NOT used; wire data is authoritative.
- **auggie, copilot** - no `@ccusage/*` package; out of scope.

## API surface

The raw cost-list response always includes `tokens_out`. It contains a number
for an observed count and JSON `null` for an unknown count. It does not omit
the field when the count is unknown.

### Cost resolution (two-layer lookup)

1. **Provider-reported cost** (`cost_source = provider_reported`). If the CLI emits cumulative USD session cost on `usage_update`, subtract the previously consumed session baseline and store the nonnegative turn delta as `int64(amount * 10000)` hundredths-of-a-cent. Ignore non-USD cost values. This is the only accurate path for claude-acp. No rate columns are recorded on this path.
2. **`models.dev` lookup** (`cost_source = models_dev_list`). For CLIs reporting tokens but no cost (gemini, opencode BYOK, codex fallback), resolve pricing against the cached dataset. The four applied per-million rates and the catalogue's `pricing_catalog_version` are recorded alongside `cost_subcents`.

When both miss (first-boot, no network, model unknown, proprietary id), the row records `cost_subcents=0` and `cost_source=unpriced`. `estimated` is NOT forced true on this path - it independently tracks whether the token counts themselves were synthesised (see the `office_cost_events` table above). Before contract v2, an unpriced row was indistinguishable from a token-synthesis row; downstream consumers that read `estimated` as a stand-in for pricing confidence should switch to `cost_source`. UI shows "pricing unavailable". Users can override pricing per model in workspace settings.

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

SQL token aggregates skip NULL values. Thus, an aggregate output-token total
is a lower bound if one or more source rows have unknown output counts. Cost
totals still include those rows. Use the raw cost-list response when a consumer
must determine if an output-token total is complete.

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
- **Manage budget policies** (`POST /workspaces/:wsId/budgets`, `PATCH /budgets/:id`, `DELETE /budgets/:id`): UI / admin only. The CEO's `kandev-team-admin` skill is the agent-facing surface for budget checks and proposals; CEO agents do not call the budget HTTP endpoints directly, they raise proposals that the user actions through the inbox.
- **Override pricing** (per-model overrides written into workspace settings): user only. Agents cannot mutate the pricing table.
- **Trigger pause** (the side effect of `CheckBudget` when `action_on_exceed=pause_agent`): performed by the `system` actor inside the cost subscriber, not by any caller. The agent pause it produces is identical to a user-initiated pause; only the user can resume by raising the budget or waiting for the monthly reset.

There is no per-field permission model. Conformance tests should assert that cost-read endpoints accept any authenticated workspace member, and that mutating budget endpoints either accept the admin / UI session or follow the same JWT-bypass rules used by the rest of the office API.

## Failure modes

- **models.dev miss**: row recorded with `cost_subcents=0` and `cost_source=unpriced`; `estimated` is unaffected (tracks token-synthesis independently, see the `office_cost_events` table). UI shows "pricing unavailable".
- **Prompt-usage event redelivered** (e.g. a reconnecting WS client replaying a buffered stream event): the second `CreateCostEvent` call collides on the unique `usage_event_id` partial index and is dropped as a no-op rather than double-inserting or double-counting the session rollup.
- **Writer silently stops**: every path a prompt-usage event can take (decode failure, missing task/session ids, task-fields lookup failure, insert failure or duplicate, or a successful write) increments an `expvar` counter (`cost_events_written_total` / `cost_events_dropped_total`, dev-mode `/debug/vars`) and logs, so a stopped writer is observable instead of silently producing zero rows.
- **`recordCostEventAndRollup` only inserts `office_cost_events`** ([task-cost-ledger](../task-cost-ledger/spec.md) AC-10/AC-21): it no longer increments the `task_sessions` rollup or shares a transaction with it. The ledger writer's own insert (`task_usage_events`) and its `task_sessions` rollup increment are wrapped in one transaction on its side, independent of this call, so a redelivery is idempotent on each ledger's own unique index without the two ledgers coordinating a shared transaction.
- **models.dev fetch fails or is canceled**: background refresh falls back to
  existing on-disk and in-memory data; no crash. The in-flight guard is released
  and a later refresh can retry.
- **models.dev refresh is requested concurrently**: one network request and one
  atomic cache replacement run per client. All explicit callers observe the
  shared result, and temporary files do not collide or remain after failure.
- **Disk-runner `npx` unavailable**: log one warning per provider per process lifetime, mark run skipped, backend continues. Codex sessions retain wire-side `estimated=true` rows; amp sessions remain untracked.
- **ccusage JSON schema drift**: schema validator returns decode error; office subscriber treats run as no-op (no rows touched). Codex falls back to wire-side estimated path; amp absent. Nightly fixture-smoke CI alerts maintainers.
- **`@ccusage/<provider>@latest` yanked**: next runner invocation fails; coverage degrades the same as parse failure.
- **Coalescing**: if `session/complete` fires while a runner is already executing for that provider, the second invocation is dropped. The 60s sweep catches sessions in the gap.

## Persistence guarantees

**Survives restart:**

- `office_cost_events` rows (full history, never trimmed). Disk-runner rows keyed by `(session_id, provider_event_id)` survive re-ingestion without duplicating; wire-side rows are additionally deduplicated by the unique `usage_event_id` partial index (contract v2). The wire-side `estimated=true` rows for codex are deleted once the matching aggregate row lands.
- **Column semantics never change value mid-series across a contract bump.** Legacy rows (written before a column's semantics changed) read NULL, never `0`, for a newly nullable value - the downstream point-in-time extract has no schema versioning, so a column whose meaning flips from "not recorded" to "recorded as zero" (or vice versa) partway through the series would be silently discontinuous. This applies both to columns introduced by contract v2 and to `tokens_out`, an existing column whose contract became nullable in v3: pre-v3 rows read `0`, not NULL, for `tokens_out`. Contract v3 changes the meaning of `estimated` for new rows only; legacy rows keep their recorded value. `cost_contract_version` on each row is the in-band marker a consumer checks instead of inferring from a date.
- `office_budget_policies` rows.
- `TaskSession.cost_subcents` / `tokens_in` / `tokens_out` / `tokens_cached_in` running totals, maintained by the task cost ledger writer, not this Office subscriber ([task-cost-ledger](../task-cost-ledger/spec.md) AC-10/AC-21); its ledger insert and rollup increment are atomic, so they cannot diverge on a partial write. The numeric `tokens_out` rollup is a lower bound when a source row has an unknown output count.
- Per-agent `budget_monthly_cents` stored on the agent instance row.
- Per-model pricing overrides stored in workspace settings.
- The on-disk `models.dev` cache at `<data-dir>/cache/models-dev.json`. Recovery on next boot: the in-memory pricing map is empty until first query; queries fall back to the on-disk file when the background refresh has not yet completed.
- A failed refresh never replaces or invalidates the last valid cache. Refresh
  coordination and temporary files are transient and do not survive restart.
- Activity log entries `budget.alert` and `budget.exceeded` (workspace-scoped, included in the standard office backup as part of normal SQLite persistence - see `persistence.Provide` for the snapshot policy).

**Does NOT survive restart:**

- In-memory pricing map inside the models.dev client. Rebuilt lazily on first lookup from the on-disk cache file.
- The disk-runner coalescing set (the in-process record of "already running for provider X"). After restart the 60s periodic sweep is the catch-up path - no in-flight session state is replayed.
- Wire-side `estimated=true` rows for codex sessions that completed during downtime: those are reconciled when the next disk-runner sweep promotes them to the aggregate row.
- The "monthly reset already ran for month M" guard is durable (idempotent by month boundary) so a restart on the 1st UTC does not refire the reset for agents that already returned to `idle`.
- Cached aggregation results in the frontend store (`office.costSummary`, `office.budgetPolicies`) - rehydrated from the API on next page load.

No TTL or retention is applied to `office_cost_events`; rows accumulate for the lifetime of the workspace. Cleanup happens only through workspace deletion (cascade via task workspace foreign key) and agent / project deletion garbage collection (see `office/repository/sqlite/runtime.go`).

## Scenarios

- **GIVEN** an ACP agent session processing a turn, **WHEN** its `complete` event carries normalized usage, **THEN** a cost event is created from the provider-reported USD delta or estimated via models.dev pricing. The session's cumulative `cost_subcents` is updated.

- **GIVEN** a model not found in the models.dev dataset and no user override configured, **WHEN** a cost event is recorded, **THEN** token counts are stored but `cost_subcents` is zero and `cost_source=unpriced`. The cost explorer shows "pricing unavailable" for that model.

- **GIVEN** a claude-acp turn that reports both cached-read and cached-write tokens, **WHEN** the cost event is recorded, **THEN** `tokens_cached_read` and `tokens_cached_write` are stored as the separate values reported (not merged), `tokens_cached_in` equals their sum, and `cost_subcents` reflects each portion priced at its own per-million rate.

- **GIVEN** a codex-acp turn using the context-occupancy-growth fallback (no typed usage frame available; tokens synthesised from `usage_update.used` growth, `estimated=true`), **WHEN** the cost event is recorded, **THEN** `tokens_cached_read` and `tokens_cached_write` are NULL (never `0`) because the split was never observed, while `tokens_cached_in` and `cost_subcents` are still recorded from the synthesised totals.

- **GIVEN** a codex-acp 1.4.0+ turn whose typed usage frame reports a nonzero cache split, **WHEN** the cost event is recorded, **THEN** `estimated=true` (the frame is the last request only, not a per-turn total) and `tokens_cached_read`/`tokens_cached_write` are stored as the reported split. When the frame's cache fields are both zero, the split is stored as NULL rather than `0/0`, matching the "split never observed" rule above rather than distinguishing a reported zero from an absent field.

- **GIVEN** a synthesised-usage turn (`estimated=true`, no output token count observed, a provider-reported cost sample present), **WHEN** the cost event is recorded, **THEN** `tokens_out` is NULL (never `0`) even though `cost_subcents` is positive. A row must not present an unmeasured output count as a measured zero.

- **GIVEN** an estimated turn with `output_tokens_present=true` and `output_tokens=0`, **WHEN** the cost event is recorded, **THEN** `tokens_out` is a non-NULL zero because the adapter observed the value.

- **GIVEN** a turn priced via the models.dev lookup, **WHEN** the cost event is recorded, **THEN** `cost_source=models_dev_list` and the row carries the four `rate_*_per_million` values and `pricing_catalog_version` actually applied, distinguishable from a provider-reported row (`cost_source=provider_reported`, no rate columns) and an unpriced row (`cost_source=unpriced`, `cost_subcents=0`).

- **GIVEN** a prompt-usage event for an in-progress turn, **WHEN** the cost event is recorded at turn completion, **THEN** `turn_id` is populated with the turn that was resolved at completion time and `usage_event_id` is set to a deterministic id derived from the underlying completion's identity.

- **GIVEN** the same underlying prompt-usage completion is delivered twice (e.g. a reconnecting WS client replaying a buffered stream event), **WHEN** the second copy is processed, **THEN** it derives the same `usage_event_id` and the `office_cost_events` insert is rejected by the unique partial index; the `task_sessions` rollup is unaffected, per the ledger writer's idempotent insert ([task-cost-ledger](../task-cost-ledger/spec.md) AC-10/AC-21).

- **GIVEN** an `office_cost_events` table that predates contract v2, **WHEN** the repository boots and runs its migration, **THEN** the new columns are added nullable, every pre-existing row reads NULL (never `0`) for all of them, and running the migration again on the same database is a no-op.

- **GIVEN** many stale pricing lookups and direct refresh calls arrive together,
  **WHEN** refresh starts, **THEN** one network fetch runs for that client while
  stale lookup data remains available and direct callers share its result.

- **GIVEN** a refresh fails or its context is canceled, **WHEN** a later lookup
  requests refresh, **THEN** the later refresh can run, the previous valid cache
  remains readable, and no temporary cache file is left behind.

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

## Implementation plan

[Backend failure containment](../../plans/backend-failure-containment/plan.md)
