---
status: draft
created: 2026-04-28
owner: cfl
---

# Recovery & Reliability Hardening

## Why

Three silent failure modes exist in the current wakeup scheduler:

1. **Stale wakeup execution.** A wakeup is queued for agent A on task T. Before the wakeup is claimed, the task is reassigned to agent B — or completed. The wakeup fires anyway, launching agent A against work it no longer owns. The agent takes action on a task it has no business touching.

2. **Stale retry execution.** A wakeup fails and is scheduled for retry with exponential backoff. Between failure and retry, the task is reassigned. The retry fires after the backoff window and runs the wrong agent — or a cancelled task — without checking whether the context is still valid.

3. **Unstarted task.** An assigned `TODO` task has no queued wakeup and no prior run — either because the wakeup was lost on a restart, never queued, or consumed by a bug. The task sits indefinitely. No heartbeat will spontaneously pick it up; the only recovery path today is a manual board nudge.

These are not edge cases. They occur naturally during normal agent orchestration: tasks are reassigned, retries back off for minutes or hours, and backend restarts lose in-flight queue state. The result is agents that silently do nothing, or worse, do the wrong thing.

## What

### 1. Staleness check before claiming a wakeup

Before `processWakeup` proceeds past the agent status guard, the scheduler checks whether the wakeup's context is still valid. This runs on every wakeup that carries a `task_id` in its payload.

**Staleness conditions** (each produces a distinct cancel reason):

| Condition | Cancel reason |
|---|---|
| Task not found | `task_not_found` |
| Task assignee changed (`task.AssigneeAgentInstanceID != wakeup.AgentInstanceID`) | `assignee_changed` |
| Task reached terminal state (`DONE`, `CANCELLED`, `ARCHIVED`) | `task_terminal` |
| Task's review stage participant changed | `review_participant_changed` |

A stale wakeup is cancelled (status: `cancelled`), not retried. Cancellation is idempotent and logged. The checkout lock is released if held.

**Placement in `processWakeup`:**

```
1. Guard: agent status (existing)
2. [NEW] Staleness check: task assignee / terminal state / review participant
3. Idle skip (existing)
4. Checkout: atomic task lock (existing)
5. Budget check (existing)
6. Executor resolution (existing)
7. Launch (existing)
8. Finish (existing)
```

### 2. Retry cancellation on reassignment

At retry promotion time (`scheduleRetry` / `scheduleRetryAt`), before re-queuing a scheduled retry, the service checks whether:

- `scheduledRetry.AgentInstanceID` still matches `task.AssigneeAgentInstanceID`, or
- The task is now `CANCELLED`.

If either condition holds, the retry is cancelled instead of promoted. The wakeup is marked `cancelled` with reason `retry_stale_assignee` or `retry_task_cancelled`. Any execution locks held by the old agent are cleared.

This also applies at the route level: when a task's assignee is updated via the API, any pending `scheduled_retry` wakeups for the previous assignee are cancelled immediately, without waiting for the retry promotion path to catch them.

### 3. Unstarted task dispatch (recovery sweep)

The existing scheduler tick is extended with a recovery sweep that runs once per tick after the wakeup drain. The sweep finds assigned `TODO` tasks with no prior queued or running wakeup and dispatches them as `task_assigned` wakeups.

**Selection criteria:**

```sql
SELECT t.id FROM tasks t
WHERE t.state = 'TODO'
  AND t.assignee_agent_instance_id IS NOT NULL
  AND t.archived_at IS NULL
  AND t.created_at >= NOW() - INTERVAL '<lookback_hours> hours'
  AND NOT EXISTS (
      SELECT 1 FROM wakeup_requests w
      WHERE w.payload->>'task_id' = t.id
        AND w.status IN ('queued', 'claimed', 'finished')
  )
```

Guards applied per candidate:
- Skip if the agent is paused or stopped.
- Skip if a wakeup is already queued for this task (prevents duplicates on concurrent ticks).
- Skip if the agent's invocation budget is exhausted.

