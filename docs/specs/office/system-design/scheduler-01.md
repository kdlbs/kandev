---
status: draft
system: office
requirements:
  - REQ-OFFICE-SCHEDULER-001
created: 2026-04-25
owners:
  - cfl
---
# Office Scheduler System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-SCHEDULER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-SCHEDULER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Kandev's base task scheduler is reactive: tasks enter the queue only when a user explicitly starts them or sends a prompt. Office adds autonomous agent operation, which requires the system to wake agents on its own when events happen (assignments, comments, blocker resolutions, approvals), on a schedule (routines), and on heartbeat ticks (periodic coordinator checks). Without an autonomous wakeup pipeline, every interaction needs a human to initiate it, and the cost / reliability story (idle skips, rate-limit retries, staleness, recovery) has nowhere to live.

Office supplies autonomous run producers and Office-specific maintenance. The persisted `runs` queue and its single backend-wide consumer are shared workflow infrastructure, not one scheduler per Office workspace. Shared ownership, scoping, and shutdown contract is defined by [run queue](../../tasks/requirements/run-scheduling.md) and [ADR-2026-08-01-global-run-scheduler-ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md).

## What

### Shared scheduler boundary

- One shared runs scheduler processes persisted work for every workspace.
- Office event subscribers and unstarted-task recovery act only on tasks whose
  project/workflow identity makes `Task.IsFromOffice` true. A runner on an
  ordinary Kanban task is not sufficient.
- Explicit workflow `queue_run` actions remain available to every workflow
  style and are not filtered by Office identity.
- Office recovery is maintenance, not part of every five-second queue drain.
  When no workspace has adopted Office, it skips the task scan.
- The shared runs scheduler and cron loop stop and join before database cleanup
  during graceful shutdown.

### Wakeup queue

A SQLite-persisted queue of "wake this agent up" requests. Every periodic, event-driven, and reactive trigger flows through this queue before becoming an agent run. Each request:

- Has a `source` discriminator (see table below) plus a typed payload.
- Carries an `idempotency_key` for source-level dedup. The queue checks recent rows within 24 hours and the persisted key remains unique for its lifetime.
- Is coalesced into an in-flight run when one exists for the same agent (claim-time merge).
- Produces exactly one `runs` row on successful claim; the run is the execution record, the wakeup-request is the dispatch record.

### Wakeup sources

| Source | Trigger | Payload |
|--------|---------|---------|
| `routine` | A routine's cron / webhook / manual trigger fires | `{routine_id, variables, missed_ticks?}` |
| `comment` | Comment posted on a task assigned to this agent (non-self). Also the channel pathway: inbound Telegram/Slack messages become comments on a channel task. | `{task_id, comment_id}` |
| `agent_error` | A sub-agent's session failed (escalation to coordinator) | `{agent_profile_id, run_id, error}` |
| `self` | Agent self-wake via tool call | `{reason, payload?}` |
| `user` | User mention / explicit wake from the UI | `{user_id, context?}` |
| `task_assigned` | An authoritative Office task's `assignee_agent_instance_id` is set or changed, including a newly-created task/subtask that already has a runner | `{task_id}` |
| `task_blockers_resolved` | All blocking tasks of an assigned task reach `done` | `{task_id, resolved_blocker_ids}` |
| `task_children_completed` | All child tasks of an assigned task reach terminal state | `{task_id}` |
| `approval_resolved` | An approval requested by this agent is approved/rejected | `{approval_id, status, decision_note}` |
| `budget_alert` | Budget threshold crossed for a sub-agent (coordinator only) | `{agent_instance_id, budget_pct}` |

The task-event sources are dispatched by office event subscribers when the corresponding task event fires. The legacy `heartbeat` source is **retired**: all periodic wakes flow through the `routine` source - each coordinator agent gets a pre-installed routine at onboarding (see *Routines*), and that routine's cron tick is what wakes the coordinator.

### Manual status changes (kanban drag-drop)

Office tasks live on the system office workflow and appear on the kanban board. Users can drag them between columns (steps), which emits a `task.moved` event. The office event subscribers handle `task.moved` events for office tasks (identified by `assignee_agent_instance_id != null`) and fire the appropriate wakeups:

