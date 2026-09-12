---
status: draft
system: office
requirements:
  - REQ-OFFICE-KILL-SWITCH-001
  - REQ-OFFICE-KILL-SWITCH-002
  - REQ-OFFICE-KILL-SWITCH-003
  - REQ-OFFICE-KILL-SWITCH-004
  - REQ-OFFICE-KILL-SWITCH-005
  - REQ-OFFICE-KILL-SWITCH-006
---

# Office Workspace Kill Switch System Design

## Purpose and boundaries

Office owns this contract because every gated launch point is an Office
component: the routine dispatcher, the wakeup dispatcher, the run queue writers,
and the scheduler integration that claims runs and starts agents.

Used but not owned: the shared `runs` table and its status writers in
`internal/runs`; task execution cancellation via the `TaskCanceller` seam Office
already holds; the Office workspace scope middleware in `internal/backendapp`.

## Surface decision

- **Rejected: a runtime feature flag.** `features.office` is instance-wide,
  carries `RestartRequired: true`, and has no field for actor or reason.
- **Rejected: `office_workspace_governance`.** Its accessor returns
  `(false, nil)` for every read error, not only a missing row, so a stop stored
  there silently stops applying the moment the read fails.
- **Rejected: `office_workspace_settings`.** The closest runner-up — already one
  row per workspace, so atomicity holds — but provenance for both pause and
  release costs six columns that must be nulled on resume, destroying the record
  of what happened as the price of resuming. It also has no HTTP surface.
- **Chosen: `office_workspace_pauses`**, an append-only hold table modelled on
  `office_task_tree_holds`. One unreleased row per workspace is the whole state;
  released rows stay as history, and the read gate, release semantics and event
  shape all have an in-repo precedent to match.

## Components and responsibilities

