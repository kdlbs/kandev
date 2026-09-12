---
status: active
system: office
created: 2026-09-08
owners:
  - kandev
---

# Office Routine Status Gating Requirements

## Overview

`office_routines.status` accepts `active`, `paused` and `archived`, and nothing that
fires a routine reads it. A scheduled routine dispatches on its cron slot whatever its
status, and so do the manual fire (`POST /api/v1/office/routines/:id/run`) and the
webhook fire (`POST /api/v1/office/routine-triggers/:publicId/fire`). An operator who
pauses a routine gets a persisted status, an "Off" badge, and a routine that keeps
launching agents.

This capability makes `status` load-bearing at every point a routine can fire. It is
the primitive an Office-wide kill switch needs, not that switch.

## Terminology

- **Slot:** one scheduled firing time from a cron expression.
- **Fire:** dispatching a routine run: an `office_routine_runs` row, then a wakeup
  request (lightweight routine) or a task (heavy routine).
- **Suppress:** decline a slot on status, leaving no run row, wakeup or task.
- **Cursor:** `office_routine_triggers.next_run_at`, the single persisted value
  deciding when a cron trigger is next due.
- **Firing status:** the statuses under which a routine may fire: `active` plus the
  empty string.
- **Outage catch-up:** the existing `catch_up_policy` / `catch_up_max` behavior
  replaying slots missed because the backend was down.

## Requirements

### REQ-OFFICE-ROUTINE-STATUS-001: A non-firing routine does not fire on its schedule

**Intent:** Pause is the operator's only means of stopping a routine short of
deleting it. It has to actually stop it, on every path, or nothing built on top of
it can be trusted.

**User story:** As an operator, I want a paused routine to stop launching agents, so
pausing is a real control and not a label.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-STATUS-001.1:** When a cron trigger is due and its routine's
  status is `paused`, the system shall not fire the slot.
- **AC-OFFICE-ROUTINE-STATUS-001.2:** When a cron trigger is due and its routine's
  status is `archived`, the system shall not fire the slot.
- **AC-OFFICE-ROUTINE-STATUS-001.3:** When a cron trigger is due and its routine's
  status is any value other than `active`, `paused`, `archived`, or the empty
  string, the system shall not fire the slot. Membership is decided on the stored
  bytes, with no case folding and no trimming, so `Active` and `" active"` do not fire.
- **AC-OFFICE-ROUTINE-STATUS-001.4:** When a cron trigger is due and its routine's
  status is `active` or the empty string, the system shall fire the slot with the
  same run row, wakeup payload, task creation, concurrency policy and
  outage-catch-up attribution it produces today.
- **AC-OFFICE-ROUTINE-STATUS-001.5:** When a slot is suppressed, the system shall
  create no `office_routine_runs` row for it, including no row with status
  `skipped` or `cancelled`.
- **AC-OFFICE-ROUTINE-STATUS-001.6:** When a slot is suppressed, the system shall
  create no agent wakeup request and no task.
- **AC-OFFICE-ROUTINE-STATUS-001.7:** When a slot is suppressed, the system shall
  leave the trigger's `last_fired_at` and the routine's `last_run_at` unchanged.
- **AC-OFFICE-ROUTINE-STATUS-001.8:** When a slot is suppressed and its cursor is
  advanced, the system shall record the routine id, trigger id, and observed status
  in a structured log entry. This entry is not deduplicated: it occurs at most once
  per scheduled slot, so its volume is the routine's own schedule. A suppression whose
  cursor could not be advanced is recorded under -002.8 instead, which -003.6 bounds.

### REQ-OFFICE-ROUTINE-STATUS-002: Suppression drops the slot and never banks it

**Intent:** A pause is an operator decision, not an outage. If suppressed slots
accumulated as missed ticks, resuming a routine, or releasing a kill switch, would
fire up to `catch_up_max` runs at once: the control would trade a runaway loop for a
burst.

**User story:** As an operator, I want resuming a paused routine to start it at its
next scheduled time, so the pause does not become a backlog that fires all at once.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-STATUS-002.1:** When a slot is suppressed, the system shall
  set the trigger's cursor to the first slot produced by its cron expression and
  timezone strictly after the evaluation time. The value written shall be a time
  that expression names; a computed time that does not match it shall be handled as
  a failure to compute (-002.8) rather than stored.
- **AC-OFFICE-ROUTINE-STATUS-002.2:** When a routine returns to a firing status, the
  first fire shall occur at the first slot due at or after the cursor left by the last
  suppressed evaluation, and no earlier.
