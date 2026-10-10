---
created: 2026-09-28
status: in_progress
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/sidebar-archived-filter.md
legacy_specs: []
---

# Implementation Plan: Task Navigation Responsiveness

## Overview

Repair overview hydration, repeated session reads, and blocking file-tree
restoration. Then measure the repaired task switch and investigate the remaining
memory/debug questions in an isolated runtime. Execute the work orders
sequentially in the primary session. Implementation was explicitly requested
after the design checkpoint. Tasks 01-06 are complete; Task 07 restores small sidebar views from Zustand in
the same PR, as explicitly requested. No additional local test runs or seeded
preview are required; CI owns further validation. the user's follow-up
[Task 06](task-06-firefox-task-paint.md) removes measured Firefox rendering work
and records the remaining latency and desktop/phone verification. Its background
refresh follow-up also removes the task-list shift caused by routine query status.

## Evidence and confirmed causes

The September 28 diagnostic capture examined running commit
`89ff7ff7131d3856eb775cb79e4a960be75a867c`. The affected source files match this
checkout's baseline `36a2f95e8763253aa2044511a829aa257d4f65d3`.
Local supporting artifacts are retained under `.kandev/diagnostics/`, including
`responsiveness-report.md`, the baseline ZIP, and correlated request timings.
They are deliberately not committed with private task/log data.

| Finding | Evidence | Cause |
| --- | --- | --- |
| Overview route crashes during hydration | Three logged render failures; served bundle matches the guard; temporary JavaScript reproduction throws | `cached?.snapshot === snapshot` permits two absent values, then dereferences `cached.hiddenStepIds` |
| Duplicate reads during one task return | Nine shell reads, four commit reads, three simultaneous identical diff reads | Per-hook initialization refs; each diff subscriber resets the shared loading flag |
| Files take several seconds to restore | 22 serial requests; 3.643s and 3.842s total; request-duration sums nearly equal those totals | Root plus 21 expanded paths awaited one at a time; tree published only after completion and cleared on switch |

There were 39-56 requests in each sampled five-second navigation window. Those
counts include legitimate reads. The fourth later diff request followed a real
invalidation and must not be suppressed. History-response timestamps are not
paint timings. The ordinary five-second message backfill is not a diagnosed bug.

The machine had CPU/RAM headroom and fast health/static responses when sampled.
The main backend's approximately 3.8GiB RSS, another pre-existing instance's
approximately 2.5GiB RSS, and enabled frontend debug logging warrant measurement.
They do not establish a memory leak, bad settings, or a need to stop either
instance. Authenticated live profiler endpoints returned 401; no token was
available. No browser CPU trace was obtained in that diagnostic turn.

## Ownership, assumptions, and specification reconciliation

UI owns reusable client navigation, request coordination, and presentation.
The existing board visibility requirements do not define missing-snapshot
hydration; existing file-tree requirements cover actions and geometry rather
than restoration availability. The new durable
[requirement](../../specs/ui/requirements/task-navigation-responsiveness.md)
adds those missing outcomes. Its [design](../../specs/ui/system-design/task-navigation-responsiveness.md)
keeps task/workspace authorization and lifecycle unchanged.

The assumption check has no unresolved product choice: preserve current
navigation, filtering, session eligibility, file operations, and phone entry
points while eliminating redundant/blocking work. No delegation is authorized.

Related-package inventory:

- [Task surface render isolation](../task-surface-render-isolation/plan.md)
  owns memoization and virtualization. Preserve its existing implementation;
  this package addresses read ownership and restoration instead.
- [File tree hidden measurements](../file-tree-hidden-measurements/plan.md)
  is implemented. Preserve positive row measurements, contiguous geometry,
  and the existing large-tree mounted-row bound.
- [Workspace read recovery](../../specs/workspaces/system-design/workspace-read-recovery.md)
  owns workspace hydration failures. Do not bypass its authority with cached
  task data.
- The System Info query-cache ADR does not authorize a repository-wide query
  migration. Existing store-scoped promise patterns are sufficient here; the
  design records the local alternatives, so no new ADR is necessary.

## Scope

