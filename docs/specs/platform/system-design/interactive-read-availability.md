---
status: current
system: platform
updated: 2026-10-29
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-001
  - REQ-PLATFORM-INTERACTIVE-READS-002
  - REQ-PLATFORM-INTERACTIVE-READS-003
  - REQ-PLATFORM-INTERACTIVE-READS-004
  - REQ-PLATFORM-INTERACTIVE-READS-005
  - REQ-PLATFORM-INTERACTIVE-READS-007
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
| REQ-PLATFORM-INTERACTIVE-READS-004 | Clarification workload control |
| REQ-PLATFORM-INTERACTIVE-READS-005 | Task navigation recovery |
| REQ-PLATFORM-INTERACTIVE-READS-007 | Quiet task refresh recovery |

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

## Clarification workload control

The task repository owns one admission gate for `ListUnresolvedClarificationBundles`
and `CountHiddenClarificationBundles`. Permit one operation at a time, across
workspaces and HTTP/MCP callers sharing that repository. Acquire before SQL,
release after rows close, and bound queue plus execution to ten seconds.
Use the caller's earlier deadline when present. Check cancellation after
admission before starting SQL. Match the existing analytics admission pattern;
do not introduce a generic database scheduler or additional pool.

The normal and hidden HTTP inbox handlers also impose a ten-second total read
deadline. Carry it through authorization, listing, message hydration, and counts.
Translate an internally expired read into sanitized HTTP 503 with `Retry-After: 2`.
Preserve authorization and non-timeout database error handling. Caller cancellation
must release capacity without changing required-store health. MCP keeps its
existing error envelope, and mutating clarification responses keep their current
ownership and transaction path.

Measure the list and hidden-count query plans before changing SQL or indexes.
`clarificationBundleTableExpr` currently groups message history before applying
workspace conditions. Inspect its access paths on substantial unrelated history.
Push eligible task/session restrictions into candidate selection where semantics
permit, or add a measured dialect-safe index through existing schema replay.
Keep the common predicate shared by listing and counts. Preserve mixed-status
bundles, parent-question exclusion, current-turn authority, nested question IDs,
sidecars, and cursor ordering. Do not copy a live database into a fixture.

The admission test holds the admitted query at a barrier. Excess work must wait
without borrowing connections. A task read and real `requiredstores.Health.Check`
must succeed through the same factory-created SQLite pool. Test cancellation,
SQL failures, missing tables, and recovery separately. PostgreSQL retains the
same operation cap and equivalent query results.

`useNeedsYouInboxController` owns one refresh coordinator for all five existing
trigger families. Coalesce mount, reconnect, foreground, periodic, snooze, and
WS refresh requests into one in-flight read for the active scope. Events during
execution set one dirty marker; a trailing read refreshes the latest state.
Use a one-second minimum between background request starts. A failed temporary
read imposes a shared cooldown of 2, 5, 15, then 30 seconds, capped at 30 seconds.
Honor a longer Retry-After. Events cannot reset the failure count or shorten it.
The cooldown itself does not create an endless timer: a pending trigger can
produce one deferred read. Retain the existing visible-only periodic trigger.

Abort outstanding reads and clear pending work when workspace, identity, feature
activation, or component lifetime changes. Keep generation guards as a second
defense against late results. Auth and permanent lookup failures suspend automatic
refresh until the scope changes; explicit user recovery remains available.
Preserve the Inbox's existing rule that failed reads clear its rows and badge.
This repair adds no speculative counts or new badge presentation.

Log slow or failed clarification operations with fixed operation names, admission
wait, execution time, and reader pool statistics. Avoid row data, metadata, SQL,
credentials, and transcripts. Before claiming the live incident solved, correlate
query ownership with a health-probe timeout or reproduce it on disposable data.
One occupied inbox slot does not reserve capacity against other subsystems.

## Task navigation recovery