- Move to "In Progress": `task_assigned` wakeup for the assignee agent.
- Move to "Done" or "Cancelled": resolve blocker dependencies, fire `task_blockers_resolved` / `task_children_completed` wakeups for affected agents.
- Move to "In Review": if execution_policy has reviewers, wake reviewer agents.
- Move from "In Review" back to "In Progress": `comment` wakeup with rejection context for the assignee.

Non-office tasks (those without `assignee_agent_instance_id`) are ignored by office subscribers.

### Coalescing - three layers, one job each

Coalescing happens entirely at the wakeup-request layer; the runs table is the execution record.

1. **Source-level dedup via `idempotency_key`** (UNIQUE column). Format:
   - `routine:<routine_id>:<trigger_id>:<unix_minute>` for cron routines.
   - `comment:<comment_id>` for comments.
   - `task_assigned:<task_id>:<agent_instance_id>` for task creation/assignment events.
   Duplicate inserts in the same window are rejected silently. Handles webhook re-delivery, event-bus replay, and restart recovery.

2. **Claim-time merge.** The source first persists the wakeup-request with `status="queued"`. The dispatcher then looks for an in-flight run for the same agent (`queued` or `claimed`). If one exists **and it is still `queued`**: mark the persisted request with `status="coalesced"` and `run_id=<existing>`, merge its payload into the existing run's `context_snapshot`, increment the run's `coalesced_count`, and — if the existing run is periodic-classified (see "Idle wakeup skip" below) while the new request is event-classified — promote the run's `reason` to the new request's classification (monotonic: periodic -> event only, never the reverse). The promotion, request transition, count update, and payload merge use one transaction, so the scheduler cannot claim a partially updated run. The agent sees the merged context and the promoted reason when it actually runs. If the in-flight run is already **`claimed`** *and the promotion above would otherwise be required* (the run is periodic-classified and the new request is event-classified): it is not promoted or merged into — the scheduler has already read its `reason` into memory for the idle-skip decision and never re-reads the row, so mutating it afterward would race a decision already in flight. The new request instead creates its own fresh `runs` row and is marked `claimed` against it, exactly as if no in-flight run existed. A `claimed` run that needs no promotion (the existing run is already event-classified, or the new request is itself periodic-classified) is coalesced into exactly as before. If no in-flight run exists at all: create the corresponding `runs` row and mark the persisted request as `claimed` against it.

3. **`runs.idempotency_key`** is kept as a defensive secondary key. Rarely tripped now (the wakeup-request layer handles the common case), but useful for the rare "two cron processes fired the same tick from different leaders" scenario during a leadership change.

A coalescing window (default 5 seconds) also merges wakeups for the same agent and same source if no run is yet in flight. Example: 5 subtasks complete within 3 seconds generating 5 `task_children_completed` wakeups, coalesced into 1 with `coalesced_count=5`.

### Routines

A routine is a task template (or taskless wake spec) with one or more triggers. Routines are the **only** mechanism for periodic wakes; there is no system-level agent heartbeat cron.

Routine fields:
- `id`, `workspace_id`, `name`, `description`.
- `assignee_agent_instance_id` - who gets the resulting task / run.
- `status`: `active` | `paused` | `archived`.
- `concurrency_policy`: `coalesce_if_active` (default) | `skip_if_active` | `always_enqueue`.
- `catch_up_policy`: `enqueue_missed_with_cap` (default, cap 25) | `skip_missed`.
- `catch_up_max`: integer, default 25.
- `task_template`: JSON. Empty means **lightweight** routine (taskless run per fire). Non-empty means **heavy** routine (fresh task created on the `routine` workflow).
- `variables`: declared template variables (type, default, required).

Triggers:
- **Schedule (cron)**: `cron_expression`, `timezone`, computed `next_run_at`, `last_fired_at`.
- **Webhook**: `public_id` (URL path component), `signing_mode` (`none` | `bearer` | `hmac_sha256`), `secret`. URL: `POST /api/routine-triggers/<public_id>/fire`. Webhook payload is available as variables.
- **Manual**: fired only via UI or API.

Variables use `{{name}}` syntax in title/description with types `text`, `number`, `boolean`, `select`. Built-ins: `{{date}}`, `{{datetime}}`. Resolution order (later wins): built-ins -> declared defaults -> provided values (manual UI or webhook payload). Adding `{{new_var}}` to a template auto-creates the variable declaration on save (`text`, no default, not required).

