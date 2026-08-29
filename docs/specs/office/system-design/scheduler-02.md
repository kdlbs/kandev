---
status: draft
system: office
requirements:
  - REQ-OFFICE-SCHEDULER-001
created: 2026-04-25
owners:
  - cfl
---
# Office Scheduler System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-SCHEDULER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-SCHEDULER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
scope              TEXT NOT NULL  -- routine:<routine_id> or agent:<agent_profile_id>
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
continuation_scope TEXT NOT NULL DEFAULT ''     -- summary key selected at run creation
```

The run repository selects `continuation_scope` once when it creates a row:
`routine:<id>` when the initial context has a routine ID, otherwise
`agent:<agent_profile_id>`. Coalescing can change `context_snapshot`, but it
does not change the scope. The first wake that creates the run owns its
continuation-summary chain.

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

`GetRunDetail` returns the `RunDetail` aggregate including the new columns (`result_json`, `assembled_prompt`, `summary_injected`, `continuation_scope`) plus existing `context_snapshot`, `output_summary`. The detail UI surfaces a "Prompt" tab rendering the assembled prompt + injected summary. WS live updates via `useRunLiveSync` are unchanged.

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

## Permissions

The scheduler's reach is workspace-scoped: every routine, wakeup-request, run, and recovery dispatch is keyed by `workspace_id` (routines) or by an `agent_profile_id` that resolves to a single workspace. Cross-workspace dispatch is not possible by construction - the dispatcher only loads agents and tasks from the routine / wakeup's own workspace.

| Action | Who can perform it |
|---|---|
| Create / update / delete / pause routines in a workspace | The workspace's UI user (no per-field permission model in v1). Routines API endpoints under `/api/office/routines` are not gated by agent capability today. |
| Fire a routine manually (`POST /api/office/routines/:id/fire`) | UI user. Agent callers cannot manually fire routines from the runtime (no `CapabilitySpawnRoutine`). |
| Fire a webhook trigger (`POST /api/routine-triggers/:public_id/fire`) | Any external caller possessing a valid signature per the trigger's `signing_mode` (`none` accepts unauthenticated requests). Failed signature returns 401; no routine run is created. |
| Enqueue a wakeup (`office.wakeup.Service.Enqueue`) | Internal-only Go API. Office event subscribers, the workflow engine, comment handlers, approval handlers, and the recovery sweep are the legitimate callers. Agents cannot synthesize wakeups directly - the `self`-source wakeup is produced by the runtime in response to a recognized agent tool call, not by an HTTP route. |
| Override / cancel an in-flight wakeup or scheduled retry | Indirect only. Reassigning a task via the task API or cancelling / completing the underlying task triggers the staleness and retry-cancellation paths described above. There is no direct "cancel this wakeup" endpoint. |
| Read run inspection (`assembled_prompt`, `summary_injected`, `result_json`) | UI user for the workspace. No agent-side capability exposes raw prompts cross-agent. |
| Adjust workspace settings (`recovery_lookback_hours`) | UI user. Workspace-level setting; no per-agent or per-project override. |

Agent capabilities (`CapabilityPostComment`, `CapabilityUpdateTaskStatus`, etc., per `agents.md`) govern what an agent can do **inside** a run. They do not gate which agents the scheduler will wake - that is determined entirely by routine assignment, task assignment, and the `reviewers` / `approvers` lists managed in `tasks.md`.

A formal per-route authorization model (workspace membership, admin role, RBAC over routines / wakeups) is out of scope here; see Out of scope.

## Failure modes

| Dependency / invariant | Behavior |
|---|---|
| SQLite write failure during enqueue | Source caller surfaces error; idempotency-key collision is silent (treated as already-enqueued). |
| Claim query returns 0 rows | Another process won the claim or no eligible wakeup; tick exits cleanly. |
| Office is enabled with no Office workflow/project | Office recovery skips its task scan; ordinary Kanban assignments are not launched. |
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
| Graceful shutdown begins | Queue and cron loops cancel and join before repositories and SQLite close; no scheduler logs `database is closed`. |

## Persistence guarantees

Survives a kandev process restart: all `agent_wakeup_requests` rows including `queued`, `claimed` (re-claimed on restart), and `scheduled_retry_at`; all `office_routines`, `office_routine_triggers` (with `next_run_at` advanced), `office_routine_runs` history; `agent_continuation_summaries` (last-good); `runs` rows including `result_json`, `assembled_prompt`, `summary_injected`, and `continuation_scope` snapshots.

Does NOT survive (reconstructed on next tick): in-memory claim leases - a `claimed` wakeup whose process died is picked up by the staleness/recovery path; the scheduler's claim query is the source of truth. The unstarted-task recovery sweep suppresses duplicates with its `NOT EXISTS` check on `runs`: any queued, claimed, or finished run of any age blocks redispatch, while failed and cancelled runs do not.

Retention: idempotency-key dedup window 24 hours; summary cap 8 KB per row; routine run history retained for inspection (no automatic prune in scope here); catch-up cap (default 25) drops missed routine ticks beyond it (not recorded individually).

The scheduler reads all `queued` and unexpired-retry wakeup requests on boot and resumes processing them.

## Scenarios

- **GIVEN** an authoritative Office task is assigned to a worker agent instance, **WHEN** the assignment is saved, **THEN** a `task_assigned` wakeup is queued for that agent. The scheduler claims it, creates a session, and the agent starts working on the task.
- **GIVEN** Office mode is enabled but a workspace contains only Kanban workflows, **WHEN** a Kanban task with a runner remains in `TODO`, **THEN** Office subscribers and recovery do not queue a `task_assigned` run for it.

- **GIVEN** Kandev is shutting down while the shared runs scheduler or cron loop is active, **WHEN** graceful shutdown reaches database cleanup, **THEN** those loops have already stopped and joined and no `sql: database is closed` scheduler error is emitted.

- **GIVEN** a worker agent is currently running a session (at capacity), **WHEN** a `comment` wakeup arrives for the same agent, **THEN** the wakeup stays in `queued` status. When the current session completes, the next scheduler tick picks up the wakeup and the agent processes the comment.

- **GIVEN** a task with three subtasks assigned to different workers, **WHEN** all three subtasks reach `done`, **THEN** a single `task_children_completed` wakeup (coalesced) is queued for the parent task's assignee.

- **GIVEN** a coordinator agent with the pre-installed "Coordinator heartbeat" routine (cron `*/5 * * * *`), **WHEN** the wall clock crosses a 5-minute boundary, **THEN** the routines cron tick inserts a `routine`-source wakeup request, the dispatcher creates a taskless run, the agent receives the prompt with the continuation summary prepended, and on completion the summary is upserted under `scope="routine:<id>"`.

- **GIVEN** a wakeup for a `paused` agent instance, **WHEN** the scheduler claims it, **THEN** the wakeup is marked `finished` with no session created. The wakeup is not retried.

- **GIVEN** a backend restart, **WHEN** the scheduler starts, **THEN** it reads all `queued` wakeup requests from SQLite and resumes processing them. No wakeups are lost.

- **GIVEN** a parent task with subtasks [Spec (requires_approval, assigned to planner), Build (blocked_by Spec, assigned to developer)], **WHEN** the planner completes the Spec subtask, **THEN** Spec moves to `in_review`. **WHEN** the user approves, **THEN** Spec moves to `done`, Build's blocker resolves, and a `task_blockers_resolved` wakeup is queued for the developer agent. The developer starts working on Build automatically.

- **GIVEN** a heavy routine "Daily Dep Update" with cron `0 9 * * *` and assignee "Frontend Worker", **WHEN** 09:00 UTC arrives, **THEN** its task is created on the `routine` workflow and `task.created` starts the normal task-bound session.

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

- **GIVEN** an authoritative Office task in `TODO` state assigned to agent A has no queued, claimed, or finished run and was created within the lookback window, **WHEN** the recovery sweep runs, **THEN** a `task_assigned` run is dispatched and a `recovery_dispatch` activity entry is logged. Ordinary Kanban tasks, tasks with a queued, claimed, or finished run of any age, and tasks created outside `recovery_lookback_hours` are skipped.

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
