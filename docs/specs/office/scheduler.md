---
status: draft
created: 2026-04-25
owner: cfl
needs-upgrade: [permissions]
---

# Office Scheduler

## Why

Kandev's base task scheduler is reactive: tasks enter the queue only when a user explicitly starts them or sends a prompt. Office adds autonomous agent operation, which requires the system to wake agents on its own when events happen (assignments, comments, blocker resolutions, approvals), on a schedule (routines), and on heartbeat ticks (periodic coordinator checks). Without an autonomous wakeup pipeline, every interaction needs a human to initiate it, and the cost / reliability story (idle skips, rate-limit retries, staleness, recovery) has nowhere to live.

The office scheduler is the single seam for all autonomous wakeups: a SQLite-persisted wakeup queue with coalescing and idempotency, a claim/dispatch loop that creates one-shot agent runs, a routines table that drives all periodic and webhook-triggered wakes, plus the reliability primitives (rate-limit retry, idle skip, staleness check, recovery sweep) that keep the system honest at long uptime.

## What

### Wakeup queue

A SQLite-persisted queue of "wake this agent up" requests. Every periodic, event-driven, and reactive trigger flows through this queue before becoming an agent run. Each request:

- Has a `source` discriminator (see table below) plus a typed payload.
- Carries an `idempotency_key` for source-level dedup within a 24-hour window.
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
| `task_assigned` | Task's `assignee_agent_instance_id` is set or changed | `{task_id}` |
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
   - `task_assigned:<task_id>:<timestamp>` for assignment events.
   Duplicate inserts in the same window are rejected silently. Handles webhook re-delivery, event-bus replay, and restart recovery.

2. **Claim-time merge.** When the dispatcher processes a wakeup-request, it looks for an in-flight run for the same agent (`queued` -> `scheduled-retry` -> `running`, in that order). If one exists: insert the new request with `status="coalesced"`, `run_id=<existing>`, merge the new request's payload into the existing run's `context_snapshot`, and increment `coalesced_count` on the in-flight wakeup-request. The agent sees the merged context when it actually runs. If none exists: insert the wakeup-request with `status="queued"` and create the corresponding `runs` row.

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
- **Heavy** (`task_template` set): fire creates a fresh task in the `routine` workflow (a single auto-completing `in_progress -> done` step, system-flagged via `SystemWorkflowTemplateIDs` so heavy routine tasks inherit the hide-by-default UX), then a normal task-bound run. Use case: "daily review" where output should be a trackable item.

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
| `routine` (lightweight, heartbeat-style) | Yes |
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

**Default backoff for non-rate-limit retries**: 4 attempts at `[2m, 10m, 30m, 2h]` with 25% jitter. After `MaxRetryCount` (4) failures, `escalateFailure` is called - the wakeup is marked `failed`, an `agent.error` inbox item is created, and the coordinator receives an `agent_error` wakeup.

### Recovery sweep (unstarted tasks)

The scheduler tick is extended with a recovery sweep that runs once per tick after the wakeup drain. It finds assigned `TODO` tasks with no prior queued or running wakeup and dispatches them as `task_assigned` wakeups.

Selection:

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
  9. Create session           -- TaskSession through the orchestrator pipeline; per-task-and-agent session
  10. Launch                  -- lifecycle manager -> executor backend -> agentctl -> agent subprocess
  11. Finish                  -- mark wakeup finished; parse output for follow-up actions
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

The base `scheduler.Scheduler` and `queue.TaskQueue` continue to handle user-initiated task execution unchanged. The wakeup queue is a parallel path: same scheduler tick loop, different queue, different processing logic. Both paths converge at the lifecycle manager - the same `LaunchAgent()` / `StartAgentProcess()` calls regardless of whether the session was user-initiated or wakeup-initiated.

## Data model

### `agent_wakeup_requests`

