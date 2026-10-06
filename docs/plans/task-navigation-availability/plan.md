---
created: 2026-10-06
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-004
  - REQ-PLATFORM-INTERACTIVE-READS-005
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
legacy_specs: []
---

# Implementation Plan: Task navigation availability

## Overview

Limit background inbox work, then make task navigation recover from temporary failures.
Keep the existing required-store health gate and its two-second probe deadline.
Implement each work order in this primary session, in the order below.

## Evidence and assumptions

The supplied console shows task REST requests returning `persistence_unavailable`
while an established WebSocket continues to deliver messages. The screenshot shows
the generic missing-task surface after those requests fail.

Backend bundle `c9bcbc22e80f4af51ead22d3fb0e30f9` records `reader_ping` timing out
after 2001ms at 18:21:42 Lisbon time. All four readers were occupied; the writer
was idle. Persistence recovered at 18:21:55. The bundle is partial because older
logs exceed its byte limit; the observed failure window is present.

Before that failure, repeated `/api/v1/clarification-inbox` reads took approximately
two seconds. During rejection, some consecutive reads arrived about 100ms apart.
These timings identify a workload candidate and request pressure. They do not
identify the four operations holding the reader pool or prove a query leak.

`useNeedsYouInboxController` permits independent refresh triggers to overlap.
`useTaskDetails` records any failed detail read as a generic task load error.
These source-level defects support the client repairs without assuming that every
slow query belongs to Inbox.

Baseline command passed:

```bash
(cd apps/backend && go test ./internal/persistence/requiredstores -run '^TestPersistenceContentionFixture$' -count=1 -v)
```

Both writer and reader subtests reproduced timeout, unhealthy state, and recovery.
No production or permanent test changes form part of this design turn.

## Scope

The subsequent [user journey read-efficiency audit](journey-read-audit.md) covers
Home, Kanban, task details, task switching, and message admission. It records
source-confirmed costs, two focused reproductions, and prioritized candidates.
Those candidates do not expand the three work orders below. New API contracts,
batch completion-gate reads, and turn-history paging need scoped designs before
implementation.

### In scope

- Shared, cancellable admission for clarification list/count SQL and measured query improvements.
- One browser inbox request across trigger families, with delayed trailing refresh and failure cooldown.
- Temporary task-load presentation, bounded retries, and manual recovery.
- Existing task/session identity, workspace authorization, and desktop/phone parity.
- Bounded operation timing evidence to identify remaining reader contention.

### Out of scope

- Additional database pools, more readers, longer health timeouts, or a health bypass.
- Broad scheduler changes, cross-tab coordination, persisted caching, or feature flags.
- Claiming that unrelated workloads cannot exhaust the remaining reader capacity.
- Automatic page reload, task creation, or agent launch from recovery.

## Technical approach

