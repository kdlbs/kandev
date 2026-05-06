---
status: draft
created: 2026-04-28
owner: cfl
---

# Idle Wakeup Skip

## Why

Kandev's office scheduler wakes agents on a periodic heartbeat (default interval: configurable per agent). On each heartbeat the scheduler claims the wakeup, resolves an executor, builds a full prompt, and launches an agent session — even when the agent has nothing to do. An idle worker with no assigned tasks burns real LLM tokens doing a complete turn that produces no useful work.

In practice, worker agents sit idle most of the time: tasks are assigned in bursts, worked to completion, then nothing for hours. Every heartbeat during that idle window is a wasted invocation. At a modest heartbeat rate (e.g. one per minute) with no assigned tasks, a worker can easily consume thousands of tokens per day purely from "nothing to do" heartbeats. At scale across many agents this is the dominant cost category.

A simple pre-flight check — query the DB for actionable tasks before processing the wakeup — eliminates these idle invocations with no change to agent correctness.

## What

### Idle skip check

Before processing a `heartbeat` wakeup, the scheduler checks whether the agent has any tasks in actionable states assigned to it. If none are found, the wakeup is skipped (marked finished) and no agent session is launched.

**Actionable states:** `TODO` and `IN_PROGRESS`. Tasks in terminal states (`DONE`, `CANCELLED`, `ARCHIVED`) or review-gated states (`IN_REVIEW`) do not count — the agent cannot do useful work on them without an external event.

**What triggers the skip:**
- Wakeup reason is `heartbeat`
- Agent has `skip_idle_wakeups = true` (see below)
- `CountActionableTasksForAgent` query returns 0

All three conditions must hold. If any condition is false, processing continues as normal.

### Event-triggered wakeups always proceed

The skip applies **only** to `heartbeat` wakeups. All other wakeup reasons bypass the check entirely and are processed unconditionally:

| Reason | Skippable? |
|--------|-----------|
| `heartbeat` | Yes (if `skip_idle_wakeups = true` and no actionable tasks) |
| `task_assigned` | No |
| `task_comment` | No |
| `task_blockers_resolved` | No |
| `task_children_completed` | No |
| `approval_resolved` | No |
| `routine_trigger` | No |
| `budget_alert` | No |
| `agent_error` | No |

Event-triggered wakeups already carry a specific task or event ID; the agent has concrete work to do. Heartbeats are speculative — the agent wakes up to check whether there is anything to do, which is exactly the question we are answering in advance with the DB query.

### Per-agent configuration: `skip_idle_wakeups`

Whether idle heartbeats are skipped is configurable per agent instance via a `skip_idle_wakeups` boolean field.

**Defaults by role:**

| Role | Default |
|------|---------|
| `worker` | `true` |
| `specialist` | `true` |
| `assistant` | `true` |
| `ceo` | `false` |

CEO agents default to `false` because their heartbeat purpose is fundamentally different from worker heartbeats. A CEO wakes on heartbeat for self-directed coordination work — surveying project status, reassigning tasks, spinning up new agents, checking budget utilization — activities that do not require a task assigned directly to the CEO. Skipping CEO heartbeats when the CEO has no directly assigned tasks would defeat the purpose of the CEO role.

Users can override the default for any agent. A CEO agent that has been configured to only react to events (no proactive heartbeat work) can set `skip_idle_wakeups = true`. A worker agent acting as a monitoring bot with no persistent task assignments can set `skip_idle_wakeups = false`.

### Observability

Skipped wakeups are not silently discarded. The scheduler:
1. Logs at `INFO` level with fields `wakeup_id`, `agent_instance_id`, `agent_name`, and `reason = "no_actionable_tasks"`.
2. Marks the wakeup `finished` (normal terminal state) — no special status needed.
3. Records an activity log entry (`wakeup_idle_skipped`) identical in structure to the existing `wakeup_budget_blocked` entry, so skips appear in the office activity feed and can be queried for analytics.

The activity entry lets operators see how many idle heartbeats were suppressed per agent per day, confirming the feature is working and providing data for tuning heartbeat intervals.

### DB query

The check is a single indexed count query:

```sql
SELECT COUNT(*) FROM tasks
WHERE assignee_agent_instance_id = $agentID
  AND state IN ('TODO', 'IN_PROGRESS')
  AND archived_at IS NULL
```

This query is fast: `assignee_agent_instance_id` is already indexed (or can be indexed as part of this change), the state filter is selective, and the result is a scalar. It adds negligible overhead on wakeups that are not skipped and eliminates the full session launch cost for wakeups that are.

### Placement in processWakeup

The idle skip check runs **after** the agent status guard (paused/stopped check) and **before** the task checkout, budget check, executor resolution, and session launch. This ordering is important:

1. **After status guard**: no need to run the query if the agent is paused or stopped — those guards already finish the wakeup early.
2. **Before checkout/budget/executor**: the check is the cheapest possible gate. We want to short-circuit before any heavier operations.

```
processWakeup:
  1. Guard: agent status (existing)
  2. [NEW] Idle skip: heartbeat + skip_idle_wakeups=true + 0 actionable tasks → skip
  3. Checkout: atomic task lock (existing, only when payload has task_id)
  4. Budget: pre-execution budget check (existing)
  5. Executor: resolve config (existing)
  6. Launch: session start (existing)
  7. Finish: mark wakeup done (existing)
```

## Scenarios

- **GIVEN** a worker agent with `skip_idle_wakeups = true` and no assigned tasks in `TODO` or `IN_PROGRESS` state, **WHEN** a `heartbeat` wakeup is claimed, **THEN** the scheduler logs `wakeup_idle_skipped`, marks the wakeup `finished`, records an activity entry, and does not launch an agent session.

- **GIVEN** the same worker agent, **WHEN** a `task_assigned` wakeup arrives for the same agent, **THEN** the skip check is not performed and the wakeup proceeds normally to session launch.

- **GIVEN** a worker agent with `skip_idle_wakeups = true` that has one task in `IN_PROGRESS` state and one task in `DONE`, **WHEN** a `heartbeat` wakeup is claimed, **THEN** the idle skip check finds 1 actionable task and the wakeup proceeds normally to session launch.

- **GIVEN** a CEO agent with default `skip_idle_wakeups = false`, **WHEN** a `heartbeat` wakeup is claimed and the CEO has no directly assigned tasks, **THEN** the skip check is not performed and the wakeup proceeds normally so the CEO can do proactive coordination work.

- **GIVEN** a worker agent with `skip_idle_wakeups = false` (overridden), **WHEN** a `heartbeat` wakeup is claimed and the agent has no actionable tasks, **THEN** the skip check is not performed and the wakeup proceeds normally.

- **GIVEN** the office activity feed for a workspace, **WHEN** a worker's heartbeat has been idle-skipped, **THEN** a `wakeup_idle_skipped` activity entry appears, showing the agent name and wakeup ID, so operators can see the suppression rate.

## Out of scope

- Dynamically adjusting heartbeat interval based on workload (backpressure scheduling).
- Suppressing non-heartbeat wakeups based on task state.
- Configuring which task states count as "actionable" per agent or workspace.
- Wakeup analytics dashboard (covered by the existing activity log query).
