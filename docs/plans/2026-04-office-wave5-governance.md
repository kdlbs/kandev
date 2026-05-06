# Office Wave 5: Cost Tracking, Budgets, Inbox, Activity & Dashboard

**Date:** 2026-04-26
**Status:** proposed
**Specs:** `office-costs`, `office-inbox`
**UI Reference:** `docs/plans/2026-04-office-ui-reference.md` (dashboard, costs page, budget cards, inbox, activity page, approval rows)
**Depends on:** Wave 3 (task model), Wave 4 partially (wakeups for budget alerts)

## Problem

Agents run autonomously but there's no visibility into spending, no way to set guardrails, no centralized view of items needing attention, and no audit trail. This wave adds the monitoring and governance layer.

## Scope

### 5A: Cost Tracking (parallelizable)

**Backend:**

**models.dev pricing integration** (`internal/office/service/pricing.go`):
- Fetch pricing data from models.dev GitHub repo (JSON/YAML)
- Parse model entries: provider, model ID, input cost per million tokens, output cost per million tokens, cached input cost
- Cache in memory, refresh on startup and periodically (configurable, default 24h)
- Fallback: if model not found, `cost_cents = 0` with a flag for "pricing unavailable"
- User overrides: stored in workspace settings, checked before models.dev data

**Cost event handler** (`internal/office/service/cost_handler.go`):
- Subscribe to existing ACP `context_window` events (already flowing through `event_handlers_streaming.go`)
- On each event: extract token counts (tokens_in, tokens_cached_in, tokens_out), model identifier
- Look up pricing for the model
- Calculate cost: `(tokens_in * input_price + tokens_cached_in * cached_price + tokens_out * output_price) / 1_000_000`
- Write `office_cost_events` row
- Increment `TaskSession.cost_cents`, `TaskSession.tokens_in`, `TaskSession.tokens_out`
- Check budget policies (see 5B)

**Cost aggregation queries** (`internal/office/repository/sqlite/costs.go`):
- `GetCostsByAgent(ctx, workspaceID, timeRange)` -- group by agent_instance_id
- `GetCostsByProject(ctx, workspaceID, timeRange)` -- group by project_id
- `GetCostsByModel(ctx, workspaceID, timeRange)` -- group by model
- `GetCostsByDay(ctx, workspaceID, days)` -- daily totals for chart
- `GetCostSummary(ctx, workspaceID)` -- total this month, total all time

**Frontend** (`/office/company/costs`):
- Summary bar: total spend this month, trend vs last month
- Tabs: By Agent | By Project | By Model | By Time
- By Agent: table with agent name, spend, budget, utilization %, status
- By Project: table with project name, task count, total spend
- By Model: bar chart
- By Time: line chart (last 30 days)
- Budget policies section: list with create/edit/delete (see 5B)

### 5B: Budget Policies & Enforcement (parallelizable with 5A)

**Backend:**

**Repository** (`internal/office/repository/sqlite/budgets.go`):
- Full CRUD for `office_budget_policies`
- `GetPoliciesForScope(ctx, scopeType, scopeID)` -- find applicable policies
- `GetSpendForScope(ctx, scopeType, scopeID, period)` -- aggregate cost_events for period

**Service** (`internal/office/service/budgets.go`):
- `CheckBudget(ctx, agentInstanceID, projectID)`:
  - Called after each cost event
  - Find all applicable policies (agent-level, project-level, workspace-level)
  - For each policy: calculate current spend for the period
  - If spend >= alert_threshold_pct * limit: create inbox item, queue `budget_alert` wakeup for CEO
  - If spend >= limit and action=pause_agent: set agent status to `paused`, pause_reason=`budget_exceeded`, cancel pending wakeups, log activity
- `MonthlyReset()`:
  - Called by scheduler on 1st of month
  - Reset monthly spend counters
  - Un-pause agents that were budget-paused if new month spend is within limits
  - Idempotent (check if already reset this month)

**Frontend:**
- Budget policy CRUD in cost explorer page
- Budget gauge on agent cards (overview + agents list)
- Alert indicators in inbox

### 5C: Activity Log (parallelizable)

**Backend:**

**Repository** (`internal/office/repository/sqlite/activity.go`):
- `Log(ctx, entry)` -- append-only insert
- `List(ctx, workspaceID, filters)` -- paginated, filterable by actor_type, action, target_type, time range
- `ListRecent(ctx, workspaceID, limit)` -- for dashboard