### In scope

- Overview cache-presence guard and mixed loaded/unloaded workflow regression.
- Shared shell, commit, and cumulative-diff request ownership with correct scope,
  invalidation, error, unmount, and reconnect behavior.
- Bounded recent-tree retention and progressive, dependency-aware restoration.
- Desktop/phone browser regressions, isolated before/after timings, debug
  comparison, and bounded memory investigation.

### Out of scope

- Runtime restart/deployment, stopping another user's test instance, database
  cleanup, host swap changes, or changing live debug settings.
- Message-pagination rewrites, backend API changes, new user settings,
  dependency upgrades, or speculative replacement of Zustand.
- Claiming an unmeasured backend or browser memory leak is repaired.

## Technical approach

1. Require an existing projection cache entry in
   `hooks/domains/kanban/use-swimlane-render-data.ts`; test the public hook with
   the real store and absent snapshots.
2. Add `lib/state/session-read-coordinator.ts`. Move request ownership out of
   individual session hooks; keep result ownership in existing slices and the
   scoped diff cache. Migrate `invalidateCumulativeDiffCache` and all its callers
   to explicit store ownership. Follow the design's identity and bounded-retention
   rules, with deferred-promise tests for cleanup and invalidation races.
3. Add `components/task/file-browser-tree-cache.ts`. Rework the restoration
   helper and loader to publish the root and successful folder merges while
   scheduling at most four independent reads for the active owner. Reuse valid
   cached trees on return; preserve parent dependency and stale-owner guards.
   Adapt loading/retry presentation only where it would hide usable rows.
4. Add causal desktop/mobile regressions and an opt-in isolated profiling case.
   Reuse managed E2E builds and fixture-owned resources. Record measurements
   without converting machine-dependent timings into flaky CI thresholds.

The compatibility matrix is intentionally narrow:

| Context | Intended behavior | Evidence |
| --- | --- | --- |
| Multiple consumers, same WS/read scope | One initialization request; shared completion | Deferred multi-hook tests and browser request correlation |
| Environment-only versus task-scoped shells | Distinct reads, only current scope publishes | Out-of-order scope transition test |
| Ready versus starting/terminal session | Preserve resource-specific empty/retry semantics | Existing and extended domain-hook tests |
| Task/workspace/auth/reconnect transition | No stale response/cache reuse across scopes | Coordinator tests, A-to-B-to-A, workspace isolation E2E |
| Desktop Dockview / phone Files unmount | Same data behavior, existing different composition | Chromium and mobile-chrome projects |
| Remote/custom executor | Same WS protocol; no new capability assumed | Contract-level readiness tests; no new executor-specific certification |

## ASCII UI preview

UI-01: Overview entry while one workflow is still loading. The current guard
can throw; the proposed view keeps available workflows usable.

```text
Desktop overview                  Phone overview
+----------------------------+    +-------------------------+
| Existing filters           |    | Existing filters        |
| Workflow A: available tasks|    | Workflow A              |
| Workflow B: loading        |    | available tasks         |
+----------------------------+    | Workflow B: loading     |
                                  +-------------------------+
```

This is the existing responsive overview composition, not a new board layout.
Labels describe states rather than prescribing new copy. It maps to `.1` and
is verified with missing-snapshot hook and rendered navigation tests.

UI-02: Files after task selection or return, while a child response is held.
Previously the uncached serial restoration showed a full-panel loader until
all expanded paths finished. The proposed view publishes available rows.

```text
Desktop task                         Phone task / Files
+---------------+----------------+   +--------------------------+
| Current task  | Files toolbar  |   | Task header              |
| chat/editor   | v src          |   | Files toolbar            |
|               |   app.ts       |   | v src               [...]|
|               | > pending-dir  |   |   app.ts            [...]|
|               | loading/retry  |   | > pending-dir       [...]|
+---------------+----------------+   | loading/retry            |
                                     +--------------------------+
                                     | Existing bottom nav      |
                                     +--------------------------+
```