- **AC-OFFICE-ROUTINE-STATUS-002.3:** When a routine is returned to a firing status
  and every one of its cron cursors is at a time in the future at the moment of the
  resume, the first fire shall report zero missed ticks, regardless of how long it was
  suppressed or what its `catch_up_policy` and `catch_up_max` are. Any suppressing
  evaluation leaves a cursor in the future (-002.1); one still in the past means no
  evaluation has run since the pause.
- **AC-OFFICE-ROUTINE-STATUS-002.4:** When the backend was not running for part of
  a suppression window, the first fire after the routine returns to a firing status
  shall report zero missed ticks for slots inside that window, under the same
  future-cursor condition as -002.3. One suppressing evaluation suffices however long
  the outage: the advance in -002.1 is measured from the evaluation time, not the old
  cursor, so it collapses the backlog rather than walking it.
- **AC-OFFICE-ROUTINE-STATUS-002.5:** When a routine holds a firing status, the
  system shall apply `catch_up_policy` and `catch_up_max` exactly as today, with no
  change to counts, cursor placement or reported missed ticks.
- **AC-OFFICE-ROUTINE-STATUS-002.6:** When two evaluations race on the same due
  slot of a routine that does not hold a firing status, the system shall advance
  the cursor at most once and shall fire nothing.
- **AC-OFFICE-ROUTINE-STATUS-002.7:** When an evaluation observes a cursor that
  another evaluation has already advanced, the system shall take no action for
  that slot.
- **AC-OFFICE-ROUTINE-STATUS-002.8:** When the new cursor cannot be computed or
  cannot be written, the system shall still not fire, shall leave the stored
  cursor unchanged, and shall record the failure in a structured log entry. The
  trigger remains due and is suppressed again next tick, subject to the repeat
  bound in -003.6. "Cannot be computed" covers a malformed expression, an unloadable
  timezone, and an expression naming no slot within the search horizon.
- **AC-OFFICE-ROUTINE-STATUS-002.10:** When two evaluations race on the same due
  slot of a routine that holds a firing status, exactly one shall fire it and the
  other shall take no action. Unchanged from today.

### REQ-OFFICE-ROUTINE-STATUS-003: The decision is made before the trigger is claimed

**Intent:** Claiming a trigger records a fire: it stamps `last_fired_at` and clears
the cursor. A suppressed slot must never leave that evidence, and a slot the system
could not decide must never be left un-armed.

**User story:** As an operator, I want a routine the scheduler could not evaluate to
be retried rather than silently retired, so a transient read failure does not
permanently kill a schedule.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-STATUS-003.1:** When a due trigger is evaluated, the system
  shall determine the routine's status before performing any state change that
  records a fire.
- **AC-OFFICE-ROUTINE-STATUS-003.2:** When the routine backing a due trigger
  cannot be read, the system shall not fire, shall leave the cursor unchanged so the
  trigger remains due, and shall log the error as observed, subject to the repeat
  bound in -003.6. The entry shall not assert the routine was deleted: the read path
  reports an absent row and a failed read identically.
- **AC-OFFICE-ROUTINE-STATUS-003.3:** When the routine backing a due trigger does
  not exist, the system shall not fire, shall leave the cursor unchanged, and shall
  not disarm, delete or otherwise modify the trigger, because the read that would
  justify doing so cannot distinguish a deleted routine from a failed read. Such a
  trigger is cleared by the existing orphan reconciler, not by this path.
- **AC-OFFICE-ROUTINE-STATUS-003.4:** A status write shall govern only slots
  evaluated after it commits: a slot already past the gate completes its fire even if
  the routine is paused immediately afterwards, and a slot already suppressed stays
  suppressed even if it is resumed immediately afterwards.
- **AC-OFFICE-ROUTINE-STATUS-003.5:** Evaluating one due trigger shall not change
  the outcome of any other due trigger in the same tick. For a fixed set of routine
  statuses, the set of slots fired and the set suppressed shall be identical under
  any evaluation order. A status write committing while a tick is in flight is not
  a fixed set: per AC-OFFICE-ROUTINE-STATUS-003.4 it may land on either side of any
  trigger's evaluation, and which triggers observe it is deliberately unspecified.
- **AC-OFFICE-ROUTINE-STATUS-003.6:** The two outcomes that leave a trigger due
  without advancing it, an unreadable routine (-003.2) and a cursor that could not be
  computed or written (-002.8), shall each be logged at most once per trigger per
  process. The comparison key is the trigger id and which of those two outcomes it
  is, and nothing else: no observed status, no error value. Both outcomes leave the
  trigger due, so it is re-evaluated every tick and an unbounded rule would write one
  entry every tick for the life of the process. The bound covers the logging only,
  not the repeated evaluation, and the state backing it is per-process, so a restart
  logs each outcome once more.

