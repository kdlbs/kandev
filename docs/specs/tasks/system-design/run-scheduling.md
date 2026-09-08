---
status: current
system: tasks
requirements:
  - REQ-TASKS-RUN-SCHEDULING-001
created: 2026-08-24
owners:
  - cfl
---

# Queued Run Scheduling System Design

## Context and boundaries

The task system owns the persisted runs queue and its single backend-wide
consumer. Office supplies some run producers and Office-only maintenance, but
does not own a scheduler per workspace.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-RUN-SCHEDULING-001 | Queue ownership, classification, lifecycle, shutdown, recovery |

## Queue ownership and classification

The existing `runs` table is the durable queue. One scheduler claims work for all
workspaces in global request order while preserving the existing per-agent
serialization rule. Explicit `queue_run` workflow actions may enqueue from any
workflow style.
Interactive user launches stay on the interactive orchestrator path.

Office assignment subscribers and unstarted-task recovery classify a task as
Office-owned only through the authoritative Office project/workflow identity.
An ordinary Kanban runner never becomes autonomous by assignment alone.
Office recovery and cron work are disabled when their corresponding Office
configuration is absent.

## Scheduler lifecycle

The scheduler and each cron loop own one goroutine and one cancellation context.
The lifecycle is stopped -> running -> stopping -> stopped. Duplicate Start and
Stop calls are idempotent. Stop cancels future ticks and waits for the active
tick and handler fan-out before returning.

An in-process signal wakes the consumer immediately. A periodic safety pass
reclaims persisted work after a missed signal or restart. No new schema or run
state is introduced.

## Shutdown and recovery

Graceful shutdown stops external intake, joins queue and cron loops, stops the
orchestrator and agent runtime, then closes event-bus, repository, and database
resources. SQLite is never closed underneath live scheduler work. A blocked
handler keeps shutdown in progress; an outer supervisor may enforce its own
process grace period.

Queued and retryable runs remain durable across restart. A claimed run follows
the existing stale-claim recovery policy. In-memory signals and scheduler
contexts are not persistence sources.

## Failure and observability

Missed signals are recovered by the safety pass. An empty queue is a normal
wait state. Office configuration without an Office workflow does not trigger
ordinary Kanban scans. A signal-driven shutdown must not emit database-closed
scheduler errors or scheduler logs after the graceful-shutdown completion
message.

## Verification

- Test one global consumer across multiple workspaces, global FIFO ordering, and
  per-agent serialization.
- Test Office classification, explicit `queue_run`, missed-signal recovery, and
  restart recovery.
- Test idempotent lifecycle calls and shutdown while idle or inside a handler.
- Test that queued rows remain available after process restart.

## Related records

- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
- [Run scheduler lifecycle](../../../plans/run-scheduler-lifecycle/plan.md)
