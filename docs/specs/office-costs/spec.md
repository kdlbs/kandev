---
status: in-progress
created: 2026-04-25
updated: 2026-05-12
owner: cfl
---

# Office: Cost Tracking & Budget Management

## Why

Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

Office adds per-session cost estimation, aggregated views by agent/project/model, and budget policies that can alert or hard-stop agents when estimated limits are exceeded.

**Important framing**: costs displayed in Office are **estimated costs** based on token usage and published model pricing. They may not reflect the user's actual bill -- users on subscription plans (e.g. Claude Max, Copilot Enterprise) pay a flat fee regardless of token consumption. The cost explorer is a visibility and governance tool, not a billing system.

## What

### Cost events

- Every agent session generates cost events as it runs.
- A cost event records:
  - `id`: unique identifier.
  - `session_id`: which session incurred the cost.
  - `task_id`: which task the session belongs to.
  - `agent_instance_id`: which office agent instance (null for non-office sessions).
  - `project_id`: which project the task belongs to (null if unassigned).
  - `model`: the model used (e.g. "claude-sonnet-4-20250514").
  - `provider`: the provider (e.g. "anthropic", "openai").
  - `tokens_in`: input token count for this event.
  - `tokens_cached_in`: cached input tokens (for prompt caching).
  - `tokens_out`: output token count.
  - `cost_subcents`: estimated cost stored as hundredths of a cent (int64, avoids floating point). UI divides by 10000 when rendering dollars. The column / field name was renamed from `cost_cents` to remove the misleading suffix.
  - `estimated`: boolean flag set when token counts were synthesised (e.g. cumulative-delta inference for codex-acp, which emits no per-turn split) rather than reported directly by the agent.
  - `occurred_at`: when the cost was incurred.
