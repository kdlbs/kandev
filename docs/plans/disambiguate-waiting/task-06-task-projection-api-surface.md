# Task 06: Task-level projection, revision epoch, API surface publish rules

Spec: §"Data model → Task-level projection", §"Revision epoch", §"API surface".
ACs: AC-22, AC-36, AC-38, AC-39, AC-49, AC-50, AC-62, AC-77, AC-78.
Round-5 F4 and F5 dispositions live here (see plan.md).

## Task-level projection

`parked_on_background_work` at task level = OR across the task's sessions.
`parked_revision` is the task's **own** monotonic `uint64`, incremented once
per observed change of the task-level boolean — **never** `max()` of member
sessions' revisions (that rule is proven defective in the spec; do not
reintroduce it). Boolean + counter read in one critical section across all of
the task's sessions.

## Revision epoch

`parked_epoch` = backend process start time, Unix nanoseconds, identical on
every carrier, fixed for process life. Consumer discard rule: lexicographic
`(parked_epoch, revision)`, strictly higher epoch always wins regardless of
revision; within an epoch, discard a strictly lower revision.

**F4 disposition (Build decision, see plan.md):** the boot payload is the sole
reset-delivery mechanism. Do not build a new "reset broadcast on WS reconnect"
mechanism — none exists in this codebase and inventing one is out of this
feature's scope. AC-77's WS-reconnect-without-reboot clause is accepted as an
unclosed gap; record it again here in the task's own test file comments so a
future reader does not assume it works.

**F5 disposition:** keep epoch as Unix-nanosecond process start time as
specified; do not add a persisted monotonic counter (would require a schema
migration, which this feature explicitly excludes). Accepted risk: an NTP
step / restored snapshot / host migration could produce an equal-or-lower
epoch across a restart, in which case clients would (incorrectly) discard the
new process's frames. Out of scope to fix.

## Publish rules

- A transition of `parked_on_background_work` in either direction, via any of
  the three terms, publishes `session.activity_changed` for that session
  (new value, `revision`, `parked_epoch`).
- `task.updated` publishes **only if** the task-level OR also changed
  (AC-78: two sessions, one already parked, the second parking too → only
  `session.activity_changed` for the second, no `task.updated`, task's
  `parked_revision` unchanged).
- DTO fields: `parked_on_background_work` (bool, default `false`),
  `revision`/`parked_revision` (uint64, default `0`), `parked_epoch` (uint64,
  default `0` — sorts below every real epoch, per D9). Follow the
  `CancellationPending`/`EnrichCancellationPending` snapshot-provider pattern
  (`internal/task/dto/cancellation_pending.go`) exactly: a type-asserted
  optional upgrade that degrades gracefully.
- Session and task carriers, and the boot payload's session/task records, all
  gain the three fields.

## Tests

- AC-22: task DTO reflects session's parked value; `foreground_activity`
  unchanged.
- AC-36: backend restart (simulated in test by constructing a fresh service)
  → boot payload reports `false`.
- AC-38: `false → true → false` transitions: revision strictly increases
  across both, re-deriving an unchanged value publishes nothing.
- AC-39: a stale `(E, N-1)` update is discarded, displayed value unchanged.
- AC-49: two-session task, S1 already toggled twice (revision 2, false), S2
  never transitioned (revision 0, false); S2 parks → task's `parked_revision`
  strictly greater than its prior value (not `max(2, 1) == 2`); a
  lower-revision-at-same-epoch task update is discarded.
- AC-50: task with no sessions, or no transitioning session → `false`,
  revision `0`.
- AC-62: `live → settled` publishes `session.activity_changed` (false, higher
  revision) AND `task.updated` (OR changed).
- AC-77: consumer applying `(E1, 7)`, backend "restarts" (fresh process
  state in test) and publishes `(E2, 0)` with `E2 > E1` → consumer applies it;
  boot payload resets the consumer's applied-revision map for every session/task
  in it.
- AC-78: as above, no `task.updated` for the second session's parking.