```
id                       TEXT PRIMARY KEY
agent_profile_id         TEXT NOT NULL
source                   TEXT NOT NULL  -- routine | comment | agent_error | self | user
                                        --   plus task-event sources: task_assigned, task_blockers_resolved,
                                        --   task_children_completed, approval_resolved, budget_alert
reason                   TEXT NOT NULL  -- short label for telemetry
payload                  TEXT NOT NULL DEFAULT '{}'  -- JSON, typed at boundary (see Payload schema)
status                   TEXT NOT NULL  -- queued | claimed | coalesced | skipped | finished | failed | cancelled
cancel_reason            TEXT           -- task_not_found | assignee_changed | task_terminal | review_participant_changed
                                        --   | retry_stale_assignee | retry_task_cancelled
coalesced_count          INT  NOT NULL DEFAULT 1
idempotency_key          TEXT UNIQUE
retry_count              INT  NOT NULL DEFAULT 0
scheduled_retry_at       TIMESTAMP
context_snapshot         TEXT NOT NULL DEFAULT '{}'  -- pre-computed prompt context (task summary, new comments)
run_id                   TEXT  -- the run this request fulfilled (when status terminal)
requested_at             TIMESTAMP NOT NULL
claimed_at               TIMESTAMP
finished_at              TIMESTAMP
INDEX (agent_profile_id, status)
INDEX (idempotency_key)
```

### `agent_continuation_summaries`

```
agent_profile_id   TEXT NOT NULL
scope              TEXT NOT NULL  -- routine:<routine_id>
content            TEXT NOT NULL DEFAULT ''  -- markdown, capped 8 KB
content_tokens     INT  NOT NULL DEFAULT 0
updated_at         TIMESTAMP NOT NULL
updated_by_run_id  TEXT NOT NULL DEFAULT ''
PRIMARY KEY (agent_profile_id, scope)
```

### `office_routines`

```
id                    TEXT PRIMARY KEY
workspace_id          TEXT NOT NULL
name                  TEXT NOT NULL
description           TEXT
assignee_agent_id     TEXT NOT NULL
status                TEXT NOT NULL  -- active | paused | archived
concurrency_policy    TEXT NOT NULL  -- coalesce_if_active | skip_if_active | always_enqueue
catch_up_policy       TEXT NOT NULL  -- enqueue_missed_with_cap | skip_missed
catch_up_max          INT  NOT NULL DEFAULT 25
task_template         TEXT NOT NULL DEFAULT ''  -- JSON; empty -> lightweight
variables             TEXT NOT NULL DEFAULT '[]'  -- JSON array
priority              INT  NOT NULL DEFAULT 0
created_at, updated_at
```

### `office_routine_triggers`

```
id                TEXT PRIMARY KEY
routine_id        TEXT NOT NULL
kind              TEXT NOT NULL  -- schedule | webhook | manual
cron_expression   TEXT           -- when kind=schedule
timezone          TEXT
next_run_at       TIMESTAMP      -- computed; atomically claimed for cron scheduling
last_fired_at     TIMESTAMP
public_id         TEXT UNIQUE    -- when kind=webhook
signing_mode      TEXT           -- none | bearer | hmac_sha256
secret            TEXT
enabled           BOOL NOT NULL DEFAULT TRUE
```

### `office_routine_runs`

```
id                       TEXT PRIMARY KEY
routine_id               TEXT NOT NULL
trigger_id               TEXT NOT NULL
source                   TEXT NOT NULL  -- cron | webhook | manual
status                   TEXT NOT NULL  -- received | task_created | skipped | coalesced | failed
trigger_payload          TEXT NOT NULL DEFAULT '{}'
linked_task_id           TEXT
coalesced_into_run_id    TEXT
dispatch_fingerprint     TEXT NOT NULL
started_at, completed_at
```

### `runs` (additions)

```
result_json       TEXT  NOT NULL  DEFAULT '{}'  -- structured adapter output for summary builder
assembled_prompt  TEXT  NOT NULL  DEFAULT ''    -- final prompt as the agent saw it (inspection)
summary_injected  TEXT  NOT NULL  DEFAULT ''    -- the summary that was prepended (per-run snapshot)
```

`runs.payload.task_id` is empty for taskless runs; all other run lifecycle fields (`idempotency_key`, claim/coalesce, cost rollup) carry over unchanged.

### Workspace settings (additions)

```
recovery_lookback_hours   INT  NOT NULL DEFAULT 24   -- range 1-720, clamped on write
```

### Agent instance / profile (additions)

```
skip_idle_wakeups       BOOL  NOT NULL  -- default true for worker/specialist/assistant, false for ceo
max_concurrent_sessions INT   NOT NULL DEFAULT 1
cooldown_sec            INT   NOT NULL DEFAULT 10
last_wakeup_finished_at TIMESTAMP
```

