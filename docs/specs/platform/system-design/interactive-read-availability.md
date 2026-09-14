---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-001
  - REQ-PLATFORM-INTERACTIVE-READS-002
  - REQ-PLATFORM-INTERACTIVE-READS-003
---

# Interactive Read Availability System Design

## Purpose and boundaries

Platform owns operational read availability. Analytics owns aggregate execution,
while workspace recovery follows its
[workspace design](../../workspaces/system-design/workspace-read-recovery.md).
The existing [required-store design](postgres-domain-store-parity.md#runtime-health)
and [persistence ADR](../../../decisions/2026-09-05-required-internal-persistence.md)
remain authoritative. This design adds workload control, not a health bypass.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-INTERACTIVE-READS-001 | Query shape and metric compatibility; Performance evidence |
| REQ-PLATFORM-INTERACTIVE-READS-002 | Admission and cancellation; Persistence and diagnostics |
| REQ-PLATFORM-INTERACTIVE-READS-003 | Section recovery and presentation |

## Query shape and metric compatibility

Change `internal/analytics/repository/sqlite/stats.go`. All runtime reads use
`Repository.ro`. The database layer supplies the selected SQLite or PostgreSQL
handle; continue using `dialect` helpers, `sqlx.In`, and `Rebind`.

For `GetTaskStats`, select the eligible workspace task page first, using the
existing task-created range, updated-time ordering, and caller's limit including
the extra pagination row. Aggregate sessions, turns, and messages independently
for those task IDs, then join aggregates. Session-start filtering remains distinct
from task-created filtering. Do not add a message timestamp filter where none
exists. Empty sessions still contribute a session count; messages without a turn
still contribute message counts. Preserve last completion, elapsed span, active
duration, and zero values. Add a task-ID tie-break only if existing tests/contracts
permit it; this repair does not require changing tied-row ordering.

For `GetRepositoryStats` and `buildRepositoryStatsQuery`, select eligible workspace
repositories before their child aggregates. Preserve the current distinctions:
task counts follow task-created range; session counts/messages/duration follow
session-start range; commit totals follow committed-at range. Task/session totals
are attributed through `task_repositories`; git totals use the session repository.
A task attached to two repositories contributes once to each eligible repository.
Do not sum raw turns after joining raw messages. Exclude soft-deleted repositories,
ephemeral tasks, and automation tasks as before.

For `GetDailyActivity`, constrain turn days to the requested inclusive UTC date
series before grouping. Count turns/tasks by day independently. Count messages
through distinct eligible (session, day) pairs, preserving current behavior: a
message contributes only when that session has a turn on the same day. Do not
silently redefine this as all messages created that day. Use portable timestamp
bounds/dialect normalization and cover midnight boundaries and naive UTC columns.
The date series still emits zero buckets. All-time heatmap remains 365 days.

`GetGlobalStats` already separates major aggregates. Preserve its clean-turn
outlier calculation. Keep completed activity, model usage, git totals, DTOs, top
model limits, completion definitions, and authorization unchanged. Add an index
only when query-plan evidence shows an unmet access path; replay it through the
existing `ensureStatsIndexes`, without a speculative index inventory.

## Admission and cancellation

Use one context-aware admission gate owned by the shared analytics repository
instance. Acquire once at each of its eight public read methods, including
`ListSessionCodeStats`; release after all rows close or any error. Do not acquire
again inside query helpers. Both HTTP Stats and plugin Host data reads receive the
same repository from `backendapp/storage.go`, so they share this budget.

Allow at most two concurrent analytics operations. The default SQLite pool has
four reader connections; queued analytics must not borrow another connection.
Keep the same analytics cap for PostgreSQL to avoid driver-specific request
behavior. It does not promise reserved capacity against unrelated workloads.
No new pool, broad scheduler, configurable knob, or persistent cache is needed.

Each operation has a ten-second total queue-plus-query deadline, capped by the
caller's earlier deadline. Waiting uses context cancellation. Check cancellation
again after admission and before SQL. Context-aware query/scan cleanup must return
capacity on cancellation, errors, and panic-safe deferred cleanup. Plugin calls
retain existing service error translation; no new plugin wire contract is added.

The Stats HTTP handler maps its own admission/query deadline to HTTP 503 with
`error_code: "analytics_busy"`, a sanitized message, and `Retry-After: 2`.
Client cancellation is not reported as a database defect. Other database errors
retain the existing error handling. Authorize before exposing analytics status.
Use `ApiError.errorCode` for the new error; persistence middleware instead uses
`body.code: "persistence_unavailable"`, so client classification must inspect that
existing field without changing the middleware contract.

The cap protects capacity; query optimization makes it practical. Never ship
serialization as the only performance fix. Tests hold admitted work at barriers,
then prove a normal read and a real health probe complete through the shared pool.

## Section recovery and presentation

`apps/web/app/stats/stats-data.tsx` retains independently loaded sections and adds
retry state/actions. Preserve its AbortController and workspace/range generation.
Keep successful section data during same-selection retries. A selection change
resets all sections and aborts every old timer/request. Copy remains gated by
`composeStatsResponse`.

Retry failed transient sections only: network failures, HTTP 429/502/503/504.
Allow two scheduled retries at 2 and 5 seconds after each preceding failure,
honoring a larger Retry-After. Cancel automatic retries while hidden; use
`useForegroundRefresh` to start one bounded recovery cycle on return. Manual
Retry starts a bounded cycle immediately. Timer, foreground, and manual triggers
coalesce into one in-flight request per section. No automatic retry for auth,
404, parse, or cancellation errors. Test using fake timers and deferred responses.

Render an inline failure and Retry in affected section cards using the existing
stats components. A same-selection refresh with retained data labels it stale;
an initial failure displays no fabricated zero. Show retry-in-progress status and
disable duplicate actions. Map temporary availability errors to translated copy
rather than exposing raw backend text. Update all five locale catalogs and generate
the Traditional Chinese pair using the repository script.

Desktop keeps its multi-column Stats layout. Phone enters through the existing
hamburger Stats destination and uses the existing single-column cards. Retry is
inside the failed card with at least a 44px touch area; ordinary desktop sizing
remains 28px. Keep the page's existing scroll owner, safe areas, focus behavior,
and no horizontal overflow. Workspace navigation remains its existing drawer.

## Performance evidence

Seed disposable data: 700 eligible tasks, 800 sessions, 5,000 turns, 650,000
messages, 2,200 commits, and five repositories in the measured workspace. Add a
second equally sized workspace to detect unnecessary cross-workspace scans.
Concentrate at least 200 turns and 10,000 messages in one session. Spread the
history over 400 days, including empty sessions and days without turns.

Use the same SQLite file shape, indexes, machine, and non-race binary before and
after optimization. Record CPU, memory, filesystem, driver, query plans, per-endpoint
latency, admission wait, and all-seven completion latency. Run ten measured warm
cycles after one warmup, for week/month/all ranges. The five-second p95 target
applies on a machine with at least four available CPU cores, 8 GiB RAM, and local
SSD storage, without unrelated load. Record cold results separately, without
claiming warm targets for cold caches. Also record a minimum fivefold improvement
for the heavy-join fixture; this is a delivery benchmark, not a fragile CI timer.

Normal tests assert numeric equivalence and concurrency bounds. Add opt-in Go
benchmarks with deterministic fixtures and report results in the work order.
Do not copy the user's database or transcripts into fixtures. A benchmark that
misses the target requires query work before the work order is done.

## Persistence and diagnostics

No new schema is required unless a measured index is necessary. Retain dual-engine
conformance. Actual missing tables and closed pools must still fail health and
recover through the existing probe rules. Do not increase health timeouts or
reader counts to hide overload. Pool exhaustion caused by other subsystems remains
outside this repair and must not be reported as solved.

Record bounded analytics operation names and elapsed admission/execution time in
existing structured logs on slow or failed calls. Do not log SQL, task titles,
workspace data, credentials, or query payloads. Request timings and health logs
provide the integration evidence; diagnostic bundles remain the collection path.
