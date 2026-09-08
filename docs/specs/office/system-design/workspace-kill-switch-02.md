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

# Office Workspace Kill Switch System Design — Part 2

Continues [Part 1](workspace-kill-switch-01.md), which owns the surface decision,
the data and contracts, the gate points and the control flow. This part opens with
the failure and recovery policy those gates rely on, then covers how the capability
is stored, secured, observed and tested, and the frontend state machine that
renders it.

## Failure and recovery

The gate fails **closed**: a read error is treated as paused. The idle-skip gate
fails open, and the difference is intentional — failing open there means doing
probably-unnecessary work, here it means ignoring an operator's stop.

Failing closed must not destroy work, so **every** gate distinguishes a confirmed
pause from a failed read. The gate table's last column is that contract per site
and -002.9 is its requirement: a confirmed pause is a decision and may be
terminal; a read error is an absence of information and must leave the work in
its most retryable state. Four points the table cannot carry:

- **`HandleRunFailure` is explicitly not the requeue mechanism**, though the same
  function uses it a few lines above for a genuine error. It applies exponential
  backoff and after `MaxRetryCount` marks the run permanently failed, and a gate
  read error correlates with exactly the sustained database failure below, so
  reusing it would terminally fail the very runs -002.9 requires stay retryable.
  `RequeueClaimedRun` instead sets `status = 'queued', claimed_at = NULL` under a
  `status = 'claimed'` CAS, incrementing no `retry_count` and setting no
  `scheduled_retry_at`: a gate read failure is not a run failure, so the run is
  eligible on the next claim tick.
- If the requeue write itself fails — likely, since the same database failed the
  read — the run stays `claimed` and the age-based stale-claimed recovery
  re-queues it. That backstop is slower than the next tick and is the honest
  floor here.
- **The cron cursor is never restored.** The failing check is downstream of
  `UpdateTriggerNextRun`, so one tick is dropped rather than replayed as a burst,
  matching existing code that already treats an `UpdateTriggerNextRun` failure as
  non-fatal. The retryable unit is the fire: cron re-fires on its next cadence,
  and a webhook or manual caller gets `503` rather than the `409` of a confirmed
  pause, so a sender can retry an outage and not a stop.