### Payload schema

`agent_wakeup_requests.payload` is `TEXT DEFAULT '{}'`. The DB column is free-form so adding a new source needs no migration. Type safety lives in code (`internal/office/wakeup/payloads.go`): one Go struct per `source`, unmarshaled at the dispatcher boundary.

```go
type RoutinePayload struct {
    RoutineID   string         `json:"routine_id"`
    Variables   map[string]any `json:"variables,omitempty"`
    MissedTicks int            `json:"missed_ticks,omitempty"` // when catch-up cap collapsed N fires
}
type CommentPayload struct {
    TaskID    string `json:"task_id"`
    CommentID string `json:"comment_id"`
}
type AgentErrorPayload struct {
    AgentProfileID string `json:"agent_profile_id"`
    RunID          string `json:"run_id"`
    Error          string `json:"error"`
}
type TaskAssignedPayload struct { TaskID string `json:"task_id"` }
// ... one per source. Switched on agent_wakeup_requests.source.
```

## API surface

### Routines

- `GET /api/office/routines?workspace_id=...` - list.
- `POST /api/office/routines` - create.
- `GET /api/office/routines/:id` - detail (includes triggers, recent runs).
- `PATCH /api/office/routines/:id` - update name / description / status / concurrency / catch-up / variables / assignee.
- `DELETE /api/office/routines/:id` - delete.
- `POST /api/office/routines/:id/fire` - manual fire (with variable payload).
- `POST /api/routine-triggers/:public_id/fire` - webhook fire (signed per `signing_mode`).

### Wakeup queue (internal)

- `office.wakeup.Service.Enqueue(req)` - source-keyed insert (returns the existing wakeup if `idempotency_key` collides).
- `office.wakeup.Service.HandleWakeupFailure(wakeupID, err)` - rate-limit parsing + scheduleRetry / escalateFailure.
- `office.wakeup.Service.CancelStale(wakeupID, reason)` - mark `cancelled`, release locks, log activity.
- `office.wakeup.Dispatcher.processWakeup(wakeupID)` - the pipeline described above.

### Activity codes (surfaced in office activity feed)

- `wakeup_idle_skipped` - heartbeat-style skip on no actionable tasks.
- `wakeup_budget_blocked` - pre-flight budget pause.
- `wakeup_stale_cancelled` - stale check cancelled a queued wakeup (`reason`, `task_id`, `agent`).
- `wakeup_retry_cancelled` - retry cancelled due to reassignment (`task_id`, `old_agent`).
- `recovery_dispatch` - unstarted task dispatched by recovery sweep (`task_id`, `agent`).
- `recovery_sweep_complete` - summary entry per sweep (`dispatched_count`).

### Run inspection (existing `/office/agents/[id]/runs/[runId]`)

`GetRunDetail` returns the `RunDetail` aggregate including the new columns (`result_json`, `assembled_prompt`, `summary_injected`) plus existing `context_snapshot`, `output_summary`. The detail UI surfaces a "Prompt" tab rendering the assembled prompt + injected summary. WS live updates via `useRunLiveSync` are unchanged.

### Frontend UI

- `/office/routines` (list): each row shows name, trigger type, cron expression, next-fire-at, status, last-run, assignee. Enabled toggle flips status between `active` and `paused`. "Create Routine" opens a dialog (name, description, task template fields, trigger configuration, concurrency policy, catch-up policy + max, variable declarations).
- `/office/routines/[id]` (detail/edit): same field surface plus status radio, last-fired indicator, "Run now" button, and a run history table (status, trigger payload, linked task link).
- Coordinator-empty-state: banner on agent detail page when role is coordinator and no enabled routines target the agent - "This coordinator has no scheduled wake-ups. It will only fire on comments, errors, or manual triggers." Linkable to routines page.
- Wakeup list page gains status badges for new terminal states: "Cancelled - assignee changed", "Cancelled - task completed", "Cancelled - task cancelled", "Cancelled - retry stale", "Recovered".
- Workspace settings advanced section: "Recovery lookback window" numeric input (hours, default 24, range 1-720).

## State machine

### Wakeup request

