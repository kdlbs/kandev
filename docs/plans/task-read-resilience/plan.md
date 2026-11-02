---
created: 2026-10-27
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-005
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
legacy_specs: []
---

# Implementation plan: Task read resilience

## Overview

Recover automatically across a short persistence outage and keep the current task visible during temporary read failures.
One work order delivers the retry policy, route continuity, recovery presentation, and regression coverage together.
Platform owns this contract because it owns shared interactive-read availability and the existing navigation recovery owner.
Tasks retains record authority, permissions, selected-session ownership, and permanent missing-task behavior.

## Evidence and scope decisions

On October 27, the writer health probe exceeded its two-second deadline at 16:50:21.639 UTC.
The API admission gate remained unhealthy until 16:50:34.640.
Task reads failed around 16:50:23, :25, and :30. The browser exhausted recovery before the gate reopened.
The same task loaded again around 16:50:45.
The incident window contained 36 HTTP 503 responses.
The exact writer operation remains unknown. Pool counters do not identify it.

The running build was `b6263ea4c3165c86ec60e29e798475ad552ad6ce`.
Its relevant health, middleware, pool, navigation-read, and route source matches this checkout.
The local evidence is `.kandev/diagnostics/task-read-2026-10-27T1650Z.json` and bundle `266a0fdf72e7fc36716a2159c1d36ee8`.
These local artifacts are not required to execute the work order.
The export has retention limits, but this incident window is present.

The user asked about longer waits and frontend resilience.
The proposed policy uses thirty seconds of visible-tab time and at most five total identity attempts.
This value is an implementation proposal for review, not a user-confirmed timing preference.
AC-005.2 changes the earlier retry limit. AC-005.7 expands continuity to an already visible current projection.
AC-005.8 through AC-005.10 define automatic recovery, hidden-tab accounting, and accessible progress.
The existing owner and permission boundaries remain sufficient. This numerical policy needs no separate ADR.

## Scope

### In scope

- One shared recovery cycle with bounded time and request count.
- Current task and valid selected-session continuity through temporary errors.
- Cold-load progress, retained-content notices, exhausted recovery, and manual Retry.
- Desktop and phone coverage, translated availability copy, and public recovery guidance.

### Out of scope

- Database writer attribution, SQL optimizations, health-probe policy, and backend timeout changes.
- Bypassing persistence admission, extra database connections, or new health polling endpoints.
- Generic fetch retries, mutation retries, offline writes, page reloads, or new task/session records.
- New feature flags, settings, dependencies, or telemetry infrastructure.

## Technical approach

Keep `TaskNavigationReads` as the only owner of retries for the selected navigation identity.
Use its existing generation, scope, retention, shared promise, and child-request cancellation boundaries.
Add a private visible-time clock beside that owner.
The design specifies the retry schedule, deadline precedence, hidden-tab behavior, and Retry-After handling.
Do not lengthen the individual request deadline or add a route-local retry loop.

Carry temporary error classification into the route view.
Reuse `useTaskRouteProjection` only under its existing current-task and workspace eligibility checks.
Retain current store presentation without replaying old hydration snapshots.
Never use a previous task's content to fill an unavailable destination.
Clear retained presentation on permanent errors or scope invalidation.

Use the shared navigation snapshot for retry notices and controls.
Keep a neutral explanation of temporary availability and a stable Retry label.
Update all supported locale catalogs and generated locales for changed copy.
Add a short recovery paragraph under Troubleshooting in `docs/public/tasks-and-workflows.md` after implementation.
That page remains a how-to guide. No new public page or screenshot is required.

The completed [task navigation package](../task-navigation-availability/plan.md) remains historical evidence.
Its original two-retry counts describe the previous policy.
The [journey loading package](../journey-data-efficiency/plan.md) retains shared session reads and projection ownership.
This package changes neither package's completed status or recorded test results.
Update their affected test scenarios through this work order, rather than rewriting historical verification.

## ASCII UI preview

UI-01: Cold task route during automatic recovery. Entry: sidebar, task picker, or direct URL.

```text
Task temporarily unavailable
Retrying...
[Retry (disabled)]
[Back to task overview]
```

Desktop and phone share this composition.
The overview action remains active while recovery runs.
After exhaustion, Retry becomes active and the retry progress ends.
The phone uses full-width actions inside the route content with targets of at least 44px.
Desktop uses ordinary 28px controls.

UI-02: A task view already exists when a temporary read fails.

The strip spans the task content area directly below its header and above session tabs and panels.
It participates in the layout. The phone header offset reserves space above it.
The existing sidebar is outside this strip.

```text
Desktop
[Task details could not be refreshed. Retrying...  Retry (disabled)]
[Current task and selected conversation remain visible             ]

Phone
[Task details could not be refreshed.]
[Retrying...                        ]
[        Retry (disabled)           ]
[Selected conversation              ]
```