**Service** (`internal/office/service/activity.go`):
- `LogActivity(ctx, actorType, actorID, action, targetType, targetID, details)` -- helper
- Called from all office services on mutations (agent CRUD, skill CRUD, approval resolution, budget events, etc.)
- Emit `office.inbox.item` event for notification-worthy entries

**Frontend** (`/office/company/activity`):
- Chronological feed with filters
- Each entry: actor avatar/name, action verb, target link, timestamp
- Filters: actor type, action, target type, time range
- Click target to navigate to detail page

### 5D: Inbox & Approvals (depends on 5A-5C)

**Backend:**

**Inbox service** (`internal/office/service/inbox.go`):
- Computed view, no dedicated table. Queries:
  - Pending approvals from `office_approvals` where status=pending
  - Budget alerts from activity log where action=budget.alert and unresolved
  - Agent errors from activity log where action=agent.error and recent
  - Review requests from tasks where status=in_review and has reviewers/approvers
- `GetInboxItems(ctx, workspaceID)` -> unified list sorted by recency
- `GetInboxCount(ctx, workspaceID)` -> badge count

**Approval service** (`internal/office/service/approvals.go`):
- `Create(ctx, approval)` -- insert + emit event + log activity
- `Decide(ctx, approvalID, status, decidedBy, note)`:
  - Update approval status
  - If `hire_agent` approved: activate agent instance (status idle)
  - If `task_review` approved: advance execution policy stage or move to done
  - If `task_review` rejected: return task to in_progress
  - If `skill_creation` approved: add skill to registry
  - Queue `approval_resolved` wakeup for requesting agent
  - Log activity
- `GetPending(ctx, workspaceID)` -- list pending approvals

**Execution policy flow** (`internal/office/service/execution_policy.go`):
- `EnterReviewStage(ctx, taskID)`:
  - Set execution_state to first review stage
  - Wake all reviewer participants in parallel
- `RecordParticipantResponse(ctx, taskID, participantID, verdict, comments)`:
  - Update execution_state with response
  - If all approvals_needed met: check verdict
    - All approve: advance to next stage (or done)
    - Any reject: wait for remaining, then return to in_progress with all feedback aggregated
- `AdvanceStage(ctx, taskID)`:
  - Move to next stage in execution_policy
  - If next is review: wake reviewers
  - If next is approval: create inbox items
  - If no more stages: move task to done, resolve blockers

**Notifications:**
- New inbox items trigger notifications via existing providers
- Add event type `office.inbox_item` to notification service
- Subscribe Local and System providers by default

**Frontend** (`/office/inbox`):
- List of inbox items grouped by type
- Each item: type icon, summary, agent/task link, timestamp, action buttons
- Approval items: Approve/Reject buttons with optional note input
- Badge count on sidebar "Inbox" link
- Archive view for resolved items

### 5E: Dashboard Page (depends on 5A-5D)

**Frontend** (`/office`):
- Agent status cards: each agent instance with status dot, current task, budget gauge
- Agents enabled count: N total, X running, Y paused, Z errors
- Run activity chart: sparkline of sessions over last 14 days
- Spend this month: summary stat
- Recent activity feed: last 10 entries from activity log

**API:**
- `GET /api/v1/office/workspaces/:wsId/dashboard` returns:
  - Agent instance summaries (status, current task, budget usage)
  - Run counts by day (last 14 days)
  - Cost summary (this month)
  - Recent activity entries

## Tests

- Cost handler: token event -> cost event created with correct pricing
- Cost handler: unknown model -> cost_cents=0
- Budget check: alert at threshold, pause at limit
- Monthly reset: idempotent, un-pause budget-paused agents
- Activity log: entries created on mutations
- Inbox: computed view aggregates approvals + alerts + errors
- Approval flow: create -> decide -> wakeup emitted -> agent/task state updated
- Execution policy: multi-reviewer parallel, wait for all, aggregate feedback
- Execution policy: sequential stages (review -> approval -> done)
- Notifications: inbox item triggers browser notification

## Verification

1. `make -C apps/backend test` passes
2. Cost events appear when agent sessions run
3. Cost explorer shows correct breakdowns
4. Budget policy pauses agent when exceeded
5. Inbox shows pending approvals, budget alerts, errors
6. Approve/reject works and triggers wakeups
7. Multi-reviewer flow: all reviewers complete -> task advances or returns with all feedback
8. Dashboard shows live agent statuses and activity
9. Browser notification fires on new inbox item