- For ACP-based agents, cost events are populated from the `complete` event emitted at end-of-turn. The adapter extracts a `usage` object from each CLI's response shape, which differs across vendors:
  - **claude-acp**: `result.usage` (camelCase, with `cachedRead`/`cachedWriteTokens`), plus `usage_update.cost.amount` in USD which is preferred when present (claude-acp's model identifiers are logical aliases like `sonnet` / `haiku`, so provider-reported cost is the only accurate signal for that CLI).
  - **opencode-acp**: `result.usage` with `inputTokens` / `outputTokens` / `thoughtTokens` (no cached tokens). Optional `usage_update.cost.amount` (often `0` on BYOK configs).
  - **gemini**: `result._meta.quota.token_count.{input_tokens, output_tokens}` (snake_case, no cached, no cost).
  - **codex-acp**: no per-turn usage frame; only a cumulative `usage_update.used` counter. The adapter infers per-turn deltas and flags the row with `estimated=true`. Input vs output cannot be split.
- ACP CLIs that emit no usable token telemetry — currently **auggie** and **copilot-acp** — are not tracked in this iteration. `_meta.copilotUsage` is a billing multiplier, not a token count, and Copilot's `/usage` slash command would need scraping rather than passive ingestion.
- For non-ACP agents that don't report token usage, cost events are not generated automatically. Manual cost entry or future adapter-specific parsing can fill the gap.

### Cost resolution (two-layer lookup)

Cost is resolved per cost event using the first source that produces a value:

1. **Provider-reported cost.** If the agent CLI emits a USD amount on its `usage_update` frame (claude-acp always does; opencode-acp sometimes does), the row is stored as `int64(amount * 10000)` hundredths-of-a-cent and pricing lookup is skipped. This is the only accurate path for claude-acp, which surfaces logical model aliases (`default`, `sonnet`, `haiku`) rather than real model names in any frame — the underlying mapping is owned by claude-code and shifts whenever Anthropic flips the default. Trusting the CLI's reported cost sidesteps that entirely.
2. **`models.dev` lookup.** For CLIs that report tokens but no cost (gemini, opencode on BYOK, codex fallback), the backend resolves pricing against [models.dev](https://github.com/anomalyco/models.dev). The dataset is fetched once per day to a workspace-local disk cache (`<data-dir>/cache/models-dev.json`) and indexed lazily in memory — the file lives on disk full-fat, but only models actually queried get loaded into the in-memory map. Refresh runs in a background goroutine; refresh failures fall back to the existing on-disk file rather than crashing. Pricing is recorded per-million-tokens for input, cached read, cached write, and output separately. Anthropic charges different rates for cached read vs. cached write and the lookup preserves that.

No static fallback table exists. When both layers miss — first-boot before the cache warms, no network, model not yet in models.dev, or a proprietary id with no public price — the row is recorded with `cost_subcents=0` and `estimated=true`; the UI shows "pricing unavailable". Users can override pricing per model in workspace settings (e.g. for custom API pricing tiers or enterprise discounts).

### Cost aggregation

- Cost events are aggregated on read (not pre-computed) for flexibility:
  - **By agent instance**: total spend per agent over a time window.
  - **By project**: total spend across all tasks in a project.
  - **By task**: total spend across all sessions of a task.
  - **By model**: spend broken down by model used.
  - **By time**: daily/weekly/monthly spend trends.
- The existing `TaskSession` model gains `cost_subcents`, `tokens_in`, and `tokens_out` fields that are incrementally updated as cost events arrive, providing quick per-session totals without scanning the cost_events table.

### Budget policies

- A budget policy defines a spending limit for a scope:
  - `scope_type`: `agent` (per agent instance), `project`, or `workspace`.
  - `scope_id`: the entity ID.
  - `limit_cents`: the spending cap in hundredths of a cent.
  - `period`: `monthly` (resets on calendar month), `total` (lifetime).
  - `alert_threshold_pct`: percentage at which to surface a warning (default 80%).
  - `action_on_exceed`: what happens when the limit is hit:
    - `notify_only`: surface alert in inbox, no enforcement.
    - `pause_agent`: auto-pause the agent instance (status -> `paused`, no new wakeups processed).
    - `block_new_tasks`: prevent new task assignment to this agent but let current work finish.
- Multiple policies can apply to the same scope (e.g. a monthly + total limit).

### Budget enforcement

- After each cost event, the system checks all applicable budget policies for the affected scopes.
- If `alert_threshold_pct` is crossed: an inbox item is created for the user and, for agent-scoped budgets, a `budget_alert` wakeup is queued for the CEO.
- If `limit_cents` is exceeded and `action_on_exceed=pause_agent`:
  - The agent instance's status moves to `paused` with `pause_reason=budget_exceeded`.
  - Pending wakeups for this agent are marked `finished` without processing.
  - The current session (if any) completes its turn but no further prompts are sent.
  - An activity log entry is recorded.
- Monthly budgets reset at midnight UTC on the 1st of each month. The reset is idempotent -- if the backend restarts mid-month, the reset only fires once.

### Agent budget on instance

- Each agent instance has a `budget_monthly_cents` field set at creation (or inherited from a workspace default).
- The CEO can propose budgets for new hires in the hire request.
- Users can adjust budgets at any time via the agent instance settings.
- The hire approval flow shows the proposed budget for review.

### Cost explorer UI

- `/office/company/costs` page shows:
  - **Summary bar**: total spend this month, budget utilization gauge, trend vs. last month.
  - **By agent**: table of agent instances with spend, budget, utilization percentage, status.
  - **By project**: table of projects with total task spend.
  - **By model**: bar chart of spend per model.
  - **By time**: line chart of daily spend over the last 30 days.
  - **Budget policies**: list of active policies with edit/create/delete controls.
- Clicking an agent row drills into the agent detail page (`/office/agents/[id]`). The Runs tab shows per-session cost; the Overview tab shows cumulative cost and budget gauge.
- Clicking a project row drills into the project detail page.

### Dashboard integration

- The Office dashboard at `/office` shows:
  - Per-agent budget utilization gauges on agent status cards.
  - A "Spend this month" summary stat.

## Scenarios

- **GIVEN** an ACP agent session processing a turn, **WHEN** the `context_window` event arrives with token counts, **THEN** a cost event is created with the estimated cost looked up from models.dev pricing. The session's cumulative `cost_subcents` is updated.

- **GIVEN** a model not found in the models.dev dataset and no user override configured, **WHEN** a cost event is recorded, **THEN** token counts are stored but `cost_subcents` is zero. The cost explorer shows "pricing unavailable" for that model.

- **GIVEN** an agent instance with a monthly budget of $10 and 80% alert threshold, **WHEN** spending reaches $8, **THEN** an alert appears in the user's inbox and a `budget_alert` wakeup is queued for the CEO.

- **GIVEN** an agent instance with a monthly budget of $10 and `action_on_exceed=pause_agent`, **WHEN** spending exceeds $10, **THEN** the agent is paused, pending wakeups are cancelled, and an activity log entry records the auto-pause.

- **GIVEN** a paused agent (budget exceeded), **WHEN** the user increases the budget via the agent settings UI, **THEN** the agent's status returns to `idle` and wakeup processing resumes.

- **GIVEN** it is the 1st of a new month, **WHEN** the scheduler runs its monthly reset, **THEN** all monthly budget spend counters reset to zero. Previously paused agents (budget-exceeded) return to `idle` if their new month's spend is within limits.

- **GIVEN** a user on the cost explorer page, **WHEN** they view "By agent", **THEN** they see each agent instance with its monthly spend, budget limit, utilization percentage, and a visual gauge.

## Out of scope

- Actual billing integration or payment processing (costs are estimates, not invoices).
- Tracking real spend for subscription-based plans (flat-fee subscriptions are not modeled).
- Per-turn cost limits (budgets are per-period, not per-turn).
- Cost allocation across multiple users (single-user workspace model).
- Cost forecasting or spend predictions.