Available rows stay interactive during loading or retry. A valid retained tree
can appear before any refresh response. Uncached root loading keeps the existing
full-panel loader; authoritative workspace unavailability keeps the recovery UI.
The Files viewport scrolls; toolbar and phone navigation retain their positions.
Spacing/names are illustrative, while availability, state precedence, one scroll
owner, and touch reachability are structural requirements (`.3`-`.6`).

Phone follows `session-mobile-layout.tsx`: Files is frequent primary content
with deep scrolling, so it keeps a focused full-height surface. Task selection
uses the existing `session-task-switcher-sheet.tsx` drawer. Shared state and
actions remain outside the responsive wrappers; no hover-only control is added.

## Tests

| Criteria | Regression and exact test file |
| --- | --- |
| `.1` | `renders a missing snapshot beside a loaded workflow` in new `hooks/domains/kanban/use-swimlane-render-data.test.tsx` |
| `.2`, `.5` | `shares initialization across consumers`, `drains one invalidation`, `ignores retired scope completion` in new coordinator tests and the three session-hook test files |
| `.3`, `.5` | `reuses only a current retained tree`, `evicts inactive trees by node and entry budget` in new `components/task/file-browser-tree-cache.test.ts` |
| `.4`, `.5` | `publishes root before held children`, `restores siblings concurrently after their parent`, `ignores old A completion after A-B-A` in `file-browser-restore-loader.test.tsx` |
| `.4`, `.6` | `keeps partial tree usable after a branch failure` in loader/load-state tests and browser specs |

Each implementation work order begins with an expected failing behavioral test,
then records RED/GREEN evidence. Do not make a test pass by exporting a private
implementation solely for assertion or weakening existing stale-response tests.

## E2E tests

Task 04 owns new `task/task-navigation-responsiveness.spec.ts` and
`task/mobile-task-navigation-responsiveness.spec.ts`. Use correlated WS response
holds, real task selection, and semantic visible-row assertions. Cover overview
hydration, request sharing, cold progressive loading, warm A-to-B-to-A, one failed
branch/retry, and workspace isolation. Preserve existing large-tree geometry and
mobile file actions. The mobile project exercises actual phone navigation.

An opt-in profile case in `task/task-navigation-profile.spec.ts` compares warmed
production builds with debug on/off and records browser/backend memory evidence.
Its output is diagnostic evidence, not a universal performance pass threshold.
Chromium measurements do not substitute for a Firefox follow-up if the original
browser still exhibits a problem after deterministic regressions pass.

## Work orders

- [x] [Task 01: Make overview hydration safe](task-01-overview-hydration.md)
- [x] [Task 02: Share session read ownership](task-02-shared-session-reads.md)
- [x] [Task 03: Restore file trees progressively](task-03-progressive-file-trees.md)
- [x] [Task 04: Verify navigation and investigate retained memory](task-04-navigation-evidence.md)
- [x] [Task 05: Immediate task route presentation](task-05-immediate-task-route.md)
- [x] [Task 06: Remove unnecessary work before task-switch paint](task-06-firefox-task-paint.md)

The sequence is 01 -> 02 -> 03 -> 04 -> 05 -> 06. It does not authorize parallel agents.
Install fresh-worktree dependencies once before implementation checks:
`(cd apps && pnpm install --frozen-lockfile)`. Each work order supplies complete
targeted commands; desktop/mobile suites run sequentially with managed limits.

## Risks

- A generation missing from a cache key can disclose stale context or suppress a
  necessary snapshot. Explicit same-store auth/reconnect and A-to-B-to-A tests
  are required.
- Concurrent merges can overwrite successful siblings or reset row identities.
  Merge against current owner state, preserving existing render-isolation tests.
- Retaining trees can worsen memory if references or timers escape eviction.
  Bound inactive snapshots and verify collection/retention separately from RSS.
- Caches must not turn transient errors into authoritative absence or hide
  workspace recovery. Root and branch failures need different assertions.
- The high live RSS may have a separate cause. If profiling reproduces one,
  record the retaining owner/root cause and reconcile its work order before
  claiming the overall performance problem is fully resolved.

## Documentation impact

