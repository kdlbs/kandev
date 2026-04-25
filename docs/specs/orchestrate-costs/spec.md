---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Cost Tracking & Budget Management

## Why

Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

Orchestrate adds per-session cost estimation, aggregated views by agent/project/model, and budget policies that can alert or hard-stop agents when estimated limits are exceeded.

**Important framing**: costs displayed in Orchestrate are **estimated costs** based on token usage and published model pricing. They may not reflect the user's actual bill -- users on subscription plans (e.g. Claude Max, Copilot Enterprise) pay a flat fee regardless of token consumption. The cost explorer is a visibility and governance tool, not a billing system.

## What

### Cost events

- Every agent session generates cost events as it runs.
- A cost event records:
  - `id`: unique identifier.
  - `session_id`: which session incurred the cost.
  - `task_id`: which task the session belongs to.
  - `agent_instance_id`: which orchestrate agent instance (null for non-orchestrate sessions).
  - `project_id`: which project the task belongs to (null if unassigned).
  - `model`: the model used (e.g. "claude-sonnet-4-20250514").
  - `provider`: the provider (e.g. "anthropic", "openai").
  - `tokens_in`: input token count for this event.
  - `tokens_cached_in`: cached input tokens (for prompt caching).
  - `tokens_out`: output token count.
  - `cost_cents`: estimated cost in hundredths of a cent (integer, avoids floating point).
  - `occurred_at`: when the cost was incurred.
- For ACP-based agents (Claude, Codex, etc.), cost events are populated from the existing `context_window` update events that already flow through the streaming pipeline. These events include token counts that are used for cost estimation. A new handler subscribes to these events and writes cost rows.
- For non-ACP agents that don't report token usage, cost events are not generated automatically. Manual cost entry or future adapter-specific parsing can fill the gap.

### Model pricing via models.dev

- Pricing data is sourced from [models.dev](https://github.com/anomalyco/models.dev), an open-source model pricing database. The backend matches the current provider and model identifier against the models.dev dataset to look up per-token costs.
- Pricing is stored as cost-per-million-tokens for input, cached input, and output separately.
- The models.dev data is periodically synced (on startup and on a configurable interval) so new models are picked up automatically.
- Users can override pricing per model in workspace settings (e.g. for custom API pricing tiers or enterprise discounts).
- If a model is not found in models.dev and no user override exists, cost events record token counts but `cost_cents` is zero. The UI shows "pricing unavailable" for those entries.

### Cost aggregation

- Cost events are aggregated on read (not pre-computed) for flexibility:
  - **By agent instance**: total spend per agent over a time window.
  - **By project**: total spend across all tasks in a project.
  - **By task**: total spend across all sessions of a task.
  - **By model**: spend broken down by model used.
  - **By time**: daily/weekly/monthly spend trends.
- The existing `TaskSession` model gains `cost_cents`, `tokens_in`, and `tokens_out` fields that are incrementally updated as cost events arrive, providing quick per-session totals without scanning the cost_events table.

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

- `/orchestrate/company/costs` page shows:
  - **Summary bar**: total spend this month, budget utilization gauge, trend vs. last month.
  - **By agent**: table of agent instances with spend, budget, utilization percentage, status.
  - **By project**: table of projects with total task spend.
  - **By model**: bar chart of spend per model.
  - **By time**: line chart of daily spend over the last 30 days.
  - **Budget policies**: list of active policies with edit/create/delete controls.
- Clicking an agent row drills into the agent detail page (`/orchestrate/agents/[id]`). The Runs tab shows per-session cost; the Overview tab shows cumulative cost and budget gauge.
- Clicking a project row drills into the project detail page.

### Dashboard integration

- The Orchestrate dashboard at `/orchestrate` shows:
  - Per-agent budget utilization gauges on agent status cards.
  - A "Spend this month" summary stat.

## Scenarios

- **GIVEN** an ACP agent session processing a turn, **WHEN** the `context_window` event arrives with token counts, **THEN** a cost event is created with the estimated cost looked up from models.dev pricing. The session's cumulative `cost_cents` is updated.

- **GIVEN** a model not found in the models.dev dataset and no user override configured, **WHEN** a cost event is recorded, **THEN** token counts are stored but `cost_cents` is zero. The cost explorer shows "pricing unavailable" for that model.

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