#### Routine runs

Each trigger firing creates a routine run record (`office_routine_runs`) with `routine_id`, `trigger_id`, `source` (`cron` | `webhook` | `manual`), `status` (`received` -> `task_created` | `skipped` | `coalesced` | `failed`), `trigger_payload` (resolved variable values), `linked_task_id` (heavy only), `coalesced_into_run_id`, `dispatch_fingerprint` (hash of resolved template + assignee), and lifecycle timestamps.

#### Heavy vs lightweight routines

- **Lightweight** (`task_template` empty): fire produces a taskless agent run. Continuation summary keyed by `routine:<routine_id>`. Use case: "check upstream PRs" without a trackable artifact.
- **Heavy** (`task_template` set): fire creates a task in the system `routine` workflow (one hidden `in_progress -> done` step). Its `task.created` event evaluates `auto_start_agent` and starts a task-bound run. Use case: "daily review" with trackable output.

#### Concurrency policy

Evaluated at dispatch by querying for an in-flight run for the same routine fingerprint:
- `skip_if_active`: do not create a new task / run. Mark `skipped`.
- `coalesce_if_active` (default): merge into the existing run. Mark `coalesced`.
- `always_enqueue` / `always_create`: always proceed.

"Active" means the linked task / run is not in a terminal state.

#### Catch-up policy

If the scheduler was down and missed cron ticks:
- `skip_missed`: fire only the current tick.
- `enqueue_missed_with_cap` (default, cap 25): fire missed ticks up to the cap; dropped ticks are not recorded individually but summarized into the next prompt's wake context ("you missed N ticks since X").

#### The pre-installed coordinator routine

At agent-create time, when an agent's role is coordinator/CEO, the system creates one routine:

```
name:                "Coordinator heartbeat"
description:         "Wakes the coordinator every 5 minutes to check workspace activity, react to errors and budget signals, and decide what to do next."
assignee_agent_id:   <new coordinator agent id>
status:              active
concurrency_policy:  coalesce_if_active
catch_up_policy:     enqueue_missed_with_cap
catch_up_max:        25
task_template:       ""
variables:           []
trigger:
  kind:              schedule
  cron_expression:   "*/5 * * * *"
  timezone:          (workspace TZ, fall back to UTC)
  enabled:           true
```

This is a regular routine - no system flag, no lock, no badge. The user can edit / pause / delete it. If deleted, the coordinator only wakes via reactive sources. Default cadence is **every five minutes** (the prior 60s default was too aggressive); users can crank it up.

### Idle wakeup skip

Before processing a routine-fired heartbeat-style wakeup (lightweight routine, no task payload), the scheduler checks whether the agent has any actionable tasks. If none, the wakeup is skipped, no session is launched.

**Actionable states**: `TODO` and `IN_PROGRESS`. Terminal (`DONE`, `CANCELLED`, `ARCHIVED`) and review-gated (`IN_REVIEW`) do not count.

**Skip conditions (all must hold)**:
- Wakeup is a periodic / heartbeat-style routine fire (lightweight, no task in payload).
- Agent has `skip_idle_wakeups = true`.
- `CountActionableTasksForAgent` returns 0.

**Per-agent `skip_idle_wakeups` defaults**:

| Role | Default |
|------|---------|
| `worker` | `true` |
| `specialist` | `true` |
| `assistant` | `true` |
| `ceo` / coordinator | `false` |

Coordinator agents default to `false` because their heartbeat purpose is self-directed coordination (surveying projects, reassigning tasks, checking budgets) which does not require a directly assigned task. Users can override per agent.

**Event-triggered wakeups always proceed** - the skip applies only to periodic wakes:

| Source | Skippable? |
|--------|-----------|
| `routine`, cron-triggered (lightweight, heartbeat-style) | Yes |
| `routine`, manual "Fire now" or webhook-triggered | No |
| `task_assigned`, `comment`, `task_blockers_resolved`, `task_children_completed`, `approval_resolved`, `agent_error`, `budget_alert`, `self`, `user` | No |