```
queued -> claimed -> finished       (normal completion)
queued -> claimed -> failed         (after MaxRetryCount retries)
queued -> claimed -> failed (retrying) -> scheduled-retry -> claimed -> ... -> finished | failed
queued -> coalesced                 (merged into in-flight at claim time)
queued -> skipped                   (concurrency_policy=skip_if_active or coalesce drop)
queued -> cancelled                 (staleness check, retry-stale at promotion, API-driven reassign)
claimed -> finished (idle skip)     (no actionable tasks; informational only)
```

Transition triggers:
- `queued -> claimed`: atomic UPDATE in claim query (one process wins).
- `claimed -> finished`: dispatch pipeline succeeds OR idle skip / paused agent guard.
- `claimed -> failed (retrying)`: `HandleWakeupFailure` schedules retry (backoff or parsed rate-limit reset).
- `claimed -> cancelled`: staleness check fails.
- `queued -> cancelled`: API reassign cancels prior-assignee retries; retry promotion finds task no longer owned.
- `queued -> coalesced` / `skipped`: dispatcher applies concurrency policy.

### Routine

```
active <-> paused
active -> archived (terminal)
```

Triggered manually via UI / API.

### Routine run

```
received -> task_created    (heavy routine: real task created)
received -> skipped         (concurrency_policy=skip_if_active and active run exists)
received -> coalesced       (concurrency_policy=coalesce_if_active and active run exists)
received -> failed          (dispatch error)
```

## Failure modes

| Dependency / invariant | Behavior |
|---|---|
| SQLite write failure during enqueue | Source caller surfaces error; idempotency-key collision is silent (treated as already-enqueued). |
| Claim query returns 0 rows | Another process won the claim or no eligible wakeup; tick exits cleanly. |
| Agent paused / stopped at claim | Wakeup marked `finished` with no action; not retried. |
| Task referenced by payload is missing | Staleness check cancels with `task_not_found`; activity logged. |
| Task assignee changed before claim | Cancelled with `assignee_changed`. |
| Task reached terminal state before claim | Cancelled with `task_terminal`. |
| Agent session fails (crash, timeout, unrecoverable) | `HandleWakeupFailure` -> parse rate-limit reset OR exponential backoff (4 attempts at 2m / 10m / 30m / 2h with 25% jitter). After cap: mark `failed`, create `agent.error` inbox item, fire `agent_error` wakeup for coordinator. |
| Rate-limit error with parseable reset | Wakeup scheduled for `reset + 30s`; backoff skipped; `RetryCount` still incremented. |
| Rate-limit error without parseable reset | Falls through to exponential backoff. |
| Retry promoted but task reassigned | Cancelled with `retry_stale_assignee`; execution locks cleared. |
| Retry promoted but task cancelled | Cancelled with `retry_task_cancelled`. |
| Budget pre-check fails | Agent paused per policy; wakeup skipped (not retried); `wakeup_budget_blocked` activity entry. |
| Atomic checkout finds task locked by another agent | Wakeup skipped, no retry. |
| Summary builder fails on successful taskless run | Previous summary left intact; failure logged; next successful run rebuilds. |
| Coordinator's pre-installed routine fails to install at onboarding | Logged + warned; coordinator's agent detail UI shows "no scheduled wake-ups" empty state. User can install one manually. |
| Routine cron tick missed (scheduler down) | `enqueue_missed_with_cap` (default): fire missed ticks up to cap=25 with "missed N ticks" in next prompt context. `skip_missed`: fire current tick only. |
| Webhook signature verification fails | Trigger rejected with 401; no routine run created. |

## Persistence guarantees

Survives a kandev process restart: all `agent_wakeup_requests` rows including `queued`, `claimed` (re-claimed on restart), and `scheduled_retry_at`; all `office_routines`, `office_routine_triggers` (with `next_run_at` advanced), `office_routine_runs` history; `agent_continuation_summaries` (last-good); `runs` rows including `result_json`, `assembled_prompt`, `summary_injected` snapshots.

Does NOT survive (reconstructed on next tick): in-memory claim leases - a `claimed` wakeup whose process died is picked up by the staleness/recovery path; the scheduler's claim query is the source of truth. The recovery sweep's idempotency is via the `NOT EXISTS` check on `wakeup_requests` plus the dispatched wakeup's `idempotency_key`.

Retention: idempotency-key dedup window 24 hours; summary cap 8 KB per row; routine run history retained for inspection (no automatic prune in scope here); catch-up cap (default 25) drops missed routine ticks beyond it (not recorded individually).