Follow the owning [design](../../specs/platform/system-design/interactive-read-availability.md#clarification-workload-control).
The backend caps clarification list/count execution at one operation per shared
task repository. Queued operations wait outside SQL and respect a ten-second
total deadline. HTTP maps its own deadline to retryable 503 after authorization.
MCP preserves its existing envelope and mutation semantics.

Record query plans on disposable history before selecting an index or changing
the query shape. Preserve the shared list/count predicate. Exercise a real
four-reader SQLite pool while admitted work remains blocked at a test barrier.
Normal reads and required-store probes must still complete.

The browser inbox coordinator combines all existing trigger families. Keep one
in-flight request and one pending refresh. Background starts have a one-second
minimum interval. Temporary failures impose 2/5/15/30-second cooldowns; triggers
cannot shorten them. Workspace and identity changes abort obsolete work.

Task recovery belongs to the navigation read owner, shared by route hydration and
the task page. Two scheduled retries follow temporary failures, after 2 and 5
seconds. Manual Retry starts another bounded cycle. Respect Retry-After, hidden
tabs, and navigation generations. Retain loaded authorized content on refresh
failure and show an in-flow notice.

| Boundary | Behavior | Evidence |
| --- | --- | --- |
| SQLite | One clarification operation; shared four-reader pool | Barrier test plus real health probe |
| PostgreSQL | Same operation cap; unchanged results | Isolated PostgreSQL parity tests |
| HTTP Inbox | Authorized read; deadline maps to sanitized 503 | Handler test |
| MCP clarification list | Shared admission; existing error envelope | Repository and existing MCP tests |
| Existing WS | Remains connected during HTTP health rejection | Recovery works without requiring reconnect |
| Desktop and phone | Same task selection and recovery state | Chromium and mobile-chrome flows |

## ASCII UI preview

### UI-01: Initial task read failure

Entry: desktop sidebar selection, phone task navigation, or a direct task URL.
Current state uses source-confirmed generic missing-task copy.

```text
Before                    After, desktop
Task unavailable          Task temporarily unavailable
It may have been deleted  We could not load this task. Try again.
or you may not have       [Retry]  [Back to task overview]
access.
[Back to task overview]

After, phone
+-------------------------------------+
| Task temporarily unavailable        |
| We could not load this task.        |
| Try again.                          |
| [             Retry              ] |
| [     Back to task overview       ] |
+-------------------------------------+
```

Control order and actions are required; wording and spacing are illustrative.
The retry-in-progress state announces `Retrying...` and disables duplicate Retry.
The exhausted state keeps both actions. A true 404 keeps the existing surface.
Desktop buttons measure 28px; phone buttons measure at least 44px.
Use the route's existing scroll owner and safe-area layout, with no horizontal overflow.

### UI-02: Failed refresh with loaded task

```text
[Temporary connection problem. Try again.  Retry]
[Existing selected task and session content]
```

The notice stays in flow. Phone wraps its explanation and places Retry beneath it.
No modal or automatic navigation covers the task. Map these views to AC-005.1,
.2, .4, .5, .6, and .7.

## Tests

- AC-004.1/.2: new `clarification_read_admission_test.go`,
  `TestClarificationReadsPreserveInteractiveCapacity`, with actual pool barriers.
- AC-004.3: existing clarification list/count suites plus a large-history parity fixture.
- AC-004.4/.5/.6: `use-needs-you-inbox-controller.test.tsx` with deferred requests,
  mixed trigger bursts, fake timers, scope changes, and cancellation.
- AC-005.1/.2/.3/.4/.5/.7: `task-navigation-reads.test.ts`, `task-detail-route.test.tsx`,
  `task-detail-route-recovery.test.tsx`, and `task-page-content.test.tsx`, including
  late retry results after task switches and real task-page session synchronization.
- Missing tables and genuinely unavailable pools retain existing required-store tests.

## E2E tests

- Extend `tests/chat/needs-you-inbox-background-refresh.spec.ts` to prove bounded
  requests during an event burst and delayed recovery after 503 (AC-004.4/.5).
- Add `tests/task/task-transient-read-recovery.spec.ts` and
  `tests/task/mobile-task-transient-read-recovery.spec.ts`. Inject a real structured
  HTTP 503, leave WS connected, recover through Retry, and preserve selection.
- Cover exhausted recovery, switching tasks before an old retry, permanent 404,
  phone containment, touch geometry, and keyboard activation (AC-005.1 through .7).
- Retain existing desktop/mobile loading and missing-task regressions.

## Work orders

- [x] [Task 01: Bound clarification database work](task-01-bound-clarification-reads.md)
- [x] [Task 02: Coalesce inbox refreshes](task-02-coalesce-inbox-refreshes.md)
- [x] [Task 03: Recover temporary task reads](task-03-recover-task-reads.md)

## Verification results

Baseline contention test passed. Implementation completed on 2026-10-06.
Design validation on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed (357 decisions, 1406 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: covered all three work orders.
- `git diff --check -- docs/specs docs/plans/task-navigation-availability`: passed.
- Requirements, designs, plans, and work orders remain unstaged and uncommitted.

Task 03 implementation checks passed:

- Focused navigation and task-page Vitest: 50 tests across 4 files, including
  recovery episodes, sibling-request cancellation, lightweight foreground
  refreshes, and real task-page selected-session synchronization.
- Web typecheck, i18n checks, `make build-web`, and `make build-backend`.
- Targeted ESLint completed with no warnings or errors.
- Chromium recovery/loading E2E: 6 passed; mobile Chrome recovery/loading E2E:
  5 passed. Both device suites cover an initial temporary failure and recovery;
  the mobile suite also verifies touch sizing and viewport containment.

The temporary task-specific PostgreSQL test container was stopped after testing.

## Risks

- The queries holding all four readers remain unidentified. Admission bounds one
  candidate workload; logs and disposable profiling must establish remaining causes.
- Query rewrites can change bundle membership or counts. Mixed states and workspace
  visibility require parity fixtures before optimization.
- Route and page retry loops can multiply requests unless one owner coordinates them.
- Session-list failure is an enrichment failure, not proof that the task has no sessions.
- Large reference fixtures belong in opt-in benchmarks, not timing-sensitive unit assertions.

## Operational follow-through

Before reporting the live incident fixed, collect sanitized operation timing and
pool-wait evidence after these changes. Identify any remaining reader holders.
If another subsystem dominates, prepare its focused work order from that evidence.
Do not change database capacity or health semantics as an unmeasured fallback.