- **The wakeup dispatcher** leaves a read error's request `queued`, inert rather
  than retried; see [Gate points](workspace-kill-switch-01.md#gate-points)
  for why nothing re-drives it.

Because the gate fails closed, a persistent database failure stops Office
launches for every workspace. That is the correct trade for a stop control, and
not a new exposure: the same failure already prevents claiming runs at all.

The halt sweep is best-effort by design (-003.7). The pause record, not the
sweep, is what makes the workspace stopped.

**A cancellation that found nothing to cancel is not a failed cancellation.**
`CancelTaskExecution` delegates to `StopByTaskID`, which returns
`ErrExecutionNotFound` both for a task with no live session and for a task whose
own session lookup failed. Idle tasks are *expected* members of the sweep's union:
`ListLiveRoutineTaskIDsForWorkspace` selects on non-terminal `tasks.state`, and a
heavy routine's task is non-terminal for as long as it sits on the board, live
agent or not. Counting that sentinel in `failures` would report a healthy pause of
a quiet workspace as a sweep that failed once per idle task, the opposite of what
-003.7 asks the count to mean: a *failed* cancellation is work that should have
stopped and did not, and an idle task has already stopped.

So the sentinel increments `executions_not_running`, and `failures` counts every
other error. Three consequences, stated because two of them are costs:

- **The count reports, it does not diagnose.** The same sentinel covers a failed
  session lookup, so a task that *is* live whose lookup failed lands in
  `executions_not_running` rather than `failures`, and the sweep does not stop it.
  That is a real hole in the halt guarantee and it is not closed here.
- **It is recorded rather than discarded, which is why the third count exists.** A
  pause reporting `executions_not_running` well above the workspace's idle-task
  count is the visible symptom. Per task, `ErrExecutionNotFound` logs at `DEBUG`,
  being expected; every other cancellation error logs at `WARN`.
- **Why the seam is not fixed instead.** Separating the two causes means changing
  `StopByTaskID`, an orchestrator function with callers outside Office, to return
  distinguishable errors. That is the same trade [Gate
  points](workspace-kill-switch-01.md#gate-points) refuses for `GetAgentFromConfig`,
  refused here for the same reason: a shared function's error contract is not this
  capability's to rewrite for a classification only this capability consumes. If a
  later change makes the two causes distinguishable, `failures` absorbs the lookup
  failure and this paragraph is amended.

### Residual windows

Three remain, stated rather than claimed closed.

**First**, a dispatcher that read the gate *before* the insert and writes its run
*after* the sweep passed that agent lands a queued run under a paused workspace.
Three things bound it: run processing gates twice on the same predicate, so the run
is never claimed into a launch; a repeat pause re-runs the sweep (-003.6); and the
sweep is idempotent, since the cancel writer only transitions runs still queued or
claimed and the checkout release is scoped to the runs it cancelled.

**Second**, between the final gate read and `si.launchAgent` there is an
instruction-level window; a pause committing inside it launches one agent whose run
was claimed *before* the pause existed, which -002.7 does not govern and -003.3's
execution cancellation covers.

**Third**, the sweep cancels by task id, and `StopByTaskID` enumerates that task's
live sessions when it is *called*, not when the snapshot was taken. A sweep still
running when a resume commits therefore cancels whatever is live at that moment,
which can be an execution launched *after* that resume — the launch -005.2
requires. This is the price of the rule that a sweep in flight runs to completion
rather than aborting, and it is bounded: one sweep's duration, only a task already
named in that sweep's union, and no repeat, since nothing re-cancels afterwards.
Closing it would need the cancel to carry an execution or session identity captured
at snapshot time, which `TaskCanceller` does not accept.

Closing the first two would need a conditional write on every launch path. This
design deliberately pays for none of the three.

## Frontend state

The store slice is hydrated by an explicit `GET .../pause` and re-read the same
way thereafter; pause state is never pushed. A client opening an already-paused
workspace was not watching when the pause was written, so the read on mount is
what makes AC-OFFICE-KILL-SWITCH-006.4 hold for exactly the operator arriving at
a stopped workspace.

The slice holds three things: the pause record or `null`; a **status** of
`unknown` or `known`; and one monotonic counter plus the sequence of the last
update applied. Issuing any request increments the counter and tags that request
with the new value. The tag exists to discard a **superseded** response:
responses arrive in whatever order the network delivers them, and without it a
slow early request would overwrite a fast later one.

The input set is enumerated and closed. Adding one rule per newly noticed input
is what left earlier revisions of this section incomplete, so an input that is
not in this table is a defect in the table, not a decision for the call site.

| Input | Effect |
| --- | --- |
| Mount, and every change of selected workspace | status `unknown`; **clear the record**; issue a read |
| WS reconnect | issue a read; status unchanged, the last known state still being the best available |
| Refresh control (-006.13) | issue a read; status unchanged until it answers |
| `GET` success | apply only if `workspace_id` matches the selected workspace **and** the tag exceeds last-applied; then status `known` |
| `GET` failure | status `unknown`; the record is **not** cleared, so a pause already read stays on screen |
| `POST` pause or resume success | apply under **both** conditions of the `GET` rule, the `workspace_id` match and the tag; then status `known` |
| `POST` pause or resume failure | status and record unchanged; surface the failure (-006.6) |

Four consequences, each a rule a builder would otherwise have to invent:

- **The record is cleared when the selected workspace changes**, so it can never
  describe a workspace the operator is no longer looking at. Mount and a workspace
  change share a row because they are the same situation: nothing known about the
  newly selected workspace yet. Retaining the record across a switch would render
  workspace A's banner over workspace B until B's read landed, and indefinitely if
  that read failed, since a failed `GET` deliberately keeps whatever record is
  held. That is -006.4 inverted: a *running* workspace showing a *stopped* banner.
  The rule is stated because the store slice has in-repo precedent for both a
  workspace-keyed and a flat shape, so neither is inferable. Clearing also means
  the "pause state unavailable" affordance appears briefly on a workspace switch
  exactly as it does on mount, which is deliberate rather than a regression:
  during that window the client genuinely does not know, and saying so is truthful
  where showing nothing is not.

- **A mutation response is checked for workspace, not only for order.** Both
  halves of the `GET` rule apply to it. The workspace half is the one that is easy
  to drop, because a client "knows" which workspace it just posted to, but it
  posted to the workspace selected *then* and the response can land after the
  operator has moved on, which is why every response body carries `workspace_id`
  at all. Nothing is applied optimistically before the response, so -006.6's "keep
  the displayed state matching the server's" holds by construction rather than by
  rollback.
- **`unknown` is a rendered state, not a hidden one.** While `unknown` with no
  record, the banner is absent and a "pause state unavailable" affordance with a
  retry is shown in its place, so the absence of a banner is never read as a
  running workspace, the -006.4 failure that matters most, since it is exactly an
  operator arriving at a stopped workspace during a database problem. While
  `unknown` with a record already read, the banner stays and is marked stale. The
  pause control stays reachable in every state: an operator must be able to stop a
  workspace whose state could not be read.
- **The displayed state can be stale, and the refresh is the correction.** A pause
  written by another operator reaches an already-open page on its next read, not
  immediately; and two reads issued close together settle on the earlier one when
  the later one answers first, because the tag orders requests by when they were
  issued rather than by what the server held when it answered each. Both are the
  same residual, and -006.13's refresh answers both: it issues a read whose tag is
  newer than every request in flight, so its response always applies. Closing the
  residual outright would need a server-side revision on the response body, which
  no Office payload carries; see [Deferred: live pause-state
  updates](#deferred-live-pause-state-updates).

## Persistence

The table is created in the Office repository's base schema. Office's workspace
deletion path must delete its rows for the workspace and the deletion test's
table list must include it; both sibling workspace tables are handled there.

`office_routine_runs` is an existing table, so its two new columns and their
partial unique index are added in `runMigrations()`, and the index must be
registered **after** the `ADD COLUMN` statements: schema init runs before
migrations, and an index over a not-yet-added column crashes every existing
database on boot.

Both columns must also be added to `models.RoutineRun` in the same change. The
two routine-run list queries scan `SELECT *` into that struct, and sqlx fails a
scan on any column with no matching field, so a migration that lands without the
struct change does not degrade reading routine runs, it breaks it. Giving the two
fields `json:` tags is also the whole of the transport work: the list response
serializes that struct, so the attribution reaches the client once it is stored.

That is as far as this design goes, and the stopping point is deliberate.
Rendering the attribution is out of scope in the requirements, so what a
follow-up needs is recorded rather than built: a label for the skip reason, and a
lookup from the pause id to that pause's reason and actor. The read endpoint
resolves that lookup only while the pause is **active**; a run skipped by a pause
that has since been released carries an id no endpoint here dereferences. A
follow-up rendering historical attribution therefore needs a fetch-by-id this
design does not add. It is not free, and it is not smuggled in as free.

State survives restart because it is a row, not process state. There is no TTL
and no scheduled release; -005.1 requires that nothing but an explicit resume
clears it. Released records are retained; no pruning is specified.

## Security

Both mutations are workspace-scoped through the existing Office middleware, which
authorizes `:wsId` for browser callers and matches the JWT workspace claim for
agent callers.

Agent callers must not pause or resume: an agent that could stop its own
workspace could also stop an operator's intervention, and one that could resume
could undo it. The handlers reject an agent caller with `403`, reusing the
agent-caller resolution the Office agents handler already performs.

The reason string is operator-supplied and rendered in the UI. It is stored
verbatim and escaped at render, never interpolated into a prompt.

## Observability

- Structured logs at each gate site naming the workspace, the gate and the
  blocked action; and on pause and resume naming actor, reason and sweep counts,
  which include `executions_not_running` beside `failures`.
- `expvar` counters under `office_pause_*`: `office_pause_created_total`,
  `office_pause_released_total`, `office_pause_blocked_total` labelled by gate,
  `office_pause_gate_error_total`. The per-gate label makes a coverage gap visible: a gate that never increments while
  a workspace is paused and active is either unreachable or unwired.
- Activity log entries use `target_type = "workspace"`, the workspace id as
  `target_id`, and three actions so a rejected request stays auditable:
  `workspace_paused`, `workspace_resumed`, and `workspace_pause_noop` for a
  request that committed neither. `details` carries the reason, and for the no-op
  which operation was requested and why it committed nothing (already paused, not
  paused, or lost a race). This is what the workspace activity page shows.

## Testing

Backend, in `*_test.go` beside each source:

- one test per gate-table row: the blocked outcome, and the originating write
  still succeeding where applicable. The two run-queue-write rows get
  **independent** tests — exercising `Service.QueueRun` proves nothing about
  `SchedulerService.QueueRun`, so reach that one through `QueueRunCtx` as
  reactivity and the approval adapter do;
- a gate-error test per row, asserting the retryable state that row promises: run
  back to `queued` with `retry_count` unchanged, no routine run row written and
  the handler answering `503`, wakeup request still `queued`;
- each derivation site resolves the workspace it should — `routine.WorkspaceID`,
  the widened `guardAgentStatus` (both copies), `AgentReader` for wakeup;
- -002.10 at the **wakeup** site as a branch: `ErrAgentNotFound` leaves launch
  behavior unchanged — an unattributed agent's wakeup still creates its run, and
  is not left `queued` — while a non-sentinel lookup error there fails closed. A
  deleted agent takes the not-found branch too;
- -002.10 at the **two run-queue writers** as a single outcome rather than a
  branch, because no sentinel reaches them: an unattributed agent, a deleted
  agent and a failing lookup each create no run row and each leave the
  originating write committed. Assert that convergence directly — a test that
  expected the three to differ would be asserting a classification this design
  states those sites cannot make;
- the skipped-run insert failing for a reason other than the unique index: the
  fire is still blocked, a webhook still gets `409`, and cron still returns `nil`;
- a typed paused error reaching `reactivity` and the approval adapter logs at
  `DEBUG`, while a genuine `QueueRunCtx` failure still logs at its present level;
- a pause whose retry loses to a concurrent **pause** returns that record and not
  `409`, distinguishing it from the pause-resume-pause sequence that does;
- a run blocked at the **final** run-processing gate launches no agent and has
  its checkout released — pause after the early read has passed, so the early
  gate alone cannot satisfy it;
- a **cron** fire blocked by a pause writes a `skipped` run carrying
  `skip_reason = 'workspace_paused'` and the blocking `pause_id`, identically to
  a webhook fire, and `TickScheduledTriggers` logs no error for it — while a gate
  *read* error on the same tick does;
- a second blocked fire of the same routine under the same pause writes no row of
  any kind, while one under a *different* pause record does (-002.13, and proof
  the insert is keyed on the pair);
- a pause-skipped routine run is distinguishable from a skip-if-active row;
- two pauses resolve to one record; two resumes release once; a pause whose
  insert loses to a concurrent resume retries once and succeeds, a second loss
  returning `409` rather than looping — and that `409` carrying `"paused": false`
  with no `reason`, distinguishing it from a blocked-caller `409`;
- cron cursor advance while paused, and no burst on resume;
- a failed activity-log write rolls the pause insert back (not paused, `500`, no
  orphan entry), same for release; a resume on an unpaused workspace releases
  nothing and still writes its activity log entry;
- a plain **repeat pause**, an already-paused workspace with no concurrent
  writer, returns the existing record unchanged *and* commits a
  `workspace_pause_noop` entry naming the requested operation, so -005.8's "every
  request auditable" covers the pressed-twice case and not only the raced one;
- halt sweep: taskless runs cancelled, checkouts released, terminal runs
  untouched, an ordinary non-Office task in the same workspace left running,
  partial failure still reporting paused with a non-zero failure count;
- halt sweep, **idle task in the union**: a task whose cancellation returns
  `ErrExecutionNotFound` increments `executions_not_running` and leaves `failures`
  at zero, so a healthy pause of a quiet workspace reports no failures. Assert the
  two counts separately — a test asserting only `failures == 0` passes with the
  sentinel silently discarded, which is the outcome this design rejected;
- halt sweep, **heavy path**: a live routine task with no `runs` row has its
  execution cancelled, a finished one is not in the set, and a task both sources
  name is cancelled once;
- resume effectiveness: a run queued after resume launches, while the same wake
  for a *budget-paused* agent launches nothing (-005.2 and -005.5);
- a wake reason named nowhere in this spec is gated too, asserting the chokepoint
  is reason-agnostic rather than an enumeration;
- a 500-CJK-character reason accepted, a 501-code-point one rejected;
- the actor sentinel recorded, and an unknown workspace id returning `404` from
  **all three** endpoints — the `GET` included, which is the one that would
  otherwise answer `200 {"paused": false}` — every case with authentication
  disabled, so the pass-through middleware is exercised rather than bypassed; a
  failed release leaving the workspace paused.

Frontend: one test per row of the `## Frontend state` input table, since that
table is the contract. Specifically: hydration on mount and on workspace change; a
workspace change **clearing** the record, asserted by switching from a paused
workspace to a running one whose read has not yet answered and finding no banner;
a `GET` response naming another workspace discarded, and a lower-sequence one
discarded; a **`POST` success naming another workspace** discarded, kept as a
test distinct from the `GET` case because it is the check most easily dropped,
and a lower-sequence `POST` success discarded; a **failed `GET`** leaving an
already-read record on screen under status `unknown`; a failed `GET` with no
record rendering the unavailable affordance rather than nothing, with the pause
control still reachable; and the refresh control's response applying over an
older read still in flight.

E2E: Office is on in the `e2e` profile. One Playwright spec covering pause from
the banner, the banner across an Office navigation, a blocked launch, and resume,
on desktop and phone viewports.

## Deferred: live pause-state updates

A workspace-scoped `office.workspace.pause_changed` event was specified and then
cut; the requirements record the cut under `## Out of scope`. What a follow-up
would need, so that this is a deferral and not a loss:

- the event name in the Office `events.*` constants, a matching `ws.Action*`, and
  registration in `RegisterOfficeNotifications`, which is where an Office event
  becomes a client-visible frame;
- a payload of `{ workspace_id, paused, pause }` mirroring the `GET` body, so the
  slice applies a push and a read through one path. `workspaceForEvent` already
  prefers an explicit payload `workspace_id`, so routing needs no change;
- an answer to ordering, which is why this is deferred rather than small. Office
  publishes post-commit and best-effort, so a pause and a resume committing close
  together can be delivered out of commit order and latch the banner to the losing
  state. No Office WS payload carries a sequence, version or revision field, so
  the slice has nothing to arbitrate with, and adding one is a change to the event
  contract rather than to this feature;
- an answer to a dropped publish, which best-effort delivery permits and which no
  amount of ordering fixes.

Until then the read path plus -006.13's refresh is the whole convergence story,
and it is closed on its own terms: every state the server holds is reachable by
an operator action that cannot be reordered against itself.

## Related decisions

No ADR is required: this adds an Office-owned table and gates behind an existing
pattern, establishing no repo-wide boundary. Generalizing the fail-closed gate
policy beyond Office later would warrant one.
