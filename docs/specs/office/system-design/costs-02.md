---
status: draft
system: office
requirements:
  - REQ-OFFICE-COSTS-001
created: 2026-04-25
updated: 2026-08-18
owners:
  - cfl
---
# Office: Cost Tracking & Budget Management System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-COSTS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COSTS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

- **GIVEN** the same underlying prompt-usage completion is delivered twice (e.g. a reconnecting WS client replaying a buffered stream event), **WHEN** the second copy is processed, **THEN** it derives the same `usage_event_id` and the `office_cost_events` insert is rejected by the unique partial index; the task ledger writer's idempotent insert keeps the `task_sessions` rollup from being incremented a second time ([task-cost-ledger](../../task-cost-ledger/spec.md) AC-10/AC-21).

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

[Backend failure containment](../../../plans/backend-failure-containment/plan.md)