The scheduler reads all `queued` and unexpired-retry wakeup requests on boot and resumes processing them.

## Scenarios

- **GIVEN** a task is assigned to a worker agent instance, **WHEN** the assignment is saved, **THEN** a `task_assigned` wakeup is queued for that agent. The scheduler claims it, creates a session, and the agent starts working on the task.

- **GIVEN** a worker agent is currently running a session (at capacity), **WHEN** a `comment` wakeup arrives for the same agent, **THEN** the wakeup stays in `queued` status. When the current session completes, the next scheduler tick picks up the wakeup and the agent processes the comment.

- **GIVEN** a task with three subtasks assigned to different workers, **WHEN** all three subtasks reach `done`, **THEN** a single `task_children_completed` wakeup (coalesced) is queued for the parent task's assignee.

- **GIVEN** a coordinator agent with the pre-installed "Coordinator heartbeat" routine (cron `*/5 * * * *`), **WHEN** the wall clock crosses a 5-minute boundary, **THEN** the routines cron tick inserts a `routine`-source wakeup request, the dispatcher creates a taskless run, the agent receives the prompt with the continuation summary prepended, and on completion the summary is upserted under `scope="routine:<id>"`.

- **GIVEN** a wakeup for a `paused` agent instance, **WHEN** the scheduler claims it, **THEN** the wakeup is marked `finished` with no session created. The wakeup is not retried.

- **GIVEN** a backend restart, **WHEN** the scheduler starts, **THEN** it reads all `queued` wakeup requests from SQLite and resumes processing them. No wakeups are lost.

- **GIVEN** a parent task with subtasks [Spec (requires_approval, assigned to planner), Build (blocked_by Spec, assigned to developer)], **WHEN** the planner completes the Spec subtask, **THEN** Spec moves to `in_review`. **WHEN** the user approves, **THEN** Spec moves to `done`, Build's blocker resolves, and a `task_blockers_resolved` wakeup is queued for the developer agent. The developer starts working on Build automatically.

- **GIVEN** a heavy routine "Daily Dep Update" with cron `0 9 * * *` in UTC and assignee "Frontend Worker", **WHEN** the clock reaches 09:00 UTC, **THEN** a task titled "Daily Dep Update - 2026-04-25" is created on the `routine` workflow, assigned to Frontend Worker, and a `task_assigned` wakeup is queued.

- **GIVEN** a routine with `concurrency_policy=skip_if_active` and an active task from a previous run, **WHEN** the routine fires again, **THEN** no new task is created and the routine run is recorded with status `skipped`.

- **GIVEN** a routine with a webhook trigger and `signing_mode=hmac_sha256`, **WHEN** an external system POSTs to the trigger URL with valid signature and payload `{"branch": "release/2.0"}`, **THEN** the routine fires with `{{branch}}` resolved to "release/2.0" in the task template.

- **GIVEN** the scheduler was down for 3 hours, **WHEN** it restarts, **THEN** routines with `catch_up_policy=skip_missed` fire only the current tick; routines with `enqueue_missed_with_cap` fire missed ticks up to the cap (default 25), with overflow summarized as "missed N ticks" in the next prompt context.

- **GIVEN** a user on the routines page, **WHEN** they click "Run Now" on a routine with a required `{{reason}}` variable, **THEN** a modal prompts for the variable value before firing.

- **GIVEN** a worker agent with `skip_idle_wakeups = true` and no tasks in `TODO` or `IN_PROGRESS` state, **WHEN** a lightweight-routine wakeup is claimed, **THEN** the scheduler logs `wakeup_idle_skipped`, marks the wakeup `finished`, records an activity entry, and does not launch a session. A `task_assigned` wakeup arriving for the same agent skips the check and proceeds normally.

- **GIVEN** a coordinator agent with default `skip_idle_wakeups = false`, **WHEN** a routine-fired wakeup is claimed and the coordinator has no directly assigned tasks, **THEN** the skip check is not performed and the wakeup proceeds normally so the coordinator can do proactive coordination work.

- **GIVEN** a wakeup fails with `"rate_limit_error: resets at 4:00 AM UTC"`, **WHEN** `HandleWakeupFailure` is called at 3:45 AM UTC, **THEN** the wakeup is scheduled for `04:00:30 AM UTC` (parsed time + 30s buffer). The exponential backoff table is not consulted. Similarly `"Retry-After: 3600"` schedules for `now + 3600s + 30s`, and `"try again in 5 minutes"` schedules for `now + 5m30s`.