Skipped wakeups are not silently discarded:
1. Logged at `INFO` with `wakeup_id`, `agent_instance_id`, `agent_name`, `reason="no_actionable_tasks"`.
2. Marked `finished` (normal terminal state).
3. Recorded as a `wakeup_idle_skipped` activity entry.

Skip check uses a single indexed count:

```sql
SELECT COUNT(*) FROM tasks
WHERE assignee_agent_instance_id = $agentID
  AND state IN ('TODO', 'IN_PROGRESS')
  AND archived_at IS NULL
```

### Executor resolution at launch

When the scheduler claims a run, it resolves the executor using the agent preference first. If the agent has no executor preference and the run payload carries `task_id`, the scheduler resolves the task's project and allows that project executor config to satisfy the launch. This prevents assigned task runs from retrying indefinitely solely because the worker's agent row has an empty executor preference.

### Staleness check (before claim)

Before `processWakeup` proceeds past the agent status guard, the scheduler checks whether the wakeup's context is still valid. This runs on every wakeup that carries a `task_id` in its payload.

**Staleness conditions** (each produces a distinct cancel reason):

| Condition | Cancel reason |
|---|---|
| Task not found | `task_not_found` |
| Task assignee changed (`task.AssigneeAgentInstanceID != wakeup.AgentInstanceID`) | `assignee_changed` |
| Task reached terminal state (`DONE`, `CANCELLED`, `ARCHIVED`) | `task_terminal` |
| Task's review-stage participant changed | `review_participant_changed` |

A stale wakeup is cancelled (status `cancelled`), not retried. Cancellation is idempotent and logged. Any held checkout lock is released.

### Retry cancellation on reassignment

At retry promotion time (`scheduleRetry` / `scheduleRetryAt`), before re-queuing a scheduled retry, the service checks whether:
- `scheduledRetry.AgentInstanceID` still matches `task.AssigneeAgentInstanceID`, or
- The task is now `CANCELLED`.

If either holds, the retry is cancelled with reason `retry_stale_assignee` or `retry_task_cancelled`. Execution locks held by the old agent are cleared.

Additionally, when a task's assignee is updated via the API, any pending `scheduled_retry` wakeups for the previous assignee are cancelled immediately, without waiting for the retry-promotion path.

### Rate-limit retry with parsed reset time

When `HandleWakeupFailure` is called, if the error is a rate-limit error the service tries to extract a reset timestamp from the message text. If one is found, the wakeup is scheduled for `parsed_reset_time + 30s` regardless of the `RetryCount` position in the backoff table. `RetryCount` is still incremented so `MaxRetryCount` escalation still applies.

**Rate-limit detection** - any of (case-insensitive where stated):
- `"rate limit"`, `"rate_limit"`, `"429"`, `"too many requests"`, `"quota exceeded"`.

**Reset-time patterns**:
- `"resets at HH:MM AM/PM"` - absolute wall-clock time on the current day (next occurrence if past).
- `"Retry-After: N"` - N seconds from now.
- `"try again in X minutes"` / `"try again in X seconds"` - relative duration.
- `"rate limit exceeded ... reset_time: <unix timestamp>"` - Unix epoch seconds.

A 30-second buffer is added to any parsed time. If no pattern matches or the parsed time is in the past after buffer, the existing exponential backoff applies unchanged.

Log fields gain `source: "rate_limit_parsed"` vs `source: "backoff"`, plus `parsed_reset_at` (UTC).

**Default backoff for non-rate-limit retries**: 4 attempts at `[2m, 10m, 30m, 2h]` with 25% jitter. After `MaxRetryCount` (4) failures, `escalateFailure` is called - the wakeup is marked `failed`, an `agent.error` inbox item is created, and the coordinator receives an `agent_error` wakeup. When the failing run's agent IS the coordinator itself, the `agent_error` wakeup is not escalated to it (no self-targeted run is queued) - the `agent.error` inbox item is still created.

### Recovery sweep (unstarted Office tasks)

Office maintenance performs a recovery sweep separately from the shared queue-drain tick. It finds authoritative Office `TODO` tasks created inside the workspace recovery lookback window and dispatches them as `task_assigned` runs only when no queued, claimed, or finished run exists for the task. The task-creation timestamp is bounded by the lookback; matching run rows are not, so a task that already started is never reclassified as unstarted merely because its prior run is old. Failed and cancelled rows do not block recovery. Assignment on an ordinary Kanban task does not imply autonomy.