`TaskNavigationReads` remains the owner of shared task/session identity reads.
Retain its navigation-generation and authentication/workspace scope checks.
Add cancellation transport to `fetchTaskNavigationIdentity`, passing the request
signal through both existing API calls. Obsolete generations cannot publish or
start a retry. Shared consumers must not independently abort each other's read;
the navigation owner controls cancellation at scope changes.

Use one recovery owner for the selected navigation identity. Route hydration and
`useTaskDetails` consume its attempt state and retry action. Do not add independent
route and component retry loops. Failed session-list enrichment keeps its existing
fallback behavior and can retry separately without discarding successful task
details. Do not infer that an empty fallback session list is authoritative.

Classify network failures and HTTP 429/502/503/504 as temporary. Recognize the
existing persistence code through `ApiError.body.code`; `ApiError.errorCode`
currently exposes only `error_code`. Do not change server envelopes or retry
parse errors, aborts, authorization errors, or 404.

### Bounded recovery across a health cycle

One cycle includes the initial attempt and at most four automatic retries.
The retry delays are 2, 5, 10, and 10 seconds after each preceding failure.
Prompt failures therefore start attempts near 0, 2, 7, 17, and 27 seconds.
The later attempts cover a complete fifteen-second health-probe interval.
This revises the previous two-retry policy without adding another recovery owner.

Cap the entire cycle at thirty seconds of visible-tab time, including attempts and delays.
Keep the ten-second wall-time deadline for each individual attempt.
The earlier deadline wins while the tab is visible.
Hiding the tab pauses the cycle budget and scheduled retry delay.
An existing attempt can settle or reach its own deadline while hidden.
No automatic retry starts until the tab becomes visible again.
Foreground triggers join that active cycle without renewing its budget.

Keep `TaskNavigationRecoveryClock` beside `TaskNavigationReads`.
The sidebar recovery owner also reuses this clock for the same visible-time policy.
It owns the remaining cycle time, visibility listener, and deadline timer.
The navigation owner disposes those resources on success, exhaustion, cancellation, and final release.
Use a monotonic clock and fake timers in tests.
Keep budget expiry distinct from navigation cancellation.
Budget expiry aborts outstanding task/session work and publishes a temporary failed snapshot.
Navigation cancellation remains an abort and cannot publish into the new scope.
Preserve the most recent temporary error during a pending retry for classification and accessible progress.
Visible warning timing follows quiet task refresh recovery.

Honor a larger `ApiError.retryAfterSeconds` value without sending an early request.
When its delay exceeds the remaining budget, wait only until budget exhaustion and stop.
Do not extend the cycle, shorten the server delay, or schedule another attempt after exhaustion.
At the deadline, success requires a completed response accepted before expiry.
A late response cannot revive the exhausted cycle.

Manual Retry starts a new bounded cycle after exhaustion.
Repeated clicks and mixed route/component triggers share the active promise.
Foreground recovery starts at most one new cycle per foreground episode after exhaustion.
Ordinary rerenders and WS notifications cannot restart an exhausted cycle.
Keep the existing optional session-list fallback and shared `TaskSessionReads` ownership rules.

### Content continuity and presentation

When no task content exists, show ordinary loading progress while the bounded cycle runs.
Keep Retry disabled with its stable accessible name and leave the task-overview link usable.
After exhaustion, enable Retry in the temporary error surface.
Keep permanent failures in `TaskLoadErrorState` without automatic retries.
Use neutral availability copy that does not attribute a server failure to the user's connection.

Preserve the route error's temporary classification through `TaskDetailRouteState` and `taskRouteReadError`.
In `deriveTaskDetailRouteView`, permit the current `useTaskRouteProjection` during loading and temporary failures.
That selector already checks task identity, authentication, active workspace, archive state, and requested-session ownership.
Do not broaden its eligibility or replay a previous route's hydration payload as fallback.
Keep `initialState` null for a retained projection so stale data cannot overwrite newer store state.
For loaded same-task details, preserve the current task view and selected session through failed refreshes.
The shared read snapshot drives quiet recovery progress and an in-flow notice after exhaustion.
On success, clear the notice without remounting the conversation or resetting a draft.
On a permanent lookup or authorization failure, stop retention even when a projection remains cached.
Existing invalidation still clears content when authorization or workspace scope changes.
Retained content does not authorize a mutation, queue writes, or establish backend health.