After exhaustion, the notice remains and Retry becomes active.
Success removes the notice and preserves the session and draft.
The existing route owns vertical scrolling and safe-area clearance.
No drawer or modal interrupts this persistent status.
The nearest precedents are `TaskLoadErrorState`, `TaskPageInner`, and the dedicated phone composition in `task-layout.tsx`.
`TaskNavigationReadFeedback` currently floats above content. This work changes that component to the in-flow strip shown here.
State and actions remain shared across viewports.
Structural choices are required. Copy, spacing, and line lengths are illustrative and must use translation keys.
UI-01 maps to AC-005.1, .2, .6, .8, and .10. UI-02 maps to AC-005.3, .4, .6, .7, and .10.

## Tests

The names below identify the delivered regressions.

| Acceptance | Evidence |
| --- | --- |
| AC-005.2, .8 | `task-navigation-reads.test.ts`: automatic recovery after thirteen seconds; fifteen-second recovery boundary; thirty-second deadline for never-settling reads |
| AC-005.2, .9, .10 | Clock and owner tests: hidden-time pause; no background attempts; Retry-After beyond the cycle; timer/listener cleanup |
| AC-005.3, .4 | Owner and route tests: mixed triggers share one cycle; selection survives; scope changes cancel timers and fence late responses |
| AC-005.5, .7 | Route view/component tests: current projection survives temporary failure; 404/auth failures and another task never retain it |
| AC-005.1, .6, .7, .10 | Component and desktop/phone E2E: progress, stable accessible names, retained view, exhaustion, manual recovery, touch geometry |

## E2E tests

Extend `tests/task/task-transient-read-recovery.spec.ts` for Chromium and its `mobile-` counterpart for mobile Chrome.
Use disposable fixtures and structured `503 {code: "persistence_unavailable"}` responses.
Add a timed outage that remains active after the old seven-second recovery period.
Release it at thirteen seconds and require automatic recovery without clicking Retry or dispatching a foreground event.
Preserve existing exhaustion and manual-recovery scenarios with the new attempt count.
Add the current-projection case and retain a valid secondary session and draft.
Use request barriers and causal HTTP waits. Fake timers belong in unit tests.
Do not add arbitrary browser sleeps or infer request absence from a timeout exception.
Capture one desktop and one phone recovery state through the focused tests.

## Work orders

- [x] [Task 01: Recover task reads across a health cycle](task-01-recover-task-reads.md)

## Verification results

Design validation passed on October 27, 2026:

- `python3 scripts/list-docs.py validate`: 408 decisions and 1,760 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation preflight: actual documentation diff exempt; linked work order covered a synthetic navigation-read source change without errors.
- `git diff --check`: passed. Both new plan files appear in workspace status.
- Plain-language review: new plan sentences stay within twenty-five words.

Implementation completed on October 27, 2026:

- Focused Vitest: 144 tests across navigation reads, recovery clock, route hydration and recovery, route-view derivation, task-page content, and phone feedback offset. Includes later-delay visibility accounting and temporary-to-permanent projection invalidation.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, and `pnpm run i18n:check`: passed. The neutral availability description is translated in all seven catalogs.
- Targeted ESLint: no errors. Remaining warnings are existing file-length limits.
- Chromium `task-transient-read-recovery` and `task-loading-state`: 8 passed. Covers exhaustion, manual recovery, a thirteen-second timed outage that recovers without another user event, and a retained current projection.
- Mobile Chrome `mobile-task-transient-read-recovery` and `mobile-task-loading-state`: 6 passed. Covers phone recovery geometry and the same timed automatic recovery.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`: passed. `docs/public/tasks-and-workflows.md` gained a short troubleshooting entry.

Static review follow-up completed on October 27, 2026:

- Each retry delay now tracks its own visible-time start. Hiding during a later retry, after a slow attempt, or during a delayed Retry-After no longer shortens the wait. Clock and owner regressions cover all three cases.
- A later permanent failure from the current shared cycle now invalidates the retained projection and any loaded route view. The page shows the generic unavailable surface. Route/page regressions cover 503 exhaustion followed by manual recovery to 404 and a subsequent 403.

The database writer remains unattributed. This delivery reduces task-navigation disruption only; it does not remove the underlying writer pressure.


## Risks

- Increasing retries without a total deadline can turn a short error into an excessive wait.
- Budget expiry must remain distinct from cancellation, or the error surface can remain stuck in retrying state.
- A retained projection must not bypass a 404, authorization error, archive rule, or workspace change.
- A second retry owner can multiply requests. The route, page content, and foreground hooks must share one cycle.
- Retained content does not guarantee that mutations succeed during the backend outage.
- This repair reduces disruption but does not identify or remove the database writer pressure.