Selection:

```sql
SELECT t.id FROM tasks t
WHERE t.state = 'TODO'
  AND t.assignee_agent_instance_id IS NOT NULL
  AND (
    COALESCE(t.project_id, '') != ''
    OR t.workflow_id = (
      SELECT w.office_workflow_id FROM workspaces w WHERE w.id = t.workspace_id
    )
  )
  AND t.archived_at IS NULL
  AND t.created_at >= NOW() - INTERVAL '<lookback_hours> hours'
  AND NOT EXISTS (
      SELECT 1 FROM runs r
      WHERE r.payload->>'task_id' = t.id
        AND r.status IN ('queued', 'claimed', 'finished')
  )
```

Per-candidate guards:
- Skip if agent is paused or stopped.
- Skip if a wakeup is already queued for this task (prevents duplicates on concurrent ticks).
- Skip if the agent's invocation budget is exhausted.

Logged: `recovery_dispatch` per dispatched task, `recovery_sweep_complete` summary entry with `dispatched_count` per sweep.

Lookback is a workspace setting `recovery_lookback_hours`, default 24, range 1-720, clamped on write.

### Scheduler processing pipeline

```
processWakeup:
  1. Claim                    -- atomic UPDATE: status='queued' -> 'claimed', claimed_at=now()
  2. Guard: agent status      -- paused/stopped -> mark finished, no action
  3. Staleness check          -- task scope still valid (assignee, terminal, review participant)
  4. Idle skip                -- routine/heartbeat-style + skip_idle_wakeups=true + 0 actionable tasks -> finished
  5. Checkout                 -- atomic CAS lock on the task (only when payload has task_id)
  6. Budget pre-check         -- workspace -> project -> agent budgets; pause_agent action skips
  7. Build context            -- assemble prompt from source, payload, context_snapshot
  8. Resolve executor         -- task override -> agent preference -> project -> workspace default
  9. Resolve execution agent  -- Office binding -> concrete profile or shared dynamic router
  10. Create session          -- one logical TaskSession through the orchestrator pipeline
  11. Launch                  -- lifecycle -> compute executor -> dynamic conductor or concrete agentctl
  12. Finish                  -- mark wakeup finished; parse output for follow-up actions
```

### Atomic task checkout

When an agent starts working on a task, it acquires an exclusive lock via CAS:

```sql
UPDATE tasks SET checkout_agent_id = $agent, checkout_at = now()
WHERE id = $task AND checkout_agent_id IS NULL
RETURNING *
```

Zero rows = another agent already holds the lock -> wakeup skipped, no retry. Released on finish or failure. Prevents two agents racing on the same task when concurrency > 1 or multiple agents are assigned.

### Agent concurrency

Each agent instance has `max_concurrent_sessions` (default 1) and `cooldown_sec` (default 10s). The claim query skips agents at capacity and skips agents whose `last_wakeup_finished_at` is within the cooldown window. Wakeups for busy or cooling-down agents stay `queued` and are picked up naturally when eligible. No re-queuing, no retry limits, no expiry: a slow QA agent with 20 tasks queued processes them sequentially. Concurrency > 1 is useful for agents handling independent tasks (e.g. multiple code reviews in parallel).

### Claim query

```sql
SELECT w.* FROM office_wakeup_queue w
JOIN office_agent_instances a ON a.id = w.agent_instance_id
WHERE w.status = 'queued'
  AND a.status IN ('idle', 'working')
  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= now())
  AND (a.last_wakeup_finished_at IS NULL OR a.last_wakeup_finished_at <= now() - a.cooldown_sec)
  AND (
    SELECT COUNT(*) FROM task_sessions ts
    WHERE ts.agent_instance_id = w.agent_instance_id
      AND ts.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
  ) < a.max_concurrent_sessions
ORDER BY w.requested_at
LIMIT 1
```

### One-shot session model + continuation summary

Each wakeup produces a single agent session that runs to completion and exits. The agent receives a structured prompt describing why it was woken.

**Taskless runs always start a fresh session.** A defensive `taskID==""` short-circuit in `HasPriorSessionForAgent` ensures we never resume across taskless fires.