### REQ-OFFICE-ROUTINE-STATUS-004: Manual and webhook fires respect status

**Intent:** The cron path is the loudest hole, not the only one. A pause holding on
the schedule but not on "Run now" or an inbound webhook makes the operator work out
which door an unwanted run came through.

**User story:** As an operator, I want every way of firing a routine to respect its
status, so pausing means one thing everywhere.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-STATUS-004.1:** When a manual fire targets a routine without
  a firing status, the system shall refuse the request with HTTP 409 and shall
  create no run row, wakeup request, or task.
- **AC-OFFICE-ROUTINE-STATUS-004.2:** When a webhook fire targets a trigger whose
  routine has no firing status, the system shall refuse the request with HTTP 409
  and shall create no run row, wakeup request, or task.
- **AC-OFFICE-ROUTINE-STATUS-004.3:** When a webhook fire carries an invalid
  signature, the system shall refuse it on the signature, whatever the routine's
  status, so that the response does not disclose the status.
- **AC-OFFICE-ROUTINE-STATUS-004.4:** When a fire is refused on status, the
  response body shall name the observed status and shall carry a stable
  machine-readable code identifying a status refusal, distinct from every other
  refusal the same routes can return. The code is what lets a surface render its own
  localized message (-006.4, -006.6) instead of displaying a server string.
- **AC-OFFICE-ROUTINE-STATUS-004.5:** When a manual or webhook fire targets a
  routine with a firing status, the system shall behave exactly as it does today.
- **AC-OFFICE-ROUTINE-STATUS-004.6:** A refused fire shall leave the routine's
  `last_run_at` and every trigger's cursor and `last_fired_at` unchanged.
- **AC-OFFICE-ROUTINE-STATUS-004.7:** A manual or webhook fire shall read the
  routine's status once and that decision shall govern the whole request. A status
  write committing after the read shall neither cancel a dispatch already permitted
  nor rescue one already refused, and the system shall not re-read the status to
  narrow that window. With AC-OFFICE-ROUTINE-STATUS-003.4, which covers cron slots
  only, this makes all three fire paths decide once, at the gate.

### REQ-OFFICE-ROUTINE-STATUS-006: The routine surfaces do not promise a fire that will not happen

**Intent:** Suppression advances the cursor, so a paused routine keeps a populated
`next_run_at` that the list row renders as a countdown for a fire that never comes.

**User story:** As an operator, I want a paused routine's row to show no next fire,
so the screen matches what the scheduler will do.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-STATUS-006.1:** When a routine does not hold a firing
  status, neither the routines list nor the routine detail view shall render a
  next-fire countdown for it.
- **AC-OFFICE-ROUTINE-STATUS-006.2:** When a routine holds a firing status, both
  shall render the next-fire countdown exactly as they do today.
- **AC-OFFICE-ROUTINE-STATUS-006.3:** When a routine's status is `paused` or
  `archived`, the list row's enabled toggle and badge shall read as off.
- **AC-OFFICE-ROUTINE-STATUS-006.4:** When a fire is refused on status, the
  surface that requested it shall show a message naming the routine's status
  rather than a generic failure, selected from the code in -004.4 rather than from
  the response's human-readable string.
- **AC-OFFICE-ROUTINE-STATUS-006.5:** When a routine's status is the empty string,
  the list row shall read as on and render its next-fire countdown, matching the
  scheduler's treatment of that value. Reading a routine shall report the stored
  status verbatim rather than substituting a replacement, so the surface observes the
  value the scheduler observes.
- **AC-OFFICE-ROUTINE-STATUS-006.6:** Every string added by this capability and
  shown to an operator shall be localized through the existing i18n pipeline in all
  supported locales. A server-side message carried in a response body is a fallback
  for a surface that has no localized copy, not the localized copy itself.

## Out of scope

- **The Office-wide kill switch** (card `b73f82b6`). This supplies the per-routine
  primitive only: no workspace flag, no bulk status write, no single control. A
  follow-up needs to know only that `paused` now stops every fire path and resuming
  banks nothing.
- **Changing outage catch-up.** `catch_up_policy`, `catch_up_max` and their missed
  tick count belong to card `1a06962a`, constrained here one way only: a suppression
  window never contributes missed ticks.
