---
created: 2026-09-12
status: implemented
requirements:
  - REQ-WORKSPACES-READ-RECOVERY-001
  - REQ-PLATFORM-INTERACTIVE-READS-001
  - REQ-PLATFORM-INTERACTIVE-READS-002
  - REQ-PLATFORM-INTERACTIVE-READS-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
system_design:
  - ../../specs/workspaces/system-design/workspace-read-recovery.md
  - ../../specs/platform/system-design/interactive-read-availability.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Implementation Plan: Stats Performance and Read Recovery

## Overview

Preserve task navigation during temporary failures, reduce analytics query work,
bound analytics concurrency, and recover failed Stats sections. Execute four work
orders in this order. Sidebar preservation is independently useful; query work
precedes admission control so a concurrency cap does not serialize expensive work.

Implementation is complete. All four work orders are complete, with the targeted
backend, frontend, desktop, and phone recovery checks recorded below. PostgreSQL
parity remains an external validation item because this environment did not
provide `KANDEV_TEST_POSTGRES_DSN`.

## Evidence and assumptions

The investigation correlated build `e59874211` with checkout `f150dd32d`.
The implicated source files are identical between those builds. On September 12,
2026, requests starting near 22:51:44 Lisbon time took 36,872 ms (tasks),
44,358 ms (repositories), and 65,264 ms (daily activity). Reader health probes
failed their two-second deadline and caused stateful HTTP 503 responses.
At 22:51:47.620 and 22:52:48.962, route context reads also returned 503.

Confirmed defects: multiplicative turns/messages joins and empty-on-error route
hydration. Pool saturation is the strongest explanation for probe timeouts;
per-query occupancy was not captured. Do not claim corruption or task deletion.
The source bundle is `.kandev/diagnostics/c85bfc85d8bb3b0bd0b987e285ac9ae3.zip`.
It has a partial manifest due to retained-log limits. Do not commit the bundle or
copy user records into fixtures. The Kandev task plan retains the full findings.

The assumption check treats metric compatibility, workspace isolation, and real
persistence fail-closed behavior as repair constraints. Retry timing and the
analytics cap are explicit draft design choices, not measured performance claims.
There are no unanswered user-preference questions blocking this package.

## Scope

### In scope

- Same-context collection retention, empty-success handling, and bounded recovery.
- Exact task/repository/daily aggregates with early eligible-set filtering.
- Shared analytics admission, cancellation, timeout response, and targeted diagnostics.
- Stats partial failure, retry controls, copy gating, desktop and phone coverage.

### Out of scope

- New metrics, approximation, persistent rollups, caches, or new database pools.
- Relaxed persistence checks, general database tuning, or guarantees under unrelated saturation.
- Offline persistence, navigation redesign, changes to permissions, and user database mutations.

## Technical approach

The [workspace requirement](../../specs/workspaces/requirements/workspace-read-recovery.md)
owns cached context identity and recovery, including desktop/phone navigation.
The [platform requirement](../../specs/platform/requirements/interactive-read-availability.md)
owns shared capacity, aggregate read efficiency, and Stats recovery. These are
independent contracts; neither is a UI-only duplicate.

1. Fix `useRouteData` and the corresponding kanban bootstrap. Commit only successful
   collection outcomes and guard every asynchronous write with workspace generation.
   Share refresh status with navigation and refresh failed snapshots after recovery.
2. Rewrite `GetTaskStats`, `GetDailyActivity`, and `buildRepositoryStatsQuery`.
   Aggregate messages/turns independently and retain each metric's timestamp and
   repository attribution rules. Split query helpers if file/function limits require it.
3. Gate all eight analytics repository read methods, including plugin code stats,
   through the same two-operation limit. Apply a ten-second total deadline and
   keep waiting outside the shared reader pool. HTTP Stats returns bounded
   `analytics_busy` responses; real persistence errors retain their current policy.
4. Add per-section recovery and localized Retry controls in Stats. Reuse existing
   foreground refresh and ApiError transport; no global fetch-client retries.

The existing required-persistence ADR remains authoritative. The record assessment
found no need for another ADR: local admission control supports that boundary and
has its rationale in the design. A separate probe pool or changed health semantics
would require a new design decision and is excluded from these work orders.

No existing companion implementation package was found for these Stats/route-read
contracts. The existing persistence specification is referenced without changing
its health policy. No schema migration is planned; add only measured indexes if needed.

## ASCII UI preview

UI-01: Navigation refresh failure, entered by opening Stats.
Before (confirmed): `TASKS / No tasks yet.` replaces previously loaded tasks.
After, desktop sidebar:

```text
TASKS                    All tasks
Could not refresh tasks. [Retry]
Task A
Task B
```

After, phone: open the existing task navigation drawer.

```text
+-------------------------------+
| Tasks                   Close |  fixed header
| Could not refresh. [Retry]    |
| Task A                        |  one scrolling body
| Task B                        |
+-------------------------------+  existing safe-area inset
```

UI-02: Stats section failure, Stats route. Desktop keeps the existing card grid;
phone keeps the existing single-column page, with this same card structure:

```text
+----------------------------------+
| Activity                         |
| Could not load activity. [Retry] |
+----------------------------------+
```

With retained data, show the chart and `Could not refresh. [Retry]` below its title.
During retry, keep retained content and replace the action with disabled
`Retrying...`. With no prior data, retain the failure region while retrying.
A successful zero result uses the existing empty chart. An initial navigation
failure uses a load-error region, not `No tasks yet`. Retry is 28px on desktop,
at least 44px on touch. Text is illustrative and must be localized.

