---
id: coordinator-wake-recording-design
title: Recording wakes design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-WAKE-001
  - REQ-COORDINATOR-WAKE-002
---

# Recording wakes System Design

## Purpose and boundaries

This design owns how an episode on a coordinator's own task becomes one
`coordinator_wakes` row: which tasks are own tasks, how the recorder reacts to
events, how `RecordWake` stores a row, and how an episode key is derived. The
wake tables, the [wake lock](wake.md#wake-lock), the [backstop](wake.md#backstop)
that repeats this recording every 60 seconds, admission and delivery stay in
[wake](wake.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-001` | [Own tasks](#own-tasks), [Recorder](#recorder), [Episode keys](#episode-keys) |
| `REQ-COORDINATOR-WAKE-002` | The backstop calls the same `RecordWake` and key readers ([wake](wake.md#backstop)) |

## Own tasks

`ListOwnTasks(ctx, coordinatorID)` selects `DISTINCT` tasks from
`coordinator_proposals` where `coordinator_id = ?`, `kind = 'create_task'`,
`status = 'approved'` and `task_id IS NOT NULL`, joins `tasks` on id, and keeps
rows with `archived_at IS NULL`, `is_ephemeral = 0` (an integer column on both
dialects; never a boolean literal, which PostgreSQL rejects) and a task id that is
not the coordinator's `conversation_task_id` (`conversation_task_id` null keeps
every row). It returns `task_id`, `workspace_id` and `workflow_id` (`COALESCE(workflow_id,
'')`, and the empty id is in no watch set), ordered by task id. The `kind` predicate matters: `CompleteKindTx` writes the target
task into `task_id` for `message`, `move` and `resume` proposals too, and those
tasks are not own tasks. The reverse lookup the recorder needs,
`CoordinatorsOwningTask(ctx, taskID)`, uses the same predicates plus
`coordinators.autonomy_enabled = 1`, returns one row per coordinator
(`DISTINCT` coordinator id, ordered by coordinator id), and returns a list
that is normally one entry (a task is created by one `create_task` proposal);
the recorder loops over it. Every caller keeps only tasks inside the
coordinator's effective watch set
([integration](integration.md#watch-set)), read once per pass or event. The
conversation-task exclusion lives only in these two queries, so the recorder
and the backstop inherit it and neither repeats it.

## Recorder

`internal/coordinator/wake_recorder.go` subscribes, only while phase 3 is
effective. The event only says *where to look*: the recorder never takes an
episode key, a session state or a pending action from the payload except the
task and session ids.

| Event | Kind | Episode key source (stored state) |
| --- | --- | --- |
| `session.pending_action_changed` with `pending_action` `clarification` | `question` | the primary session's pending clarification bundle: its `pending_id` |
| `session.pending_action_changed` with `pending_action` `permission` | `permission` | each pending permission message on the primary session: its `pending_id`, one wake per pending request, in message `created_at`, `id` order |
| `coordinator_stalls` upsert (hooked in `stalls.go` after `UpsertStall` returns true) | `stall` | the stored row's `last_event_at` (the hook re-reads the row, it does not use the event's value), only while the row is [current](wake-backstop.md#stall-currency) |
| `task_session.error_changed` with `active: true` | `error` | the primary session's stored active error `stamp` |
| `task.state_changed` with `state` `COMPLETED` | `completed` | `completed`, when the stored task state is `COMPLETED` |

The recorder and the backstop read task, session and message state only through
the narrow `WakeSources` interface, defined in the coordinator package and
implemented in `internal/backendapp` over the task repository (the coordinator
package imports no task-repository type):

```go
type WakeSources interface {
    PrimarySessionID(ctx, taskID) (sessionID string, err error)   // "" when the task has no primary session
    PendingQuestionID(ctx, sessionID) (string, error)             // "" when none
    PendingPermissionIDs(ctx, sessionID) ([]string, error)        // message created_at, id order; empty when none
    ActiveErrorStamp(ctx, sessionID) (string, error)              // "" when no active error
    TaskState(ctx, taskID) (string, error)
    LastActivityAt(ctx, taskID) (*time.Time, error)               // nil when the task's status summary has none
}
```

A task with no primary session has no question, permission or error episode;
its stall and completed episodes still apply. A read error from any method is a
read error, never "absent": `LastActivityAt` failing is not a current stall.
The one exception is `Store.GetStall`, which returns `ErrNotFound` when the task
has no stall row: that is "no stall episode", not a read error (any other
`GetStall` error is one).

The recorder owns its subscriptions through the `Service`, because the shared
registration signature carries no `routeParams`: the body of
`registerCoordinatorWake` subscribes synchronously (only when phase 3 is
effective) and hands the subscriptions to the `Service`;
`Service.StopWakeRecorder()` (idempotent) unsubscribes them and waits for
handlers in flight, and `registerCoordinatorRoutes` registers it through
`p.addCleanup` beside `StopWakeBackstop`, so both end before the store closes
(cleanups run in reverse registration order and the pool's was registered
earlier). `StopWakeRecorder` also clears the stall hook under the same mutex and
waits for hook calls in flight, so no hook call reaches the store after the
cleanup returns; a hook call that starts after it is a no-op. A handler that runs during shutdown fails its reads and drops. If one subscribe fails at registration, the error is logged at warn, the subscriptions already made are kept and the backstop covers the missing event source. An event missing its task or
session id is dropped at debug with no metric. The stall hook is not a bus
subscription: `Service.SetStallWakeHook(func(ctx, workspaceID, taskID))` is
nil by default, is set only by `registerCoordinatorWake` when phase 3 is
effective, and `stallSubscriber` calls it (nil is a no-op, a panic is
recovered and logged; it runs on the subscriber's goroutine, so a slow hook
delays only the next stall event) after `UpsertStall` returned true, so with phase 3 off
a stored `autonomy_enabled = 1` records nothing (`AC-COORDINATOR-WAKE-004.4`).

For each event the recorder resolves `CoordinatorsOwningTask`, reads the watch
set once per owning coordinator and keeps the coordinators whose set holds the
task's workflow, skips a session that is not the task's primary session,
re-reads the condition from stored state, and when it yields a non-empty key
calls `RecordWake`. A failing `CoordinatorsOwningTask`, a watch-set read error, or a
`WakeSources` read error drops the event (that coordinator's part of it), logs
at warn and counts `coordinator_wake_dropped_total{reason="read_error"}`; a
watch-set `ErrNotFound` (coordinator deleted meanwhile) counts `not_found`. A
`pending_action_changed` event names one action, and the task service publishes
it only when the action value changes: a question waiting behind a permission,
or a second permission arriving while one is pending, has no event of its own
and is recorded by the next event or the backstop. The primary-session
check and the episode reads are separate reads: if the primary session changes
between them, the wake may name a session that is no longer primary, and
delivery step 2's recheck supersedes it.

`RecordWake(ctx, coordinatorID, taskID, kind, episodeKey)` returns a result
carrying the outcome, one of `inserted`, `exists`, `capped`, `autonomy_off`,
`not_own`, or an error, and the coordinator's `workspace_id` read from the
locked row. Before the lock it rejects an empty coordinator or task id, a `kind` outside
the five kinds, and an empty or whitespace-only key with an error that stores
nothing and counts `write_error`; the recorder and the backstop never pass
such input, so this guards other callers. It runs `Store.WithWakeLock` and,
inside `fn` and in this order:
reads the locked coordinator row and returns `autonomy_off` (inserting
nothing) when `autonomy_enabled` is not 1; re-runs the own-task predicates of
[Own tasks](#own-tasks) for `(coordinatorID, taskID)` and returns `not_own`
when they fail; looks up a row with the same `(coordinator_id, task_id, kind,
episode_key)` in any status and returns `exists` when one is there (a
`superseded` or `delivered` row is never re-opened and never counted as capped);
counts the coordinator's `pending` rows and, at 200 or more, returns `capped`;
otherwise runs `INSERT ... ON CONFLICT (coordinator_id, task_id, kind,
episode_key) DO NOTHING` and returns `inserted` when it wrote a row and
`exists` when it did not. Because the autonomy read, the count and the insert
run under the lock, an autonomy-off PATCH that committed its supersede first
makes `RecordWake` insert nothing, and concurrent recording never passes 200;
only wakes returned to `pending` by a failed or interrupted turn can, and they
are not inserts (`AC-COORDINATOR-WAKE-001.4`). The watch set is not re-read
inside the transaction: a workflow removed from the set in that window yields
a wake that delivery step 2 supersedes. A failure to read `tasks` inside the
transaction is a plain error (`write_error`). A cancelled `ctx` returns its
error and is not counted or logged above debug: it only happens at shutdown.

After the transaction outcome is known:

| Outcome | Metric | Then |
| --- | --- | --- |
| `inserted` (committed) | `coordinator_wake_recorded_total{kind}` +1 | publish `coordinator.updated{autonomy_changed: true}`, then `Kick(coordinatorID)` |
| `exists` | none | none |
| `capped` | `coordinator_wake_dropped_total{reason="cap"}` +1 | none |
| `autonomy_off` | `..._dropped_total{reason="autonomy_off"}` +1 | none |
| `not_own` | `..._dropped_total{reason="not_own"}` +1 | none |
| `ErrNotFound` (coordinator deleted) | `..._dropped_total{reason="not_found"}` +1 | none |
| any other error, or a commit failure | `..._dropped_total{reason="write_error"}` +1, warn log | none |

The metric, the publish and `Kick` come only after the commit succeeded, in that order. Task 04 owns the wake-insert publish: it calls
`Service.publishCoordinatorUpdatedWith(ctx, workspaceID, coordinatorID, true)`
with the workspace id from the result (that helper reads the open-proposal count itself and is best effort); delivery, turn settle and ceiling changes publish from task 05. The
`reason` label set is closed: `cap`, `autonomy_off`, `not_own`, `not_found`,
`read_error`, `write_error`. A refusal is not an error to the caller: the
recorder logs and the backstop recovers it. The backstop counts a `capped`
outcome on every pass that meets it. Freed slots go to the kept episodes in
the order of the read phase (task id, then `question`, `permission`, `stall`,
`error`, `completed`), so an episode on a high task id can wait behind lower
ones until enough slots free; the order is deterministic and
`AC-COORDINATOR-WAKE-002.2` carves out this case.

`Kick` is the `Service.SetKick` seam task 01 already ships (the autonomy PATCH
calls it). `SetKick` and `SetStallWakeHook` may be called while recorder
goroutines run, so both store their function under the `Service`'s mutex (or an
atomic value) and every reader takes it; `-race` tests set them mid-run. Task 05 is the only task that sets it; this card only calls it. A `Service` method shared by the PATCH, the recorder
and the backstop calls it: nil-safe (no call when unset, which is every case
until task 05 sets it), a panic recovered and logged, an error logged and
ignored. This card adds no second Kick interface.

### Episode keys

The key is always a value read back from storage, so a restart, a redelivered
event and a backstop pass produce byte-identical keys on the install's dialect
(PostgreSQL `TIMESTAMP` keeps microseconds, an event payload carries
nanoseconds; a key is stable per dialect and is never compared across them):

- `question`, `permission`: the stored `pending_id`, verbatim.
- `error`: the stored active error `stamp`, verbatim (the payload's `stamp` is
  never used).
- `stall`: the `last_event_at` of the row read back with `GetStall`, as
  `t.UTC().Format("2006-01-02T15:04:05.000000000Z")` (fixed width, nine
  fraction digits, never `time.RFC3339Nano`, which trims trailing zeros).
- `completed`: the literal `completed`.

An empty or whitespace-only `pending_id` or `stamp` is not an episode: no wake,
no metric, a debug log; the next backstop pass tries again.

## Backstop recording

For one coordinator whose re-read row has autonomy on, the backstop's wake
duty is two phases:

1. **Read.** `ListOwnTasks`, the watch set, and one read-only
   `ExistingWakeKeys(ctx, coordinatorID, taskIDs)` returning the `(task_id, kind,
   episode_key)` of every stored wake in any status for the watched own-task ids
   (an empty list reads nothing). For each watched own task,
   in `ListOwnTasks` order, it reads the episodes the way the recorder does, per
   task in the order `question`, `permission` (message `created_at`, `id`),
   `stall`, `error`, `completed`, and keeps each episode with a non-empty key
   that is not in the existing set. A `GetStall` `ErrNotFound` is no stall
   episode; any other read error abandons the coordinator for the pass
   ([wake](wake.md#backstop) step 3.1).
2. **Record.** It calls `RecordWake` for each kept episode in that order. Each
   call is independent: an error is logged and counted per the outcome table
   and does not stop the rest, and wakes already stored stay stored. So
   `AC-COORDINATOR-WAKE-002.3` binds the read phase: a read error before
   recording stores nothing from the pass's reads, while a failure inside a
   `RecordWake` transaction (including its own-task re-read) affects only that
   episode.

The pre-filter means a steady state (every episode already stored, such as a
completed own task) costs no write transaction per pass. `capped` is counted
once per kept episode per pass that meets it. The pre-filter is an
optimization only: `RecordWake` re-checks existence under the lock, so a wake
stored between the read and the record yields `exists`.

## Retention and own tasks

A delivered or superseded wake is the only record that an episode was handled,
and the backstop re-reads every current episode. `PruneWakeState` therefore
deletes a `delivered` or `superseded` wake older than 30 days only when its
task cannot become an own task again: no approved `create_task` proposal of
the coordinator names it, or its `tasks` row has `is_ephemeral = 1`, or the
row is gone. An archived task keeps its wakes, because unarchiving returns it
to the [Own tasks](#own-tasks) set and the backstop would otherwise store its
`completed` episode a second time; the cost is bounded by one row per episode
per task the coordinator created. Leaving the watch set and the conversation
task do not release a wake either (a wake never exists for the latter). A
live own task keeps at most one row per episode for its life, so a completed
task's `completed` wake is not re-recorded 30 days later. `pending` wakes and
open turns are still never deleted, and the pass stays idempotent for one
`now`. Task 04 changes this statement in `store_wake.go` and its test, with a
test that archives a task, prunes with `now` 31 days later, unarchives it and
asserts the backstop stores no second wake; the SQL is one `NOT EXISTS`
subquery plus one task-row test, valid on both dialects.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