Desktop enters through the sidebar; phone enters through existing task navigation
or a direct route. Reuse the missing-task recovery surface and `task-layout.tsx`
as composition precedents. Both layouts use the same classification and recovery
state. Show status, explanation, primary Retry, then the overview link.
Phone stacks actions inside the main content area, retains the route's scroll
owner and safe-area handling, and provides targets of at least 44px. Desktop uses
ordinary 28px buttons. Use semantic status announcements and retain focus on the
Retry control during attempts. Keep the overview action reachable during an initial cold-load recovery.
After exhaustion, show the retained-content notice as one row on desktop and a wrapped block on phone.
The notice remains in normal flow above the existing task content.
`TaskNavigationReadFeedback` in `components/task/task-navigation-read-feedback.tsx` owns this strip.
Place it directly below the task header and above the session tabs and panels, across the task content area.
Replace its current absolute positioning with normal layout participation.
Include this strip in the existing mobile header-offset calculation so the fixed phone header cannot cover it.
Preserve the single content scroll region and keep the composer outside the notice's bounds.
No modal, blocking overlay, or automatic page reload is needed.

Add copy in all seven locale catalogs, generating the Traditional Chinese pair
with `i18n:zh-hant`. Test temporary failure followed by success, exhausted retries,
manual recovery, valid selected-session preservation, and navigation during an
old pending retry. Reproduce a thirteen-second structured persistence outage and a fifteen-second boundary case with fake timers.
Add a rendered current-projection regression that remains visible after an exhausted temporary cycle.
Then return 404 or an authorization error and require the permanent unavailable surface.
Cover hidden-time accounting, never-settling reads, late responses, Retry-After beyond the budget, and cleanup.
Desktop and phone E2E cover automatic recovery without a new user event, exhaustion, manual recovery, and retained content.

## Quiet task refresh recovery

This section owns AC-PLATFORM-INTERACTIVE-READS-007.1 through .8.
UI retains sidebar membership, ordering, disclosure continuity, and permission rules through its
[sidebar design](../../ui/system-design/sidebar-archived-filter.md).
Recovery changes neither the persistence admission gate nor mutation behavior.

### Sidebar recovery ownership

Keep `SidebarTaskPageCache` as the per-store owner of a query's shared read.
Implement its bounded runner in `lib/sidebar/sidebar-task-page-recovery.ts`.
Do not place retry timers in each `useSidebarTaskPage` consumer.
Use the existing cache request identity, including workspace, scope, full query, and requested page.
Preserve the separate request generation and ranking checks in the hook.

A consumer lease spans the displayed query lifetime, including terminal failure.
Release that lease on replacement, scope invalidation, disablement, or unmount.
Do not release it after each attempt or discard the terminal record when its promise settles.
Multiple desktop and phone mounts join the same cycle.
Final release aborts pending attempts, disposes the clock, and removes the recovery record.
Retain existing StrictMode restart behavior and bounded page/entity ownership.

The runner uses the task-navigation schedule: five attempts, with delays of 2, 5, 10, and 10 seconds.
Reuse `TaskNavigationRecoveryClock` for the thirty-second visible-time budget and hidden-tab accounting.
Each HTTP attempt has a ten-second wall-time deadline and its own abort signal.
Cycle expiry and cancellation abort that attempt even when a mocked transport ignores cancellation.
Cleanup closes its deadline timer and child signal listeners.

Extract shared HTTP/network classification into `lib/api/temporary-read-error.ts`.
Keep `isTemporaryTaskNavigationError` compatible with its existing cycle-deadline handling.
Both owners classify TypeError, HTTP 429/502/503/504, and structured `persistence_unavailable` as temporary.
An attempt deadline is temporary. An obsolete-request abort is cancellation.
Validation, authorization, 404, parsing, and unknown errors do not receive automatic retries.
Honor Retry-After without extending the cycle or advancing an attempt early.