Internal requirements, design, and work orders change. The how-to guidance in
`docs/public/tasks-and-workflows.md` now explains using
available Files rows during refresh and the adjacent Retry action. Navigation
labels, commands, APIs, settings, and screenshots remain unchanged.

## Verification results

The implementation and RED/GREEN evidence are recorded in Tasks 01-04.
Matched isolated production runs show median usable-file DOM time improving
from 3.74 s to 1.99 s with debug disabled (3.73 s to 1.68 s enabled), with
45 → 35 requests per warm switch and all required tree paths refreshed.
These are Chromium driver/DOM proxies; URL selection did not improve consistently.
The experiment does not reproduce or resolve the live high-RSS observation.

Completed validation includes the 95-test targeted regression run, desktop
navigation/geometry/isolation E2E, frontend typecheck, changed-file lint and
format checks, localization checks, specification catalog/lint, 62 public-doc
validator tests, 47 public pages, and actual changed-file PR documentation
coverage for all four work orders. All five final phone checks passed. The
full frontend command finished with 20,365 passed, four skipped, and seven
failures from one obsolete store mock. Those seven cases pass after correction.
A final ownership check added two environment-rebinding regressions and their
guard; all 51 affected hook/coordinator/handler cases, six desktop cases, two
affected phone cases, and both fixed profile modes pass after that change.
All 16 distinct desktop/phone behavior cases are covered across the runs.
Raw measurements are retained under ignored
`.kandev/diagnostics/`; Task 04 records their limits and exact fixture scope.

Live deployment, restart, and live Firefox certification remain outside this
package. The implementation is committed and published as PR #4041.


PR review follow-up preserves shell-error settlement and gives each session's
cumulative diff independent ownership within a shared environment. All 67
affected tests and TypeScript pass; Task 02 and Task 04 record the regression
evidence. The follow-up production build and desktop/phone smoke checks pass. CI and
review disposition remain pending on the published PR.

## Follow-up: immediate task route presentation

The user deployed merged revision e99ac10 on September 29 and reported remaining
whole-task loading. A fresh diagnostic bundle confirms that revision is running.
The original Files measurements did not measure the task route overlay. In an
isolated seeded preview, holding `/api/v1/agents` keeps the opaque route loader
visible on a warm task return; releasing it removes the loader. Client routing
currently waits for the full optional boot hydration fan-out on every switch.

Continue the already authorized repair through
[Task 05: Immediate task route presentation](task-05-immediate-task-route.md).
No delegation or changes to the personal instance are authorized or needed.

UI-03 (desktop and phone retain their existing navigation and scroll owners):

```text
Before                         After selection
+-------------------------+    +-------------------------+
| Loading task...         |    | Selected task header    |
| (all content covered)   |    | Cached chat / task view |
|                         |    | Independent panel loads |
+-------------------------+    +-------------------------+
                               | Phone bottom navigation |
                               +-------------------------+
```

- [x] Task 05: immediate task route presentation (`.5`-`.7`).

The follow-up separates essential task/session route resolution from optional
boot enrichment and presents valid current-store task/chat data while refreshing.
All 133 focused unit tests and 20 final desktop/phone browser tests pass,
including delayed optional reads, missing tasks, saved session selection, unread
cursors, and rapid returns with a fresh hydration snapshot. Task 05 records
the isolated browser measurements and their limits. PR/CI delivery is tracked
in the task session, separately from completed local implementation.

## September 29 task-switch follow-up

The [session refresh efficiency extension](../session-refresh-efficiency/plan.md#second-trace-task-switch-extension)
adds shared task identity reads, editor lifecycle attribution, shared PR
feedback, agent initialization, and integration health reads. Its
[trace report](../session-refresh-efficiency/trace-2026-09-29-task-switch.md)
is new evidence, not a replacement for this package's recorded results.
Preserve this package's row, file-tree, and scoped-read regression guarantees.
Task 04 and Tasks 06–08 now have implementation and desktop/phone regression
coverage. Task 05 remains open pending editor-owner attribution. These tasks do
not reopen completed work here.

## Added work order

- [Task 07: Restore shared-store sidebar rows](task-07-shared-sidebar-state.md), after Task 06.