Structural requirements: retained rows/cards, inline failure, visible Retry,
unchanged route/drawer navigation, one scroll owner per existing surface, no new
overlay. UI-01 maps to AC-WORKSPACES-READ-RECOVERY-001.1/.5/.6;
UI-02 maps to AC-PLATFORM-INTERACTIVE-READS-003.1/.2/.4/.5.

## Tests

| Acceptance | Evidence |
| --- | --- |
| AC-WORKSPACES-READ-RECOVERY-001.1/.2/.3/.4/.7 | `spa-routes.workspace.test.tsx`: retained data, authoritative empty, mixed outcomes, stale generation and denied access; matching kanban startup tests |
| AC-WORKSPACES-READ-RECOVERY-001.5/.6 | Route recovery fake-timer tests, snapshot failed-key test, desktop/phone browser scenarios |
| AC-PLATFORM-INTERACTIVE-READS-001.1/.2 | Analytics regression fixtures: independent counts, range boundaries, repository attribution, exclusions, same assertions on both engines |
| AC-PLATFORM-INTERACTIVE-READS-001.3 | Task 02 before/after benchmark; Task 03 final seven-section benchmark with admission enabled |
| AC-PLATFORM-INTERACTIVE-READS-002.1/.2/.3 | Admission barrier tests, cancellation/deadline tests, shared-pool read/probe integration test |
| AC-PLATFORM-INTERACTIVE-READS-002.4 and AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1-.5 | Existing required-store and middleware tests, genuine missing-table/recovery regression |
| AC-PLATFORM-INTERACTIVE-READS-003.1-.5 | Stats hook/component tests plus desktop/phone retry browser tests |

Each work order names new test methods and exact commands. Baseline fixture SQL
must be exercised before query edits. Capture RED evidence before implementation;
performance RED is a same-machine baseline, not an arbitrary short CI timeout.

## E2E tests

- Task 01: new `tests/layout/sidebar-read-recovery.spec.ts` (`chromium`) and
  `tests/layout/mobile-sidebar-read-recovery.spec.ts` (`mobile-chrome`) prove UI-01.
  Seed tasks, navigate to Stats, fail route workflow/repository/step requests,
  retain same-workspace tasks, retry, and prove workspace-switch isolation.
- Task 04: new `tests/layout/stats-read-recovery.spec.ts` (`chromium`) and
  `tests/layout/mobile-stats-read-recovery.spec.ts` (`mobile-chrome`) prove UI-02.
  Fail one section, keep successful cards, recover automatically/manually, and
  assert Copy Stats becomes available only after all sections recover.
- Reuse the mobile hamburger entry in `mobile-stats-nav.spec.ts` and the existing
  workspace-switch/sidebar isolation tests. Use API seeding and causal request
  waits. The managed runner builds current production assets and tears down.

## Work orders

- [x] [Task 01: Preserve workspace navigation during failures](task-01-workspace-recovery.md)
- [x] [Task 02: Remove multiplicative statistics queries](task-02-query-aggregation.md)
- [x] [Task 03: Bound analytics database occupancy](task-03-analytics-admission.md)
- [x] [Task 04: Recover failed statistics sections](task-04-stats-recovery.md)

## Verification results

Focused implementation validation passed for the affected backend packages and
frontend lifecycle tests, including the route-unmount, snapshot recovery, and
hidden-tab retry regressions. The real availability regression uses a separate
writer and four-reader SQLite pool, holds two reader connections through
concurrent HTTP analytics operations, queues a service/plugin operation through
the same repository, and verifies that a normal read and required-store probe
remain healthy. A companion missing-table case still fails closed.

The production-shaped concurrent HTTP benchmark launches all seven authorized
Stats handlers through one shared two-operation admission gate. With the
documented disposable fixture, the month range measured a nearest-rank cycle
p95 of 3.033 s and the all range measured 1.939 s, both below the five-second
target. The Go benchmark's `ns/op` value is a mean; the custom cycle p95/max
metrics and verbose per-cycle logs are the admission evidence. The sequential
SQLite all-seven numbers remain a Task 02 query baseline and are not the final
admission gate. These local measurements do not claim production latency.

The new desktop and phone snapshot recovery scenarios both pass through the
managed headless runner, including the mobile retry touch-target assertion. The
PostgreSQL parity subtest was skipped because `KANDEV_TEST_POSTGRES_DSN` was not
set. A repository-wide `make test` was attempted earlier, but unrelated
environment-sensitive failures remain in agentctl process probes, common config
discovery, launcher service configuration, and Office migration tests. Affected
analytics packages and backendapp persistence checks pass independently.

## Risks

- Counting by session/day must retain current daily-message eligibility; a simpler
  message-by-day query changes results.
- Repository attribution and session-start ranges differ from commit attribution
  and task-created ranges. A shared filter can silently change totals.
- Cached context must not cross workspace or identity boundaries. Authorization
  failures must not be treated as harmless temporary failures.
- Two analytics operations reserve no capacity against other callers; integration
  claims are restricted to analytics-induced pressure on otherwise healthy storage.
- Ten-second admission deadlines can produce retryable errors under bursts. Bounded
  browser retries must not amplify them. Plugin callers retain existing error handling.
- The five-second benchmark target depends on the documented reference machine;
  it is an acceptance goal, not an observed production speedup.

## Documentation impact

Task 03 updates the Operations explanation of temporary analytics pressure and
the distinction from `persistence_unavailable`. Task 04 updates the
feature-status reference with independent Stats section recovery. Existing
navigation screenshots and README terminology do not change.