Keep the hook's page request pending throughout an automatic cycle.
Temporary attempts do not publish `sidebar:queryRefreshFailed` into component state.
After exhaustion, publish the existing localized error once and retain eligible rows.
The shared record distinguishes background triggers, explicit Retry, and a foreground recovery episode.
Repeated foreground triggers join pending work and reuse an already handled episode.

Live invalidations mark membership dirty without resetting the active budget or backoff.
Preserve provisional response reconciliation and the single trailing refresh after accepted success.
`useProvisionalSidebarRefresh` must not restart a failed cycle when pending returns to null.
An exhausted cycle rejects ordinary background refresh admission until explicit Retry or a later foreground episode.
Queued invalidations cannot bypass that latch. They remain available for the next admitted read.
No retry record makes a provisional page reusable or establishes complete local coverage.

### Warning visibility

`TaskNavigationReads` adds an owner-level latch for exhausted temporary recovery.
Set it on temporary exhaustion, preserve it through explicit recovery, and clear it on success or scope invalidation.
Permanent failures use their existing unavailable surface and invalidate retained content.
Feed warning visibility through `TaskNavigationReadRecovery`; do not infer it from temporary classification alone.
The route, `useTaskDetails`, and `TaskNavigationReadFeedback` consume the same owner state.

During automatic recovery with eligible content, keep only a screen-reader status.
Do not mount a visible strip, disabled Retry row, toast, or spinner that moves existing content.
Without eligible content, keep the ordinary loading surface and reachable task-overview action.
Show the applicable error surface only after exhaustion or an immediate permanent failure.

After exhaustion, preserve the existing sidebar status slot and task-detail strip.
Explicit recovery keeps that warning visible and disables its stable Retry control until settlement.
Do not clear it merely because another request begins.
Success removes the warning without remounting the conversation or changing a draft.
Announce the start and terminal outcome of a logical cycle, not every failed HTTP attempt.

### Desktop and phone

Desktop reuses `SidebarTaskQueryStatus` below view controls and the in-flow `TaskNavigationReadFeedback` strip.
Phone reuses `SessionTaskSwitcherSheet`, `MobileTaskList`, and the existing app-navigation task outlet.
Their current drawer or menu remains the task-choice surface. Recovery opens no new overlay.
The picker body or menu retains its current scroll owner, safe areas, dismissal, and focus return.
The task-detail phone layout retains its header offset only while the terminal strip exists.
Retry remains at least 44px on phones or coarse pointers and 28px on fine-pointer desktop.
Shared state supplies identical timing, errors, and recovery actions to every presentation.

### Regression evidence

Use fake timers and deferred requests for provisional-page failure, all trigger families, and shared-consumer tests.
Assert attempt counts, elapsed visible time, terminal latching, and timer cleanup.
Cover cancellation, Retry-After, hidden tabs, never-settling requests, and permanent errors after a temporary failure.
Keep deletion, disclosure, live reconciliation, page bounds, and access-denial regressions.
Component tests prove that active automatic recovery reserves no visible error-strip space.
Desktop and phone E2E cover a thirteen-second outage, exhausted recovery, manual recovery, and retained task content.
Use existing causal waits and request counters. Keep the current permanent-error scenarios.

## Implementation plans

- [Task navigation availability repair](../../../plans/task-navigation-availability/plan.md) covers REQ-PLATFORM-INTERACTIVE-READS-004 and -005.
- [Task read resilience](../../../plans/task-read-resilience/plan.md) revises recovery timing and preserves current task content for REQ-PLATFORM-INTERACTIVE-READS-005.
- [Quiet task refresh recovery](../../../plans/quiet-task-refresh-recovery/plan.md) adds bounded sidebar recovery and defers warnings until exhaustion.

## Journey loading extension

[Journey data loading](journey-data-loading.md) owns the new reader-isolation, visible-detail, boot, and shared-loading work. The [delivery package](../../../plans/journey-data-efficiency/plan.md) preserves the existing completed plans and their verification history.