- **GIVEN** a wakeup fails with a generic network timeout (no rate-limit keywords), **WHEN** `HandleWakeupFailure` is called, **THEN** the existing exponential backoff applies unchanged. If retries exhaust `MaxRetryCount`, `escalateFailure` is called as normal - the rate-limit path does not suppress escalation.

- **GIVEN** a `task_assigned` wakeup is queued for agent A on task T, **WHEN** task T is reassigned to agent B before the wakeup is claimed, **THEN** the wakeup is cancelled with reason `assignee_changed`, agent A is not launched, and a `wakeup_stale_cancelled` activity entry is logged.

- **GIVEN** a wakeup for task T fails and a retry is scheduled 10 minutes out, **WHEN** task T is reassigned (or cancelled) before the retry fires, **THEN** the retry is cancelled at promotion time with reason `retry_stale_assignee` (or `retry_task_cancelled`), execution locks are cleared, and a `wakeup_retry_cancelled` activity entry is logged. A PATCH reassign on the API cancels pending retries for the previous assignee immediately.

- **GIVEN** a task in `TODO` state assigned to agent A has no queued or finished wakeup and was created within the lookback window, **WHEN** the recovery sweep runs, **THEN** a `task_assigned` wakeup is dispatched and a `recovery_dispatch` activity entry is logged. Tasks that already have a queued/finished wakeup, or that fall outside `recovery_lookback_hours`, are skipped.

- **GIVEN** a wakeup for a task that has reached `DONE` state, **WHEN** the staleness check runs at claim time, **THEN** the wakeup is cancelled with reason `task_terminal` and the agent is not launched.

- **GIVEN** a coordinator's pre-installed "Coordinator heartbeat" routine is deleted by the user, **WHEN** the next scheduler tick runs, **THEN** no routine wakeup is queued for that coordinator; the coordinator only wakes via reactive sources (comments, errors, manual, self, user).

- **GIVEN** two routine triggers fire for the same coordinator within the coalescing window, **WHEN** the dispatcher claims the first, **THEN** the second wakeup-request is inserted with `status="coalesced"`, its payload is merged into the first run's `context_snapshot`, and `coalesced_count` is incremented.

## Out of scope

- Distributed scheduling across multiple backend instances (single-process scheduler); deduplication of recovery dispatches across instances (idempotency key is sufficient).
- Priority ordering beyond FIFO within the queue (task priority handled at assignment time).
- Wakeup scheduling with future timestamps as a primary API (routines handle scheduled execution).
- Rate limiting per agent beyond the single-concurrency guard and cooldown.
- Dynamically adjusting heartbeat cadence based on workload (backpressure scheduling).
- Complex workflow chains (routine A triggers routine B); routines create independent tasks, inter-task dependencies use the blocker system.
- Routine templates shared across workspaces; routine-level budget limits (use agent or project budgets); plugin-managed routines (`pluginManagedResources` pattern); routine revisions / rollback.
- Webhook trigger UI polish beyond the create dialog and detail page (covered in a separate routine-webhooks spec).
- A full agent-memory subsystem (vector store, semantic recall) - the continuation summary is deliberately the minimum viable memory layer.
- Web UI for editing summaries directly (read-only display in the agent's overview tab is enough); backfilling historical heartbeat conversations into summaries.
- Modifying `MaxRetryCount` for rate-limit errors (stays at 4); surfacing parsed reset times in the UI or inbox; per-provider configuration of reset-time parsing patterns; handling `Retry-After` as an HTTP response header (this spec covers only error message text).
- Per-agent or per-project recovery lookback window overrides (workspace-level is sufficient).
- Surfacing retry cancellation reasons in the agent detail page runs tab (existing retry count display is adequate); automatic reassignment when a stale wakeup is cancelled (cancellation only - the scheduler does not infer intent).
- Suppressing non-heartbeat wakeups based on task state; configuring which task states count as "actionable" per agent or workspace.
- Recovery sweeps for non-`TODO` states (`IN_PROGRESS` tasks with no active session are covered separately by the blocked-task-escalation spec).
- Event-based triggers from external systems beyond webhooks (GitHub event subscriptions use webhooks as transport).