- **Collapsing a stale cursor when a routine is resumed** (cut from this capability
  during spec review; a follow-up would own it). Suppression only advances a cursor
  when a tick actually runs, and `cron.Loop` does not tick at startup: its first tick
  fires one full `DefaultTickInterval` (30s) in, and its own comment tells handlers
  not to rely on a startup tick. So after every restart there is a window in which a
  routine paused before the restart still holds a cursor deep in the past. Resuming
  inside that window reaches the firing path with that cursor intact, and
  `computeRoutineMissed` walks the whole paused interval and reports up to
  `catch_up_max - 1` missed ticks on that one fire. The same applies to a status write
  that does not go through the Office API at all, such as a direct database edit.
  AC-OFFICE-ROUTINE-STATUS-002.3 and -002.4 are therefore conditioned on a suppressing
  evaluation having run. Accepted here: a stale missed count on exactly one fire, never
  a routine that stops firing. A follow-up that closes it should know three things.
  First, the repair must advance the cursor and never clear it, so that a skipped or
  failed repair degrades to this same stale count rather than to a routine that never
  fires again. Second, it must be best-effort and outside the status write's
  transaction, because a cursor write failure must never refuse an operator's resume.
  Third, a routine may hold several cron triggers on different expressions and
  timezones (nothing in the schema or the create path constrains it to one), so the
  repair is a loop of per-trigger compare-and-sets, each computing its own next slot;
  a single bulk `UPDATE ... WHERE next_run_at <= ?` would cross-assign one trigger's
  next-fire time onto another.
- **Validating the status value at the writer** (cut from this capability during spec
  review; a follow-up would own it). The gate reads an allowlist, so an unrecognized
  stored status stops a routine (-001.3). Rejecting unrecognized values on write would
  turn a typo into a 400 the operator sees rather than a routine that quietly stopped,
  and would make the value set closed rather than merely filtered. It is cut because
  it drags three further problems in with it, all of which a follow-up must solve
  together rather than one at a time. First, `PATCH /api/v1/office/routines/:id`
  resolves through a read-modify-write with no version column, and `UpdateRoutine`
  writes the whole row, so two concurrent callers lose each other's status writes;
  once status is load-bearing that lost update silently resumes a paused routine, and
  a validation AC that does not also state a concurrency rule does not deliver the
  guarantee it appears to. Second, `office/service/config_import.go` `applyRoutines`
  updates an existing routine through that same full-row write, reached from config
  sync, so it writes the status column too and preserves a pause only by echoing a
  fresh read; the other two bulk writers genuinely omit the column via
  `UpdateRoutineConfigFields`, so the three paths do not share one mechanism and need
  separate treatment. Third, once the endpoint rejects an out-of-set value, the detail
  view becomes a trap: it seeds its draft with `routine.status ?? "active"`, which does
  not catch the empty string, and submits `status` on every save, so a routine holding
  a value outside the set becomes uneditable in every other field. The fix belongs at
  the surface (omit an unchanged status from the update) rather than at the writer,
  because exempting the stored value at the writer would let any unrecognized value
  survive a round-trip, which is the trap the validation exists to close.
- **Stopping a run already in flight.** Pausing does not cancel a dispatched run;
  that is agent-level pause's contract (`guardAgentStatus`). The window in
  AC-OFFICE-ROUTINE-STATUS-003.4 and -004.7 is accepted: closing it needs a second
  gate in the wakeup dispatcher, which narrows it without eliminating it.
- **A force-run override.** "Run now" is refused, not read as a human override. The
  route back is to set `active`. An override may be added later, but must not arrive
  by omission, which is how this defect existed.
- **Disarming or deleting triggers on archive.** `archived` and `paused` suppress
  identically; archiving deletes no triggers or history, so un-archiving restores
  the schedule with no repair step.
- **Reaping the triggers of a deleted routine.** Per AC-OFFICE-ROUTINE-STATUS-003.3
  this capability neither deletes nor disarms them. Deleting a routine already
  cascades to its triggers wherever the database enforces foreign keys, so the orphan
  is rare; where one exists it costs one log entry per process (-003.6) plus one
  routine lookup per tick until the startup reconciler clears it. Rescheduling that
  reconciler, and giving the gate a read that tells a deleted routine from an
  unreadable one, both belong to that follow-up.
- **Ordering of due-trigger evaluation.** `GetDueTriggers` has no `ORDER BY` and this
  capability adds none, which is why AC-OFFICE-ROUTINE-STATUS-003.5 is written as
  independence between triggers rather than as a fixed order.
- **The manual fire route's error mapping beyond the status refusal.** Naming a
  routine that does not exist still returns 500, not 404. Pre-existing; only the
  status refusal and its code are added here.
- **Status-aware dashboard projections**, **bulk or scheduled status changes**, and
  **expvar counters for suppressed slots**. No status breakdown in rollups, no
  "pause all", no scheduled pause window, no auto-pause on failure; logs only.

## System design

The technical design is in
[Office Routine Status Gating System Design](../system-design/routine-status-gating.md).
