---
status: current
system: tasks
requirements:
  - REQ-TASKS-IDLE-QUEUE-DRAIN-001
created: 2026-10-20
owners:
  - kandev
---

# Idle Queue Drain Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1, .2   | `orchestrator.Service.drainIdleQueues`, called from `ceilingSweepTick`, delegating to `DrainQueuedMessageIfAutoRun` |
| .3       | `idleQueueDrainBackoff` in `orchestrator/queue_idle_drain.go` |
| .4       | `messagequeue.Service.ListSessionsWithPending` (SQLite join on `task_sessions`) |

## Sweep pass

Each ceiling sweep tick, after deferred ceiling launches, lists sessions with
at least one queued row that is not reserved in flight. For each session whose
backoff is due it runs the body of `DrainQueuedMessageIfAutoRun`, which already
reads the Auto-run policy, rejects non-promptable sessions, and checks the
cancel/in-flight guards; the dispatch then runs through the ordinary
queued-dispatch path and its recovery-block check. The pass never sets Auto-run.
It takes the session's cancel-in-flight guard with `TryLock`: a held guard means
a cancel, steer or dispatch owns the session, so the pass skips it without
recording an attempt and stops when its context is cancelled.

## Backoff

The backoff is in memory and per session: the first attempt is immediate, then
one, two, four, eight, and fifteen minutes. Sessions missing from a listing
with no queued dispatch in flight are forgotten, so a session that drains and
later queues again starts fresh. A session whose dispatch is still in flight is
kept: its reserved message is absent from the listing but may be returned to
the queue, and must then wait out the delay. A
restart clears the state, which is safe because the next listing re-derives it.

## Listing

`ListSessionsWithPending` is an optional repository capability implemented by
the memory and SQLite repositories. The SQLite query joins `task_sessions` so
rows left by a deleted session are not offered, and skips rows reserved by an
in-flight dispatch. It reads only `session_id` and `metadata_json`, so message
bodies and attachments are not loaded on each pass.

## Verification

Orchestrator tests cover dispatch from an idle session, a paused Auto-run, a
running session, the backoff schedule, a backoff kept across an in-flight
dispatch, and a session skipped behind a held guard. Messagequeue tests cover the listing
on both repositories and the deleted-session exclusion.