The sweep logs a `recovery_dispatch` activity entry per dispatched task and a `recovery_sweep_complete` summary entry with `dispatched_count` at the end of each sweep.

### 4. Configurable lookback window

The recovery sweep lookback is configurable per workspace via a workspace settings field:

| Setting | Default | Range |
|---|---|---|
| `recovery_lookback_hours` | `24` | 1–720 |

The value is clamped to the allowed range on write. It controls how far back the sweep looks for unstarted tasks. Setting it lower reduces sweep query cost; setting it higher catches tasks that have been stuck for longer (useful after extended outages).

## UX

### Wakeup log

The wakeups list page gains status badges for the new terminal states:

| Badge | Condition |
|---|---|
| "Cancelled — assignee changed" | `cancel_reason = assignee_changed` |
| "Cancelled — task completed" | `cancel_reason = task_terminal` AND task is `DONE` |
| "Cancelled — task cancelled" | `cancel_reason = task_terminal` AND task is `CANCELLED` |
| "Cancelled — retry stale" | `cancel_reason = retry_stale_assignee` |
| "Recovered" | `reason = task_assigned` AND dispatched by recovery sweep |

Retried wakeups show retry count and next retry time (already stored in `scheduled_retry_at`).

### Activity feed

New activity action codes surfaced in the office activity feed:

- `wakeup_stale_cancelled` — stale check cancelled a queued wakeup (details: reason, task_id, agent).
- `wakeup_retry_cancelled` — retry cancelled due to reassignment (details: task_id, old_agent).
- `recovery_dispatch` — unstarted task dispatched by recovery sweep (details: task_id, agent).

### Settings (optional)

Workspace settings page, advanced section (hidden by default):

- **Recovery lookback window** — numeric input, hours, default 24, range 1–720. Label: "How far back to look for unstarted tasks during recovery sweeps."

## Scenarios

- **GIVEN** a `task_assigned` wakeup is queued for agent A on task T, **WHEN** task T is reassigned to agent B before the wakeup is claimed, **THEN** the wakeup is cancelled with reason `assignee_changed`, agent A is not launched, and a `wakeup_stale_cancelled` activity entry is logged.

- **GIVEN** a wakeup for task T fails and a retry is scheduled 10 minutes out, **WHEN** task T is reassigned to agent B before the retry fires, **THEN** the retry is cancelled at promotion time with reason `retry_stale_assignee`, execution locks are cleared, and a `wakeup_retry_cancelled` activity entry is logged.

- **GIVEN** a task is reassigned via the API, **WHEN** the PATCH handler processes the update, **THEN** any pending `scheduled_retry` wakeups for the previous assignee are cancelled immediately.

- **GIVEN** a task in `TODO` state assigned to agent A has no queued or finished wakeup and was created within the lookback window, **WHEN** the recovery sweep runs, **THEN** a `task_assigned` wakeup is dispatched for agent A and a `recovery_dispatch` activity entry is logged.

- **GIVEN** the same unstarted task already has a wakeup with status `queued`, **WHEN** the recovery sweep runs, **THEN** the task is skipped (no duplicate wakeup).

- **GIVEN** a workspace with `recovery_lookback_hours = 2`, **WHEN** a task was created 6 hours ago with no wakeup, **THEN** the recovery sweep skips it (outside the lookback window).

- **GIVEN** a wakeup for a task that has reached `DONE` state, **WHEN** the staleness check runs at claim time, **THEN** the wakeup is cancelled with reason `task_terminal` and the agent is not launched.

## Out of scope

- Recovery sweeps for non-`TODO` states (e.g. `IN_PROGRESS` tasks with no active session — covered separately by the blocked-task-escalation spec).
- Per-agent or per-project lookback window overrides (workspace-level is sufficient).
- Surfacing retry cancellation reasons in the agent detail page runs tab (existing retry count display is adequate).
- Automatic reassignment to a different agent when a stale wakeup is cancelled (cancellation only — the scheduler does not infer intent).
- Deduplication of recovery dispatches across multiple backend instances (idempotency key on the dispatched wakeup is sufficient).