**Backend** — `office/pause` (new package) owns the pause record, the gate read,
the halt sweep and the HTTP handlers, and is the only writer of
`office_workspace_pauses`. `office/repository/sqlite` gains the table and the
queries below; `internal/runs/repository/sqlite` gains `RequeueClaimedRun`;
`internal/backendapp` registers the routes. `office/routines`, `office/service`,
`office/scheduler` and `office/wakeup` consult the gate at the sites named under
[Gate points](#gate-points).

**Frontend** — a pause banner rendered by `OfficeShell` so it appears on every
Office page without per-page wiring; pause and resume calls in the Office API
client; pause state on the Office store slice, hydrated and re-read entirely by
explicit reads, with no live push (see
[Frontend state](workspace-kill-switch-02.md#frontend-state) in Part 2).

## Data and contracts

### Table

```sql
CREATE TABLE IF NOT EXISTS office_workspace_pauses (
    id                TEXT PRIMARY KEY,
    workspace_id      TEXT NOT NULL,
    reason            TEXT NOT NULL,
    created_by        TEXT NOT NULL,
    created_by_kind   TEXT NOT NULL,
    created_at        TIMESTAMP NOT NULL,
    released_at       TIMESTAMP,
    released_by       TEXT NOT NULL DEFAULT '',
    released_reason   TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_office_workspace_pause_active
    ON office_workspace_pauses(workspace_id) WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_office_workspace_pause_history
    ON office_workspace_pauses(workspace_id, created_at DESC);
```

The partial unique index makes -001.3 hold without a lock: two concurrent pauses
both attempt the insert, the loser gets a uniqueness violation, re-reads the
active row and returns it. `created_at` is the tiebreak column for history
ordering, the primary key breaking a same-timestamp tie. `created_by_kind`
separates a human operator from an automated caller, so a later auto-pause
capability is distinguishable without parsing `created_by`.

`office_routine_runs` has no column saying *why* a run was skipped, and the
concurrency path already writes `skipped`, so a pause-skip and a skip-if-active
are indistinguishable — which -002.11 requires they not be. Two columns are added
by idempotent `ADD COLUMN` migration, never by editing the `CREATE TABLE` alone
(the repository's schema rule):

```sql
ALTER TABLE office_routine_runs ADD COLUMN skip_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE office_routine_runs ADD COLUMN pause_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_office_routine_run_pause_once
    ON office_routine_runs(routine_id, pause_id) WHERE pause_id != '';
```

`skip_reason` carries exactly one value, `workspace_paused`; existing skips keep
the `''` default, so nothing is reinterpreted, and a gate **read error** writes no
row at all, so there is no second value and no undeduplicated row class.

`pause_id` records which pause blocked the fire, and the partial unique index
enforces the -002.13 bound. **The insert is conditional and is the only row
written on this path**: the gate sits *before* `CreateRoutineRun`, so a blocked
fire never creates a dispatch row needing an update. The first blocked fire of a
routine under a pause inserts its skipped row; every later one loses the
uniqueness race and writes nothing, so a long pause cannot bury a routine's
history — no counter, no read-then-write.

### Repository queries

- `GetActiveWorkspacePause(ctx, workspaceID) (*WorkspacePause, error)` —
  `WHERE workspace_id = ? AND released_at IS NULL`. Returns `(nil, nil)` for no
  row and a non-nil error for a failed read. The gate reads nil as running and an
  error as paused.
- `CreateWorkspacePause(ctx, *WorkspacePause) error` — surfaces the uniqueness
  violation as a typed `ErrWorkspaceAlreadyPaused`, not a driver error.
- `ReleaseWorkspacePause(ctx, id, releasedBy, reason) (bool, error)` — CAS update
  with `WHERE id = ? AND released_at IS NULL`, reporting whether it changed a
  row. That boolean resolves two concurrent releases to one (-005.7).
- `CreatePauseSkippedRoutineRun(ctx, routineID, triggerID, source, pauseID) (bool, error)`
  — the conditional insert above. Persists all four arguments, plus
  `status = 'skipped'`, `skip_reason = 'workspace_paused'` and `created_at` at
  insert time; only columns none of those name take a table default. `routine_id`
  and `source` are `NOT NULL` with no default, and the partial unique index is
  keyed on `routine_id`, so no identifier may be dropped. Reports `false`
  **without error** when that index rejects it, the expected outcome for every
  fire after the first.
  **A real insert error does not change the launch decision.** The pause is
  already confirmed when this is called, so the caller logs the write failure,
  still blocks the fire, and still returns the typed paused error: a webhook or
  manual fire gets `409`, and `processCronTrigger` still returns `nil`. This row
  records a blocked fire, it does not cause one, and answering `500` or a cron
  `ERROR` would report an operator's stop as a dispatch failure — the same
  misreporting the cron rule under [Gate points](#gate-points) rejects. A lost
  row still appears in the gate's structured log and in
  `office_pause_blocked_total`.
- `ListInflightRunsForWorkspace(ctx, workspaceID) ([]InflightRun, error)` —
  returns `id` plus the payload's task id for runs in `queued` or `claimed`
  belonging to an agent in the workspace, via
  `agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`.
  Selecting on the agent, not the payload task id, covers taskless runs; those
  carry an empty task id and are omitted from the task set. **This query gives
  the halt sweep its identifiers** — a bare `CancelRunsWhere` returns only a row
  count, which cannot say which checkouts to release or which executions to
  cancel. Unordered set; empty is not an error and yields a zero-count sweep. It
  sees only work that has a `runs` row, which is why the heavy path needs the
  query below.
- `ListLiveRoutineTaskIDsForWorkspace(ctx, workspaceID) ([]string, error)` —
  distinct `linked_task_id` for `office_routine_runs` of routines in the
  workspace where `linked_task_id != ''`, joined to `tasks` and restricted to
  rows whose `tasks.state` is **not** one of the three
  `models.IsTerminalTaskState` values (`completed`, `failed`, `cancelled`).
  **Routine-run status is deliberately not the filter**: `SyncRunStatus` is a
  no-op stub today, so a heavy row never leaves `task_created` and filtering on
  it would select every heavy task the workspace has ever run. The task's own
  state is the only honest liveness signal. Unordered set; an empty result is not
  an error.
- `CancelRunsForWorkspace(ctx, runIDs, reason) (int, error)` — delegates to the
  runs repository's `CancelRunsWhere` over those ids.
- `ReleaseCheckoutsForWorkspace(ctx, runIDs) error` — clears
  `checkout_agent_id`, `checkout_at`, `checkout_run_id` on tasks whose
  `checkout_run_id` is one of those ids. **Those ids are the whole snapshot from
  step 5(a), not the subset the cancel actually transitioned**: `CancelRunsWhere`
  returns a row count and no identifiers, so a "just cancelled" list cannot be
  derived, and this design does not add one. Passing the whole snapshot is safe
  because the predicate is keyed on `checkout_run_id`. A snapshot run that
  reached a terminal state in between has either cleared its checkout already, so
  no row matches, or still holds one, so releasing it is exactly what -003.4
  asks; and a checkout taken meanwhile by a *newer* run carries that run's id,
  which is not in the snapshot, so a live run's checkout is never touched.
- `RequeueClaimedRun(ctx, runID) (bool, error)` — in `internal/runs`, beside
  `ScheduleRetry` and `RecoverStale`. Sets `status = 'queued', claimed_at = NULL`
  under the CAS predicate `WHERE id = ? AND status = 'claimed'`, leaving
  `retry_count` and `scheduled_retry_at` untouched. The predicate stops it
  resurrecting a run a concurrent halt sweep already cancelled; a `false` return
  means exactly that, and the caller does nothing further.

### HTTP

Mounted under the existing Office group:

- `GET /api/v1/office/workspaces/:wsId/pause` — `{ "workspace_id": string,
  "paused": bool, "pause": { id, reason, created_by, created_by_kind,
  created_at } | null }`. A failed read returns `500`; a client must not render
  "running" from an error. **The read performs the same workspace-existence check
  as the two mutations and returns `404` for a `:wsId` that names none** (-006.9).
  It is on the read that the check earns its keep: `GetActiveWorkspacePause`
  returns `(nil, nil)` for an unmatched id exactly as it does for a running
  workspace, and the scope middleware short-circuits for every route when
  authentication is disabled, so without the check a typo'd or deleted workspace
  would answer `200 {"paused": false}` — a *stopped-looking* system reported as
  running, which is the one confusion REQ-006 exists to prevent.
- `POST /api/v1/office/workspaces/:wsId/pause` — body `{ "reason": string }`.
  `200` with `{ "workspace_id", "paused": true, "pause": {...}, "sweep": {
  "runs_cancelled": int, "executions_cancelled": int,
  "executions_not_running": int, "failures": int } }`, on success and when
  already paused; `400` on a missing, blank or over-length
  reason. The `sweep` object is the observable -004.6 and -003.7 require.
- `POST /api/v1/office/workspaces/:wsId/resume` — body `{ "reason": string }`,
  optional here. `200` with `{ "workspace_id", "paused": false, "pause": null }`,
  including when not paused. `pause` is present and `null`, not absent, so a
  client clears its record by assignment rather than by inferring a missing key.

`workspace_id` and `paused` are on all three responses: one shape everywhere, and
`workspace_id` is what lets a client match a late response to the workspace it
asked about — the `pause` object cannot, being `null` while running. Routes carry
`:wsId`, so `officeWorkspaceScopeMiddleware` authorizes them with no new resolver
entry, but they must still be reflected in the route-scope completeness test's
expectations. Blocked callers get `409` with
`{ "workspace_id", "error", "paused": true, "reason" }`, so a webhook sender can
distinguish a paused workspace from a disabled trigger. Every `409` carries
`workspace_id` and `paused` for the reason the success bodies do.

## Gate points

One exported predicate, `PauseState(ctx, workspaceID) (*WorkspacePause, error)`,
read at every point below. It returns the **record**, not a boolean: the reason is
required in the `409` body (-002.3) and by -006.1, and the `pause_id` on the
skipped routine-run row (-002.11, -002.13). A bare `bool` supplies neither, and a
second lookup to fetch them would race a resume. `(nil, nil)` means running; a
non-nil error means the read failed. There is no cache: a stop that lags is not a
stop, and the read is one indexed row lookup on the caller's connection.

The record a gate reads is the one it acts on. If a resume commits after that
read, the `409` body and the `pause_id` still describe the state at decision
time, and the workspace's next fire proceeds normally.

Each row names the **package-qualified symbol** to instrument, because Office
carries `service/` vs `scheduler/` near-duplicate pairs here, one live and one
dormant.

| Gate | Symbol to instrument | Behavior when paused | Behavior on gate read error |
| --- | --- | --- | --- |
| Routine dispatch | `office/routines.RoutineService.dispatchRoutineRun`, **before** `CreateRoutineRun` | `CreatePauseSkippedRoutineRun`; no dispatch row; typed paused error, mapped to `409` by the webhook and manual handlers | write no row at all; typed gate error, mapped to `503` (-002.12) |
| Run queue write (service) | `office/service.Service.QueueRun`, beside its own `guardAgentStatus` | typed paused error; no run row created | no run row created; originating write still succeeds (-002.6) |
| Run queue write (scheduler) | `office/scheduler.SchedulerService.QueueRun`, reached via `QueueRunCtx`, beside its own separate `guardAgentStatus` | as above | as above |
| Run processing, early | `office/service.SchedulerIntegration.processRun`, after the agent is loaded and before the staleness check | finish terminally with a `workspace_paused` outcome; no checkout taken yet | `RequeueClaimedRun`; retried next tick |
| Run processing, final | `office/service.SchedulerIntegration.prepareAndLaunch`, immediately before `si.launchAgent` | `releaseCheckoutIfNeeded`, then finish terminally with `workspace_paused`; no agent launched | `releaseCheckoutIfNeeded`, then `RequeueClaimedRun` |
| Wakeup fresh run | `office/wakeup.Dispatcher.createFreshRun` | `MarkWakeupRequestSkipped` with reason `workspace_paused`; no run row created | leave the request `queued` (-002.9); see below |

**Where each gate gets its `workspaceID`.** Only run processing has it in hand:
`processRun` loads the agent and threads that value into `prepareAndLaunch`. The
derivation for the rest is fixed here rather than left to the site, because two
of them are near-duplicate functions that would otherwise drift.

- **Routine dispatch** reads `routine.WorkspaceID`, already on the struct
  `dispatchRoutineRun` is called with. No extra query.
- **Both run-queue writers** widen their own `guardAgentStatus` from `error` to
  `(*models.AgentInstance, error)`. Each already calls `GetAgentFromConfig` and
  discards the struct, so widening reuses that fetch and costs no query; a second
  independent lookup is rejected because the two copies would drift apart.
- **The wakeup dispatcher** has no agent at all — `WakeupRequest` carries only
  `AgentProfileID` — so `createFreshRun` resolves it through the dispatcher's
  existing `AgentReader` field, which exists for this and has no other caller.

**Two different failures wear the same error today, and exactly one gate can tell
them apart.** `GetAgentInstance` maps `sql.ErrNoRows` to a plain
`"agent instance not found"` string and returns any driver error unwrapped, while
its `WHERE` clause carries `workspace_id != '' AND deleted_at IS NULL`. So "has
no workspace attribution", "was deleted" and "never existed" all arrive as that
one non-sentinel message, and a genuine read failure arrives as something else —
indistinguishable to a caller. Export a sentinel `ErrAgentNotFound`, wrapped by
the existing message so no caller's text changes, and branch on `errors.Is` at
**the wakeup dispatcher, the only site that can receive it**:

- **Not found** is -002.10's case and takes -002.10's answer: launch behavior is
  left unchanged. The gate does not run, blocks nothing and writes nothing.
  No pause record can apply to an agent with no workspace, so there is nothing
  for the gate to decide.
- **Every other lookup error** is a gate read error, taking the read-error column
  of its own row. It is an absence of information about the workspace, not a
  statement about it; treating a transient fault as "no attribution" would fail
  *open*.

**The two run-queue writers do not get this branch, because they cannot.** Both
`guardAgentStatus` copies resolve through `GetAgentFromConfig`, which is not a
passthrough: it discards the `GetAgentInstance` error, retries by name, and on
failure returns a **fresh `fmt.Errorf` carrying no `%w`**. A sentinel wrapped one
layer below never reaches `errors.Is` there, for any failure. Two fixes exist and
both are **rejected**: bypassing that resolver narrows those guards to ids,
dropping the by-name resolution their callers may use; making both copies wrap
changes the error semantics seen by every caller of a resolver used across 13
files. Either would be paid for a classification no acceptance criterion observes
at those rows — and none does, because the outcome there is the same both ways:
`guardAgentStatus` already errors on an unresolvable agent, so no run row is
created and the originating write still succeeds. **-002.10 and -002.9 both hold
at those two rows on the behavior the sites already have**, with no gate consulted
and nothing written. Only if a later change makes those two outcomes differ does
the branch become necessary, and then one of the two rejected fixes is paid for
deliberately, as an amendment here rather than a build-time improvisation.

**-002.10 needs code at one site, and the workspace-scoped filter is why.** An
unattributed agent is invisible to every Office agent lookup, so it does not fail
to reach a gate — it reaches one and fails to resolve there. Routing that down
the read-error branch would fail closed on it. That bites at the **wakeup** row
alone: `createFreshRun` loads no agent today, so an unattributed agent's wakeup
creates a run, and failing closed would instead leave the request `queued` with
nothing to re-drive it — the launch-behavior change -002.10 forbids, and for a
*deleted* agent a permanently stuck row where `HandleRunFailure` bounds the
failure today. The branch works there because `Dispatcher.agents` is the office
repository itself, so `createFreshRun` reaches `GetAgentInstance` directly and the
sentinel survives.

**The two run-processing rows are outside this rule.** `processRun` loads the
agent *before* the gate and already routes a failed load to `HandleRunFailure`.
That path is not this capability's to change, and the rule above does not reach
it.

**Routine dispatch performs no lookup**, so neither branch applies to it; it
takes `routine.WorkspaceID` directly. An empty value there takes the not-found
branch for the same reason an unattributed agent does — no pause record can name
the empty workspace — so dispatch proceeds ungated rather than failing closed on
a routine that carries no workspace.

**The cron path has no gate of its own, deliberately.** Gating
`processCronTrigger` before `DispatchRoutineRunWithMissed` would skip
`dispatchRoutineRun`, so no `office_routine_runs` row would be written and
-002.11 would be unsatisfiable for cron — an unexplained gap in the runs of the
one trigger kind this capability exists for. Falling through to the shared
dispatch gate is what makes cron, webhook and manual fires behave identically.

`processCronTrigger` **returns `nil` for a confirmed pause** rather than
propagating the typed error, because `TickScheduledTriggers` logs anything it
returns at `ERROR`: propagating would emit one line per tick for the pause's
whole duration and make an operator's stop look like a dispatch failure. The
skipped row and the gate's own structured log are the record. A gate **read
error** is propagated normally — that one is a real fault.

**The same log-noise argument reaches the event path, and is answered
differently there.** `scheduler.reactivity` logs any `QueueRunCtx` error at
`ERROR` and the approval adapter at `WARN`, so a paused workspace would emit one
such line per task event for the pause's whole duration — the same misreporting
the cron rule above exists to prevent. Cron could be quietened wholesale because
`TickScheduledTriggers` is that error's only consumer; these call sites are
shared with genuine queue failures that must stay loud, so the typed paused error
is **not** swallowed here. It propagates as normal and each caller logs it at
`DEBUG`, branching on the typed error alone and leaving every other error at its
present level. This edits two call sites the capability does not otherwise touch,
deliberately: without it the first thing an operator sees after pressing stop is
an error storm.

-002.2 needs no pause-specific code: `processCronTrigger` already advances the
cursor unconditionally (`ClaimTrigger`, `UpdateTriggerNextRun`, then dispatch), so
the cursor is past the tick before the gate is consulted, a pause cannot leave
`next_run_at` in the past, and resume cannot release a catch-up burst.

**Both run-queue-write rows are required; they are not one call path.** They are
independent functions with independent `guardAgentStatus` implementations and
disjoint callers, and `internal/office/AGENTS.md` names both as "the entry
points". The scheduler one carries most of the events -002 exists to cover —
`office/scheduler.reactivity` routes five task events through `QueueRunCtx`, and
the approval adapter routes approval-resolved the same way.

**Run processing is read twice, and the second read is load-bearing.** Between
the early read and `si.launchAgent` come the staleness check, idle skip, task
checkout, budget check, executor resolution, runtime-context persistence, token
minting and prompt assembly; a pause committing in that stretch would otherwise
launch an agent into a stopped workspace with its run row already cancelled by
the sweep. The early read blocks the common case before a checkout is taken; the
final read is what -002.7 rests on, and because it sits after the checkout its
blocked path releases that checkout first, as the executor-resolution and
token-mint failure paths already do. Opposite trap on symbol names:
`office/scheduler`'s `ClaimNextRun` / `ProcessRunGuard` / `FinishRun` in
`run_processing.go` are a **dormant** copy with no production caller.

**The wakeup gate is defence in depth.** `Dispatch` has one production caller, in
the lightweight materialisation path of `dispatchRoutineRun`, downstream of the
routine dispatch gate, so a paused workspace cannot reach it today; it is gated
anyway so a later second caller is covered by default. On a gate error the
request stays `queued`, and **nothing re-drives that row, and nothing needs to**:
`internal/office` has no retry loop for one and `ListQueuedWakeupRequestsForAgent`
has no production caller. The routine fires again on its next cadence with a
fresh request; the abandoned row is inert. The retryable unit is the fire.

Because the gate is read at these chokepoints rather than per event kind, it is
reason-agnostic: a wake reason added later is gated with no further change, which
-002.5 requires. Office declares seventeen `RunReason*` constants; none is
enumerated in the gate logic.

A new `workspace_paused` run outcome joins the existing outcome set. It needs no
bucketing change: run counts bucket any `outcome` other than `processed` as
skipped, so the new value is classified correctly the day it is written.

## Control flow

### Pause

1. Verify the workspace exists. The scope middleware short-circuits when
   authentication is disabled — the default in every shipped profile — so the
   handler checks explicitly and returns `404` (-006.9).
2. Validate the reason: trim surrounding whitespace, reject empty, then reject
   over 500 **Unicode code points** on the trimmed value
   (`utf8.RuneCountInString`, as in `task/service/task_title.go`, not bytes — at
   500 bytes a CJK reason would cap near 166 characters). The trimmed value is
   stored; "stored verbatim" under Security means no escaping, re-encoding or
   interpolation, not untrimmed.
3. Resolve the actor: the session user with authentication enabled, otherwise
   `userstore.DefaultUserID` (`"default-user"`), the sentinel `auth/httpmw`
   already injects as the synthetic admin, with `created_by_kind = "user"`. The
   field is never left empty (-004.7).
4. **In one transaction**, insert the pause record and write its activity log
   entry. Pairing them is what makes -004.4 and -006.10 hold together: either the
   workspace is paused *and* the audit entry exists, or neither happened and the
   request returns `500` having changed nothing. Splitting them has no safe
   answer — reporting failure after a committed insert tells an operator their
   stop failed when it did not, and reporting success without the entry breaks
   the audit requirement. On `ErrWorkspaceAlreadyPaused`, re-read the active
   record and carry it forward unmodified (-001.5), **and commit a
   `workspace_pause_noop` activity entry for the request that changed nothing**.
   The ordinary repeat pause — an operator pressing the button twice, with no
   race at all — takes this branch, and -005.8 requires every request be
   auditable, so that entry is not optional. It commits on its own, because the
   transaction that would otherwise have carried it is the one whose insert just
   failed, and a failure to write it is logged rather than returned: the workspace
   is paused and there is no insert to roll back, so a `500` here would report a
   stop that did happen as one that did not. That re-read can legitimately
   return **no row**, because a resume may have released it in between; retry the
   transaction **once**. The retry re-runs this whole step, so a retry whose
   insert loses to another *pause* re-reads, finds that record and returns it like
   any other already-paused caller. Only a second **no-row** re-read returns
   `409` naming the interleaved resume, rather than looping, which would hide two
   operators fighting over the switch. That `409` carries
   `{ "workspace_id": string, "error": string, "paused": false }` — deliberately
   **not** the blocked-caller shape, which asserts `"paused": true` and carries the
   pause reason. Neither is available here: the workspace is *not* paused at that
   moment and there is no record to quote. `paused` is present and false so a
   client reads one field the same way on every `409`, and the absence of `reason`
   is what distinguishes a contended switch from a stop. That request writes a
   `workspace_pause_noop` entry too, the lost-a-race case, so -005.8 holds for
   the one branch that commits nothing and returns an error. The caller may
   retry.
   -001.3's "return that identifier to both
   callers" is unaffected and scoped to the direct pause-versus-pause race:
   reaching that `409` requires a resume to commit between this request's two
   attempts, which is a pause-resume-pause sequence, not two concurrent pauses.
   Insert-retry-once is what makes -005.8's "later commit wins" reachable: each
   attempt is a single conditional insert, so commit order decides, and each
   outcome writes its own activity log entry.
5. Run the halt sweep: (a) `ListInflightRunsForWorkspace` to snapshot run ids and
   their task ids; (b) cancel those runs; (c) release the task checkouts those
   run ids held, both skipped outright on an empty snapshot rather than issuing an
   `IN ()` predicate; (d) request cancellation of in-flight task executions, through
   `TaskCanceller` with `force`, for the distinct non-empty task ids from (a)
   **united with every id from `ListLiveRoutineTaskIDsForWorkspace`**. Both
   sources are Office-owned, which is what keeps the sweep off ordinary kanban
   tasks in the same workspace as `## Out of scope` forbids. The second source is
   required because a **heavy** routine materialises a real task whose agent the
   workflow engine starts through `auto_start_agent`: it has no Office `runs` row,
   so (a) alone cannot see it, and -003.3 covers it because a heavy routine's task
   is an Office task. `CancelTaskExecution` is a by-task-id stop, so the union may
   safely overlap and may safely name an idle task — but it is a no-op only in
   *effect*: for a task with no live session `StopByTaskID` returns
   `ErrExecutionNotFound`, not `nil`. Idle tasks are expected members of this
   union, so that sentinel is **not** a failed cancellation; it increments
   `executions_not_running` instead, for the reason given under [Failure and
   recovery](workspace-kill-switch-02.md#failure-and-recovery).
   The three counts partition the deduplicated union and sum to its size:
   `executions_cancelled` for a nil return, `executions_not_running` for that
   sentinel, `failures` for every other error. So `executions_cancelled` is what
   was actually stopped, over the whole union rather than the (a) subset.
   The snapshot is taken after the pause record is durable, so
   a run queued between snapshot and cancel is blocked at claim time by the
   run-processing gates and cleared by the next sweep (-003.6).
6. Return the active pause record plus the sweep counts.

The insert precedes the sweep so any caller reading the gate **after** the insert
is already blocked, which stops the sweep from cancelling a run while an ungated
loop queues another behind it.

Three residual windows remain, stated rather than claimed closed. They are
enumerated under [Failure and
recovery](workspace-kill-switch-02.md#residual-windows), beside the sweep policy
they follow from.

The sweep never aborts on one failure: each sub-step logs and continues, and the
response reports what was cancelled. The pause is in effect once the record is
inserted, so a partial sweep still stops the workspace and merely leaves
something for the next sweep. A sweep already in flight also runs to completion
when a resume commits underneath it — it is the tail of a pause that was valid
when it started, and aborting halfway would leave a partially-cancelled
workspace neither state explains. Cancelled runs are terminal either way: resume
does not revive them (-005.4) and the stale-claimed loop does not either, since
it only re-queues rows still `claimed`. The resumed workspace launches again on
the next event or tick; nothing re-queues swept work on an operator's behalf.

### Resume

1. Verify the workspace exists, as for pause.
2. Validate the release reason if supplied. It can be omitted or blank; when
   present it takes the same 500-code-point limit on the trimmed value, and an
   omitted or blank reason is stored as `''`, never whitespace (-004.8, -004.9).
3. Read the active record. If there is none, release nothing and return
   `paused: false`. The request still writes its activity log entry recording
   that it committed no release: -005.6 forbids creating or releasing a pause
   record, not writing the audit trail -005.8 requires of every request.
4. **In one transaction**, CAS-release the record and write the activity log
   entry, for the same reason pause pairs them. If the CAS reports no row
   changed, another resume won the race; commit this request's audit entry and
   return `paused: false`. If the transaction errors, return `500`; the workspace
   stays paused, the safe direction for a failed resume (-006.10).

Resume touches no agent, routine or trigger row, which is what makes it work. The
known per-agent resume defect comes from a control that must remember which rows
it changed on the way in; this design has no such set, because pause changed no
rows.

Failure and recovery, frontend state, persistence, security, observability,
testing and related decisions continue in [Part 2](workspace-kill-switch-02.md).