**Task-bound wakeups use session resume by default**: each subsequent wakeup for a `(task, agent)` pair reloads the prior ACP session via `session/load`, falling back to `session/new` on error. See `office-task-session-lifecycle` for the per-pair model.

#### Continuation summary

To bridge context across taskless fires (heartbeats, lightweight routines) without unbounded conversation growth, the system maintains a per-(agent, scope) markdown summary.

**Table `agent_continuation_summaries`**:

```
agent_profile_id  TEXT  NOT NULL
scope             TEXT  NOT NULL  -- "routine:<routine_id>" (per-routine summary chain)
content           TEXT  NOT NULL  DEFAULT ''  -- markdown body, capped at 8 KB
content_tokens    INT   NOT NULL  DEFAULT 0
updated_at        TIMESTAMP NOT NULL
updated_by_run_id TEXT NOT NULL DEFAULT ''
PRIMARY KEY (agent_profile_id, scope)
```

Writes are upsert (one current row per scope, no history). The prompt slice is truncated to 1,500 chars; full content cap is 8 KB.

**Summary structure** (markdown sections):

```markdown
## Active focus
2-3 lines. What the coordinator is currently watching/driving.

## Open blockers
Bullet list. Each: blocker + what's needed to unblock + when surfaced.

## Recent decisions
Bullet list. Last ~5 things the coordinator committed to. Date-stamped.

## Next action
One sentence. The single next thing to do on the next wake-up.
```

**Generation is server-synthesized, not agent-written.** A builder (`internal/office/summary/builder.go`) composes the markdown deterministically from structured inputs after each successful run:

| Input | Source | Used for |
|---|---|---|
| `run.result_json` | Adapter-populated structured output; fallback chain `result_json.summary -> .result -> .message -> .error` | Recent actions / decisions |
| Workspace activity stats | `office_activity_log` + `runs` (counts of completed/failed tasks, agent-error escalations, budget signals) | Active focus, opening blocker context |
| Active blockers | Tasks in `BLOCKED` state assigned to managed agents | Open blockers section |
| Previous summary body | Prior row in `agent_continuation_summaries` | Continuity / fallback for unchanged sections |
| Inferred next action | Decision table on `(workspace state, last run status)`; falls back to "Continue monitoring." | Next action |

Idempotent: re-running with the same inputs produces the same output. Called from the `AgentCompleted` event subscriber on successful taskless runs. On failure, the previous summary is left intact (last-good wins).

### Resume delta prompt

When resuming a task-bound session (same agent, same task, session ID preserved), the agent receives only a resume delta - the new information since the last run. Full instructions and context are skipped (the agent CLI retains them from the previous session), saving ~5-10K tokens per fire.

### Subtask sequencing via blockers

Office does not have a separate workflow/template engine for subtask ordering. The agent's instructions (via skills) define how to decompose work and which subtasks to create. Sequencing is enforced through the existing blocker system: the agent creates subtask 2 with `blocked_by: [subtask 1]`.

The scheduler respects blockers: a `task_assigned` wakeup for a blocked task is held until blockers resolve. When a subtask completes:
1. If `requires_approval=true`: task moves to `in_review`, inbox item created. On user approval, task moves to `done`.
2. If `requires_approval=false`: task moves directly to `done`.
3. On reaching `done`: any sibling tasks that had this task as a blocker receive a `task_blockers_resolved` wakeup for their assigned agent.

This creates a natural pipeline: Spec (requires_approval) -> Build (blocked_by Spec) -> Review (blocked_by Build) -> Ship (blocked_by Review). The user only intervenes at approval gates.

The coordinator is woken via `task_children_completed` when all subtasks under a parent reach terminal state.

### Pre-execution budget check

Before launching an agent session, the scheduler checks all applicable budget policies (workspace -> project -> agent). If any budget is exceeded with `action_on_exceed=pause_agent`, the agent is paused and the wakeup is skipped. Prevents wasting tokens on a run that would immediately be followed by a budget-exceeded pause.

### Integration with existing scheduler

User-initiated task execution continues through the orchestrator directly. Engine-emitted work uses the single shared `runs` scheduler across all workspaces. Office contributes event-driven producers and maintenance handlers to that shared path; it does not own a per-workspace scheduler. Both paths converge at the agent runtime.
