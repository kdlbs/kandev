---
status: current
system: tasks
requirements:
  - REQ-TASKS-CREATE-RESPONSIVENESS-001
---

# Task creation responsiveness system design

## Ownership and boundaries

Tasks owns the create request and initial-session persistence.
This design reduces repeated writer acquisition within that existing path.
It uses the existing SQLite writer, WAL readers, session transactions, and
targeted user-settings updates.

The [aggregate creation protocol](task-creation-protocol.md) is a separate draft
in the [archive package](../../../plans/agent-self-archive-cancellation/plan.md).
Its retained operations and Complete/Abort machinery do not exist in this path.
This package neither implements nor supersedes that migration.
The optimized preparation can become its required `session_prepare` step later.

The existing [delivery write-cost design](../../platform/system-design/durable-delivery-write-cost.md)
and [writer admission decision](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
remain authoritative. This design changes no SQL durability setting or ACK gate.

## Requirement mapping

| Acceptance | Design section |
| --- | --- |
| AC-TASKS-CREATE-RESPONSIVENESS-001.1, .2 | Measurement boundary |
| AC-TASKS-CREATE-RESPONSIVENESS-001.3 | Prepared session seed |
| AC-TASKS-CREATE-RESPONSIVENESS-001.4 | Primary election |
| AC-TASKS-CREATE-RESPONSIVENESS-001.5 | Last-used choices |
| AC-TASKS-CREATE-RESPONSIVENESS-001.6 | Browser behavior |

## Current request path

`TaskHandlers.httpCreateTask` calls `Service.CreateTask`, persists repository
links and other required relationships, then prepares the initial session.
It marks scheduling, settles the external ID, dispatches agent launch, records
last-used choices, and finally returns the response.

Session preparation reaches `orchestrator.prepareSessionForStart`, then
`executor.prepareSessionAttempt`. The production repository inserts the session
inside a cleanup-guarded transaction. Initial runtime seed and workspace binding
already have transactional variants.

The executor then calls `SetSessionPrimary` in a second transaction.
The orchestrator follows with two `SetSessionMetadataKey` autocommits for
`initial_prompt_preview` and `initial_prompt_submission`.
These four writer acquisitions can interleave with unrelated delivery work.

`RecordTaskCreateLastUsed` then updates `users.settings` and increments its
revision even when the supplied choices match. Colors use the same user's
settings row and revision CAS, so this also creates avoidable conflict pressure.
The browser's `queueTaskCreateLastUsedFromPayload` only updates an in-memory
overlay. It does not issue a duplicate preference write.

The source proves avoidable writer work. Existing logs do not attribute every
millisecond of the observed session-preparation delay to those statements.
The full-path baseline remains a required implementation gate.

## Prepared session seed

Pass the initial preview and validated submission into the existing executor
preparation call before it constructs `TaskSession`.
A typed, request-scoped seed belongs in `internal/task/models`, which both
orchestrator and executor already consume. It must contain no transport object.
The executor merges these values into the session metadata before insertion.

All prepared-session repository variants use that same seed. These include
ordinary creation, initial runtime seed, workflow route, bound workspace, and
shared-group workspace creation. Existing cleanup, runner, environment, and
route checks stay inside their current transactions.

The caller validates and snapshots the seed before persistence.
Cancellation before admission produces no session.
A failed transaction persists neither seed nor session.
Only a successful commit permits session-created publication and later launch.
No external I/O enters the SQL transaction.

Reused Office sessions keep their existing metadata and primary ownership.
The optimization applies the seed only when this preparation creates a row.
Passthrough preparation retains its existing upgrade and deferred-start rules.
Other callers without a seed retain their current behavior.

Remove the two post-insert metadata writes after every production preparation
variant consumes the seed. Fresh recovery continues to read the same stored
metadata keys and serialized values.

## Primary election

Initial primary election joins the existing prepared-session transaction.
The creation request expresses election-if-missing separately from explicit
promotion. Under the owning task lock, the repository determines whether a
primary already exists, then inserts the new row with the committed flag.

SQLite acquires its writer at BEGIN. PostgreSQL acquires the task row before
session state, in the same order as existing primary promotion.
Concurrent preparations therefore observe the prior committed election.
An existing primary is never demoted by an ordinary new-session preparation.

The executor consumes the committed primary result and avoids the later
`SetSessionPrimary` call. Its primary callback runs once after commit only for
the newly elected row. Explicit promotion still uses `SetSessionPrimary` and
retains its task-before-session locking and lifecycle guards.

This removes one transaction and the clear-then-set updates for ordinary
preparation. It does not remove required primary publication or its callbacks.
Office session reuse and workspace-binding retry must return actual committed
ownership before the callback runs.

## Last-used choices

Keep successful-create recording synchronous and backend-owned.
Moving it into an untracked goroutine creates ordering and shutdown risks.
The existing [source-of-truth decision](../../../decisions/0028-task-create-last-used-source-of-truth.md)
and [workspace memory decision](../../../decisions/2026-08-08-workspace-scoped-task-create-workflow-memory.md)
remain applicable.

Extend the targeted `UpdateTaskCreateLastUsed` statement with null-safe
predicates for the supplied fields. An equal patch changes no row, timestamp,
or revision. A changed patch preserves omitted fields and other workspace keys.
The statement evaluates these conditions against current stored state.
A reader precheck alone cannot authorize skipping a concurrent update.

Return the stored settings plus an internal changed result.
On no change, the recorder publishes no update event.
On change, it publishes the committed settings once.
If a guarded statement returns no row, distinguish an equal patch from a
missing user. Use SQLite JSON and PostgreSQL JSONB semantics consistently.
Normal task creation still succeeds if this optional preference recording fails.

This reduces settings writes and CAS churn. Color request retries and generic
error mapping remain outside this correction.

## Browser behavior

The standard desktop dialog and phone creation surface use the same backend
acceptance boundary. Neither closes before the existing accepted result.
The recovery receipt, external-ID lookup, uncertain-response handling, initial
prompt preview, and navigation keep their existing responsibilities.

No rendered UI, copy, geometry, scrolling, touch behavior, or API shape changes.
Desktop uses the sidebar color menu. Phone uses the existing navigation drawer
and reachable color controls. Both must retain saved colors after reload.
The existing phone creation and color tests are the interaction precedents.

## Measurement boundary

Add `BenchmarkTaskCreateContention` beside the real task HTTP handlers.
Use file-backed SQLite through `OpenSQLite` and `OpenSQLiteReader`.
Wire the real task service, session preparation, user service, and settings CAS.
Only subprocess/provider launch is substituted after the preparation boundary.
No live user database or task participates.

Measure complete `POST /api/v1/tasks` handling and concurrent real settings
patches. Cover zero, four, and eight durable text streams at 100 events/s each.
Offer events independently of writer completion. Count offered, projected,
and failed events and writes. Separate immediate start and no-agent creation.
Run at least 100 requests per load point after initialization and warmup.

Retain the original benchmark binary before production changes.
Run original and candidate sequentially on the same host and identical
fixtures within one comparison session. Record commits, Go and SQLite versions,
host load, sample counts, p50/p95/p99, actual throughput, and failures.
Count writer acquisitions, commits, and statements separately.
Do not convert cumulative pool waiting into per-request latency.

Optional variants cover several repositories, blockers, attachments, and shared
workspaces. Their correctness tests are required even when they are outside the
reference performance workload. A synthetic row-insert result cannot replace
the full endpoint measurement.

Use `db_writer_pool_stats` and existing delivery lag/barrier metrics during
diagnosis. A slow-phase log can identify preparation boundaries without prompts,
SQL parameter values, or unbounded metric labels.

## Failure and compatibility checks

- Seed failure rolls back the session. Cancellation leaves no partial seed.
- Concurrent initial sessions preserve one primary and the original-session marker.
- Explicit promotion, binding retry, and Office reuse retain their behavior.
- External-ID Found responses perform no new preparation or preference writes.
- WIP queues and blockers still suppress the immediate start.
- Initial attachments, plan mode, and fresh recovery use the original submission.
- Concurrent colors survive changed and unchanged preference recording.
- PostgreSQL exercises each changed SQL method against real separate connections.

No schema migration, new pool, persistence owner, feature flag, or dependency
is required. These local changes reuse existing contracts, so no new ADR is needed.
